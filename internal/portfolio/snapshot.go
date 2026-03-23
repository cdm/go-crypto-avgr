package portfolio

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/christian/crypto-avgr/internal/coingecko"
	"github.com/christian/crypto-avgr/internal/costbasis"
	"github.com/christian/crypto-avgr/internal/etherscan"
	"github.com/christian/crypto-avgr/internal/notknowntokens"
)

// DenylistOptions skips contracts listed in .notknowntokens and records newly CoinGecko-unlisted addresses.
type DenylistOptions struct {
	SkipContracts      map[string]struct{}
	RecordNotKnownPath string
}

func skipContract(m map[string]struct{}, contract string) bool {
	if m == nil {
		return false
	}
	_, ok := m[strings.ToLower(contract)]
	return ok
}

// SnapshotOptions configures BuildSnapshot.
type SnapshotOptions struct {
	IncludeAvgCost bool
	// HideUnlisted omits tokens with no CoinGecko contract listing from the snapshot (after recording them).
	HideUnlisted bool
	DenylistOptions
}

// TokenRow is one ERC-20 position with market data.
type TokenRow struct {
	Contract     string  `json:"contract"`
	Symbol       string  `json:"symbol"`
	Decimals     int     `json:"decimals"`
	BalanceHuman float64 `json:"balance_human"`
	SpotUSD      float64 `json:"spot_usd"`
	ValueUSD     float64 `json:"value_usd"`
	Change24hPct float64 `json:"change_24h_pct,omitempty"`
	CoinGeckoID  string  `json:"coingecko_id,omitempty"`
	// PriceNote is set when USD pricing is unavailable (e.g. not listed on CoinGecko).
	PriceNote string `json:"price_note,omitempty"`
	// AvgCostUSD is estimated average buy in USD per 1 token (same heuristic as pnl).
	AvgCostUSD float64 `json:"avg_cost_usd,omitempty"`
	// AvgCostNote explains missing avg cost, CoinGecko/history errors, or replay/balance drift.
	AvgCostNote string `json:"avg_cost_note,omitempty"`
}

// NativeETHRow is native ETH (not ERC-20).
type NativeETHRow struct {
	BalanceHuman float64 `json:"balance_human"`
	SpotUSD      float64 `json:"spot_usd"`
	ValueUSD     float64 `json:"value_usd"`
	Change24hPct float64 `json:"change_24h_pct,omitempty"`
}

// Snapshot is holdings for an address.
type Snapshot struct {
	Address string        `json:"address"`
	Native  *NativeETHRow `json:"native,omitempty"`
	Tokens  []TokenRow    `json:"tokens"`
}

// PnLRow extends TokenRow with estimated cost basis from transfer replay (avg lives on TokenRow).
type PnLRow struct {
	TokenRow
	CostBasisUSD   float64 `json:"cost_basis_usd"`
	UnrealizedUSD  float64 `json:"unrealized_pnl_usd"`
	UnrealizedPct  float64 `json:"unrealized_pnl_pct"`
	HistoryWarning string  `json:"history_warning,omitempty"`
}

// averageCostUSD computes volume-weighted average buy (USD per token) from ERC-20 transfer history.
// fatalErr is set only when transfer rows cannot be parsed (caller should abort the whole operation).
// softFail is a user-facing message when history prefetch or replay fails (non-fatal per token).
func averageCostUSD(address string, rows []etherscan.TokenTransfer, humanBalance float64, cg *coingecko.Client, coinID string) (avg float64, replayQty float64, driftWarn string, fatalErr error, softFail string) {
	transfers, err := costbasis.FromEtherscan(rows, address)
	if err != nil {
		return 0, 0, "", err, ""
	}
	costbasis.SortTransfers(transfers)

	dates := uniqueInboundDates(transfers, address)
	if err := prefetchHistory(cg, coinID, dates); err != nil {
		return 0, 0, "", nil, fmt.Sprintf("historical prices incomplete: %v", err)
	}

	priceAt := func(t time.Time) (float64, error) {
		return cg.HistoricalUSD(coinID, t)
	}
	qty, avgVal, err := costbasis.ReplayAverageCost(address, transfers, priceAt)
	if err != nil {
		return 0, 0, "", nil, fmt.Sprintf("replay failed: %v", err)
	}
	if humanBalance > 0 && qty > 0 {
		rel := (qty - humanBalance) / humanBalance
		if rel < 0 {
			rel = -rel
		}
		if rel > 0.05 {
			driftWarn = fmt.Sprintf("replay quantity %.8f differs from chain balance %.8f (>5%%); avg cost may be off", qty, humanBalance)
		}
	}
	return avgVal, qty, driftWarn, nil, ""
}

// BuildSnapshot fetches transfers, balances, and CoinGecko spot prices.
// If opts.IncludeAvgCost is true, listed tokens also get AvgCostUSD from the same replay as pnl (slower).
func BuildSnapshot(es *etherscan.Client, cg *coingecko.Client, address string, opts SnapshotOptions) (*Snapshot, error) {
	address = strings.ToLower(address)
	all, err := es.FetchAllTokenTransfers(address)
	if err != nil {
		return nil, err
	}
	byContract := groupTransfersByContract(all)

	var tokens []TokenRow
	for contract, rows := range byContract {
		if skipContract(opts.SkipContracts, contract) {
			continue
		}
		dec, sym, err := decimalsSymbolFromRows(rows)
		if err != nil {
			return nil, err
		}
		bal, err := es.TokenBalance(contract, address)
		if err != nil {
			return nil, fmt.Errorf("balance %s: %w", contract, err)
		}
		if bal.Sign() == 0 {
			continue
		}
		human := costbasis.HumanAmount(bal, dec)
		row := TokenRow{
			Contract:     strings.ToLower(contract),
			Symbol:       sym,
			Decimals:     dec,
			BalanceHuman: human,
		}
		info, err := cg.GetByContract(contract)
		if err != nil {
			if errors.Is(err, coingecko.ErrNotListed) {
				if err := notknowntokens.Record(opts.RecordNotKnownPath, contract); err != nil {
					return nil, err
				}
				time.Sleep(200 * time.Millisecond)
				if opts.HideUnlisted {
					continue
				}
			}
			row.SpotUSD = 0
			row.ValueUSD = 0
			if errors.Is(err, coingecko.ErrNotListed) {
				row.PriceNote = "not listed on CoinGecko (no USD quote)"
			} else {
				row.PriceNote = fmt.Sprintf("CoinGecko: %v", err)
			}
			row.AvgCostNote = "avg cost needs CoinGecko contract listing + history"
			if !errors.Is(err, coingecko.ErrNotListed) {
				time.Sleep(200 * time.Millisecond)
			}
		} else {
			row.SpotUSD = info.CurrentUSD
			row.ValueUSD = human * info.CurrentUSD
			row.Change24hPct = info.Change24hPct
			row.CoinGeckoID = info.ID
			time.Sleep(1100 * time.Millisecond) // CoinGecko free-tier courtesy delay
			if opts.IncludeAvgCost {
				avg, _, drift, fatalErr, soft := averageCostUSD(address, rows, human, cg, info.ID)
				if fatalErr != nil {
					return nil, fatalErr
				}
				if soft != "" {
					row.AvgCostNote = soft
				} else {
					row.AvgCostUSD = avg
					row.AvgCostNote = drift
				}
				time.Sleep(1100 * time.Millisecond)
			}
		}
		tokens = append(tokens, row)
	}
	sort.Slice(tokens, func(i, j int) bool {
		return strings.ToLower(tokens[i].Symbol) < strings.ToLower(tokens[j].Symbol)
	})

	wei, err := es.NativeBalance(address)
	if err != nil {
		return nil, err
	}
	var native *NativeETHRow
	if wei.Sign() > 0 {
		ethHuman := costbasis.HumanAmount(wei, 18)
		info, err := cg.NativeETHInfo()
		time.Sleep(1100 * time.Millisecond)
		if err == nil {
			native = &NativeETHRow{
				BalanceHuman: ethHuman,
				SpotUSD:      info.CurrentUSD,
				ValueUSD:     ethHuman * info.CurrentUSD,
				Change24hPct: info.Change24hPct,
			}
		}
	}

	return &Snapshot{Address: address, Native: native, Tokens: tokens}, nil
}

func groupTransfersByContract(all []etherscan.TokenTransfer) map[string][]etherscan.TokenTransfer {
	m := make(map[string][]etherscan.TokenTransfer)
	for _, t := range all {
		c := strings.ToLower(t.ContractAddress)
		m[c] = append(m[c], t)
	}
	return m
}

func decimalsSymbolFromRows(rows []etherscan.TokenTransfer) (int, string, error) {
	if len(rows) == 0 {
		return 0, "", fmt.Errorf("empty rows")
	}
	d, err := parseDecimals(rows[0].TokenDecimal)
	if err != nil {
		return 0, "", err
	}
	return d, rows[0].TokenSymbol, nil
}

func parseDecimals(s string) (int, error) {
	// reuse costbasis parsing via small duplicate to avoid import cycle - simple Atoi
	var n int
	_, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
	if err != nil || n < 0 || n > 36 {
		return 0, fmt.Errorf("bad decimals %q", s)
	}
	return n, nil
}

// ComputePnL builds PnL rows for tokens matching filter (nil = all).
func ComputePnL(es *etherscan.Client, cg *coingecko.Client, address string, tokenFilter func(TokenRow) bool, deny DenylistOptions) ([]PnLRow, error) {
	address = strings.ToLower(address)
	all, err := es.FetchAllTokenTransfers(address)
	if err != nil {
		return nil, err
	}
	byContract := groupTransfersByContract(all)

	var out []PnLRow
	for contract, rows := range byContract {
		if skipContract(deny.SkipContracts, contract) {
			continue
		}
		bal, err := es.TokenBalance(contract, address)
		if err != nil {
			return nil, err
		}
		if bal.Sign() == 0 {
			continue
		}
		dec, sym, err := decimalsSymbolFromRows(rows)
		if err != nil {
			return nil, err
		}
		human := costbasis.HumanAmount(bal, dec)
		base := TokenRow{
			Contract:     contract,
			Symbol:       sym,
			Decimals:     dec,
			BalanceHuman: human,
		}
		info, err := cg.GetByContract(contract)
		if err != nil {
			if errors.Is(err, coingecko.ErrNotListed) {
				if err := notknowntokens.Record(deny.RecordNotKnownPath, contract); err != nil {
					return nil, err
				}
				time.Sleep(200 * time.Millisecond)
				continue
			}
			msg := fmt.Sprintf("CoinGecko contract lookup failed: %v", err)
			out = append(out, PnLRow{
				TokenRow:       base,
				HistoryWarning: msg,
			})
			time.Sleep(200 * time.Millisecond)
			continue
		}
		time.Sleep(1100 * time.Millisecond)
		base.SpotUSD = info.CurrentUSD
		base.ValueUSD = human * info.CurrentUSD
		base.Change24hPct = info.Change24hPct
		base.CoinGeckoID = info.ID
		if tokenFilter != nil && !tokenFilter(base) {
			continue
		}

		avg, _, driftWarn, fatalErr, softFail := averageCostUSD(address, rows, human, cg, info.ID)
		if fatalErr != nil {
			return nil, fatalErr
		}
		if softFail != "" {
			out = append(out, PnLRow{
				TokenRow:       base,
				HistoryWarning: softFail,
			})
			time.Sleep(1100 * time.Millisecond)
			continue
		}
		cb := human * avg
		uv := base.ValueUSD - cb
		var pct float64
		if cb != 0 {
			pct = (uv / cb) * 100
		}
		base.AvgCostUSD = avg
		base.AvgCostNote = driftWarn
		out = append(out, PnLRow{
			TokenRow:       base,
			CostBasisUSD:   cb,
			UnrealizedUSD:  uv,
			UnrealizedPct:  pct,
			HistoryWarning: driftWarn,
		})
		time.Sleep(1100 * time.Millisecond)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Symbol) < strings.ToLower(out[j].Symbol)
	})
	return out, nil
}

func uniqueInboundDates(transfers []costbasis.Transfer, user string) []time.Time {
	user = strings.ToLower(user)
	seen := make(map[string]bool)
	var out []time.Time
	for _, tr := range transfers {
		if tr.To != user {
			continue
		}
		d := tr.Time.UTC().Format("2006-01-02")
		if seen[d] {
			continue
		}
		seen[d] = true
		out = append(out, tr.Time)
	}
	return out
}

func prefetchHistory(cg *coingecko.Client, coinID string, dates []time.Time) error {
	for _, t := range dates {
		if _, err := cg.HistoricalUSD(coinID, t); err != nil {
			return err
		}
		time.Sleep(1100 * time.Millisecond)
	}
	return nil
}

// MatchToken returns true if selector matches contract (0x...) or symbol (case-insensitive).
func MatchToken(row TokenRow, selector string) bool {
	sel := strings.TrimSpace(selector)
	if sel == "" {
		return false
	}
	if strings.HasPrefix(strings.ToLower(sel), "0x") {
		return strings.EqualFold(row.Contract, sel)
	}
	return strings.EqualFold(row.Symbol, sel)
}

// FilterAll is a token filter that includes every row.
func FilterAll(TokenRow) bool { return true }
