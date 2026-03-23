package etherscan

import (
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultBaseURL = "https://api.etherscan.io/v2/api"

// minIntervalBetweenCalls stays under the free-tier cap (3 calls/sec); margin avoids NOTOK bursts.
const minIntervalBetweenCalls = 400 * time.Millisecond

const maxRateLimitRetries = 8

// Client calls Etherscan HTTP API (Ethereum mainnet, chainid=1).
type Client struct {
	APIKey  string
	BaseURL string
	HTTP    *http.Client

	mu       sync.Mutex
	lastCall time.Time
}

func NewClient(apiKey string) *Client {
	return &Client{
		APIKey:  apiKey,
		BaseURL: defaultBaseURL,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// waitRateLimit enforces a minimum spacing between API calls (global per client).
func (c *Client) waitRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.lastCall.IsZero() {
		if d := minIntervalBetweenCalls - time.Since(c.lastCall); d > 0 {
			time.Sleep(d)
		}
	}
	c.lastCall = time.Now()
}

func isRateLimitedResponse(o *apiResponse, body []byte) bool {
	var rs string
	_ = json.Unmarshal(o.Result, &rs)
	msg := strings.ToLower(o.Message + " " + rs + " " + string(body))
	return strings.Contains(msg, "rate limit") || strings.Contains(msg, "max calls per sec") ||
		strings.Contains(msg, "too many requests")
}

type apiResponse struct {
	Status  string          `json:"status"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

// TokenTransfer is one row from account action=tokentx.
type TokenTransfer struct {
	BlockNumber      string `json:"blockNumber"`
	TimeStamp        string `json:"timeStamp"`
	Hash             string `json:"hash"`
	From             string `json:"from"`
	To               string `json:"to"`
	ContractAddress  string `json:"contractAddress"`
	Value            string `json:"value"`
	TokenSymbol      string `json:"tokenSymbol"`
	TokenDecimal     string `json:"tokenDecimal"`
	TransactionIndex string `json:"transactionIndex"`
}

// FetchAllTokenTransfers paginates tokentx until a page returns fewer than offset rows.
func (c *Client) FetchAllTokenTransfers(address string) ([]TokenTransfer, error) {
	const offset = 1000
	var all []TokenTransfer
	for page := 1; ; page++ {
		batch, err := c.fetchTokenTransfersPage(address, page, offset)
		if err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		if len(batch) < offset {
			break
		}
	}
	return all, nil
}

func (c *Client) fetchTokenTransfersPage(address string, page, offset int) ([]TokenTransfer, error) {
	q := url.Values{}
	q.Set("chainid", "1")
	q.Set("module", "account")
	q.Set("action", "tokentx")
	q.Set("address", address)
	q.Set("page", strconv.Itoa(page))
	q.Set("offset", strconv.Itoa(offset))
	q.Set("sort", "asc")
	q.Set("apikey", c.APIKey)

	var raw apiResponse
	if err := c.get(q, &raw); err != nil {
		return nil, err
	}
	if raw.Status != "1" && raw.Message != "No transactions found" {
		// Etherscan returns status 0 with message for empty; also "No transactions found"
		var s string
		if err := json.Unmarshal(raw.Result, &s); err == nil && s != "" {
			if raw.Message == "No transactions found" || s == "No transactions found" {
				return nil, nil
			}
			return nil, fmt.Errorf("etherscan tokentx: %s — %s", raw.Message, s)
		}
		if raw.Message != "" {
			return nil, fmt.Errorf("etherscan tokentx: %s", raw.Message)
		}
	}
	if len(raw.Result) == 0 || string(raw.Result) == `"No transactions found"` {
		return nil, nil
	}
	var list []TokenTransfer
	if err := json.Unmarshal(raw.Result, &list); err != nil {
		return nil, fmt.Errorf("decode tokentx: %w", err)
	}
	return list, nil
}

// TokenBalance returns raw token balance (smallest units) as big.Int.
func (c *Client) TokenBalance(contractAddress, holderAddress string) (*big.Int, error) {
	q := url.Values{}
	q.Set("chainid", "1")
	q.Set("module", "account")
	q.Set("action", "tokenbalance")
	q.Set("contractaddress", contractAddress)
	q.Set("address", holderAddress)
	q.Set("tag", "latest")
	q.Set("apikey", c.APIKey)

	var raw apiResponse
	if err := c.get(q, &raw); err != nil {
		return nil, err
	}
	if raw.Status != "1" {
		var s string
		_ = json.Unmarshal(raw.Result, &s)
		return nil, fmt.Errorf("etherscan tokenbalance: %s %s", raw.Message, s)
	}
	var str string
	if err := json.Unmarshal(raw.Result, &str); err != nil {
		return nil, fmt.Errorf("decode tokenbalance: %w", err)
	}
	n := new(big.Int)
	if _, ok := n.SetString(str, 10); !ok {
		return nil, fmt.Errorf("invalid tokenbalance: %q", str)
	}
	return n, nil
}

// NativeBalance returns ETH balance in wei.
func (c *Client) NativeBalance(address string) (*big.Int, error) {
	q := url.Values{}
	q.Set("chainid", "1")
	q.Set("module", "account")
	q.Set("action", "balance")
	q.Set("address", address)
	q.Set("tag", "latest")
	q.Set("apikey", c.APIKey)

	var raw apiResponse
	if err := c.get(q, &raw); err != nil {
		return nil, err
	}
	if raw.Status != "1" {
		var s string
		_ = json.Unmarshal(raw.Result, &s)
		return nil, fmt.Errorf("etherscan balance: %s %s", raw.Message, s)
	}
	var str string
	if err := json.Unmarshal(raw.Result, &str); err != nil {
		return nil, fmt.Errorf("decode balance: %w", err)
	}
	n := new(big.Int)
	if _, ok := n.SetString(str, 10); !ok {
		return nil, fmt.Errorf("invalid balance: %q", str)
	}
	return n, nil
}

func (c *Client) get(q url.Values, out *apiResponse) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return err
	}
	u.RawQuery = q.Encode()
	rawURL := u.String()

	var lastBody []byte
	for attempt := 0; ; attempt++ {
		if attempt >= maxRateLimitRetries {
			return fmt.Errorf("etherscan: rate limit after %d retries: %s", maxRateLimitRetries, truncate(lastBody, 200))
		}
		c.waitRateLimit()

		req, err := http.NewRequest(http.MethodGet, rawURL, nil)
		if err != nil {
			return err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return err
		}
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return err
		}
		lastBody = body
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("etherscan HTTP %d: %s", resp.StatusCode, truncate(body, 200))
		}
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode etherscan response: %w", err)
		}
		if isRateLimitedResponse(out, body) {
			backoff := time.Second + time.Duration(attempt)*400*time.Millisecond
			time.Sleep(backoff)
			continue
		}
		return nil
	}
}

func truncate(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}
