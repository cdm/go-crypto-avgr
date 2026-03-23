package coingecko

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// ErrNotListed means CoinGecko has no asset for this Ethereum contract (HTTP 404 on the contract endpoint).
// Many ERC-20s are not indexed; this is expected, not an API key failure.
var ErrNotListed = errors.New("token not listed on CoinGecko for this contract")

const defaultBaseURL = "https://api.coingecko.com/api/v3"

// Client fetches prices from CoinGecko (Ethereum mainnet contract lookup).
type Client struct {
	APIKey  string // optional; sent as x-cg-demo-api-key when set
	BaseURL string
	HTTP    *http.Client

	mu                sync.Mutex
	contractToID      map[string]string
	unlistedContract  map[string]struct{} // normalized 0x-lowercase address
	historyPriceCache map[historyKey]float64
}

type historyKey struct {
	CoinID string
	Date   string // dd-mm-yyyy UTC
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:            apiKey,
		BaseURL:           defaultBaseURL,
		HTTP:              &http.Client{Timeout: 45 * time.Second},
		contractToID:      make(map[string]string),
		unlistedContract:  make(map[string]struct{}),
		historyPriceCache: make(map[historyKey]float64),
	}
}

// ContractCoin holds metadata and spot from /coins/ethereum/contract/{addr}.
type ContractCoin struct {
	ID           string `json:"id"`
	Symbol       string `json:"symbol"`
	Name         string `json:"name"`
	CurrentUSD   float64
	Change24hPct float64
}

type contractResponse struct {
	ID         string `json:"id"`
	Symbol     string `json:"symbol"`
	Name       string `json:"name"`
	MarketData struct {
		CurrentPrice struct {
			USD float64 `json:"usd"`
		} `json:"current_price"`
		PriceChangePercentage24h float64 `json:"price_change_percentage_24h"`
	} `json:"market_data"`
}

func normalizeContractKey(contractAddress string) string {
	s := strings.ToLower(strings.TrimSpace(contractAddress))
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + strings.TrimPrefix(s, "0x")
	}
	return s
}

// GetByContract returns coin info and USD price for an ERC-20 on Ethereum.
func (c *Client) GetByContract(contractAddress string) (*ContractCoin, error) {
	key := normalizeContractKey(contractAddress)
	c.mu.Lock()
	if _, ok := c.unlistedContract[key]; ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w (%s)", ErrNotListed, key)
	}
	c.mu.Unlock()

	path := fmt.Sprintf("%s/coins/ethereum/contract/%s", strings.TrimSuffix(c.BaseURL, "/"), key)
	var out contractResponse
	if err := c.getJSON(path, &out, ErrNotListed); err != nil {
		if errors.Is(err, ErrNotListed) {
			c.mu.Lock()
			c.unlistedContract[key] = struct{}{}
			c.mu.Unlock()
		}
		return nil, err
	}
	return &ContractCoin{
		ID:           out.ID,
		Symbol:       out.Symbol,
		Name:         out.Name,
		CurrentUSD:   out.MarketData.CurrentPrice.USD,
		Change24hPct: out.MarketData.PriceChangePercentage24h,
	}, nil
}

// CoinIDForContract resolves and caches contract -> CoinGecko id.
func (c *Client) CoinIDForContract(contractAddress string) (string, error) {
	key := normalizeContractKey(contractAddress)
	c.mu.Lock()
	if id, ok := c.contractToID[key]; ok {
		c.mu.Unlock()
		return id, nil
	}
	c.mu.Unlock()

	info, err := c.GetByContract(contractAddress)
	if err != nil {
		return "", err
	}
	c.mu.Lock()
	c.contractToID[key] = info.ID
	c.mu.Unlock()
	return info.ID, nil
}

// HistoricalUSD returns daily USD price at 00:00 UTC for the given date (time in UTC).
func (c *Client) HistoricalUSD(coinID string, t time.Time) (float64, error) {
	dateStr := t.UTC().Format("02-01-2006") // dd-mm-yyyy
	k := historyKey{CoinID: coinID, Date: dateStr}
	c.mu.Lock()
	if v, ok := c.historyPriceCache[k]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	path := fmt.Sprintf("%s/coins/%s/history?date=%s",
		strings.TrimSuffix(c.BaseURL, "/"),
		url.PathEscape(coinID),
		url.QueryEscape(dateStr))
	var hist struct {
		MarketData struct {
			CurrentPrice struct {
				USD float64 `json:"usd"`
			} `json:"current_price"`
		} `json:"market_data"`
	}
	if err := c.getJSON(path, &hist, nil); err != nil {
		return 0, err
	}
	usd := hist.MarketData.CurrentPrice.USD
	c.mu.Lock()
	c.historyPriceCache[k] = usd
	c.mu.Unlock()
	return usd, nil
}

// NativeETHInfo uses coin id "ethereum" for spot (not contract).
func (c *Client) NativeETHInfo() (*ContractCoin, error) {
	path := fmt.Sprintf("%s/coins/ethereum", strings.TrimSuffix(c.BaseURL, "/"))
	var out contractResponse
	if err := c.getJSON(path, &out, nil); err != nil {
		return nil, err
	}
	return &ContractCoin{
		ID:           out.ID,
		Symbol:       out.Symbol,
		Name:         out.Name,
		CurrentUSD:   out.MarketData.CurrentPrice.USD,
		Change24hPct: out.MarketData.PriceChangePercentage24h,
	}, nil
}

func (c *Client) getJSON(rawURL string, out any, notFoundErr error) error {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	if c.APIKey != "" {
		req.Header.Set("x-cg-demo-api-key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		if notFoundErr != nil {
			return notFoundErr
		}
		return fmt.Errorf("coingecko HTTP 404: %s", truncate(body, 200))
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coingecko HTTP %d: %s", resp.StatusCode, truncate(body, 300))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("decode coingecko: %w", err)
	}
	return nil
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
