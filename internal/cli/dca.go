package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/christian/crypto-avgr/internal/coingecko"
	"github.com/christian/crypto-avgr/internal/dca"
	"github.com/christian/crypto-avgr/internal/etherscan"
	"github.com/christian/crypto-avgr/internal/notknowntokens"
	"github.com/christian/crypto-avgr/internal/portfolio"
	"github.com/christian/crypto-avgr/internal/wallet"
	"github.com/spf13/cobra"
)

func newDCACmd() *cobra.Command {
	var address, token, buyPrices string
	var targetAvg float64
	cmd := &cobra.Command{
		Use:   "dca",
		Short: "Suggest buy sizes at limit prices to reach a target average",
		Long:  "Uses volume-weighted average math: x = B*(A-T)/(T-p). Default target is current spot. Skips contracts in .notknowntokens (working directory).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireEtherscanKey(); err != nil {
				return err
			}
			if strings.TrimSpace(buyPrices) == "" {
				return fmt.Errorf("--buy-price is required (comma-separated USD prices per token)")
			}
			prices, err := parseFloatList(buyPrices)
			if err != nil {
				return err
			}
			addr, err := wallet.NormalizeAddress(address)
			if err != nil {
				return err
			}
			deny, err := denylistOptions()
			if err != nil {
				return err
			}
			es := etherscan.NewClient(etherscanAPIKey)
			cg := coingecko.NewClient(coingeckoAPIKey)

			var filter func(portfolio.TokenRow) bool
			if token != "" {
				sel := token
				filter = func(tr portfolio.TokenRow) bool { return portfolio.MatchToken(tr, sel) }
			} else {
				filter = portfolio.FilterAll
			}

			rows, err := portfolio.ComputePnL(es, cg, addr, filter, deny)
			if err != nil {
				return err
			}
			if token != "" && len(rows) == 0 {
				return fmt.Errorf("no matching token with non-zero balance: %q (if its contract is listed in %s, remove that line to include it)", token, notknowntokens.FileName)
			}

			type scenario struct {
				BuyPriceUSD     float64 `json:"buy_price_usd"`
				TokensToBuy     float64 `json:"tokens_to_buy"`
				NewCostBasisUSD float64 `json:"new_cost_basis_usd,omitempty"`
				Error           string  `json:"error,omitempty"`
			}
			type rowOut struct {
				Symbol       string     `json:"symbol"`
				Contract     string     `json:"contract"`
				Balance      float64    `json:"balance"`
				SpotUSD      float64    `json:"spot_usd"`
				AvgCostUSD   float64    `json:"avg_cost_usd"`
				TargetAvgUSD float64    `json:"target_avg_usd"`
				Scenarios    []scenario `json:"scenarios"`
				SkipReason   string     `json:"skip_reason,omitempty"`
			}
			var payload struct {
				Address string   `json:"address"`
				Rows    []rowOut `json:"rows"`
			}
			payload.Address = addr

			for _, r := range rows {
				ro := rowOut{
					Symbol:     r.Symbol,
					Contract:   r.Contract,
					Balance:    r.BalanceHuman,
					SpotUSD:    r.SpotUSD,
					AvgCostUSD: r.AvgCostUSD,
				}
				if r.BalanceHuman <= 0 || r.SpotUSD <= 0 {
					ro.SkipReason = "missing spot or balance"
					payload.Rows = append(payload.Rows, ro)
					continue
				}
				if r.AvgCostUSD <= 0 {
					ro.SkipReason = r.HistoryWarning
					if ro.SkipReason == "" {
						ro.SkipReason = "no average cost computed"
					}
					payload.Rows = append(payload.Rows, ro)
					continue
				}
				T := targetAvg
				if T <= 0 {
					T = r.SpotUSD
				}
				ro.TargetAvgUSD = T
				if dca.AlreadyAtOrBelowTarget(r.AvgCostUSD, T) {
					ro.SkipReason = fmt.Sprintf("avg cost %.6f already at or below target %.6f", r.AvgCostUSD, T)
					payload.Rows = append(payload.Rows, ro)
					continue
				}
				for _, p := range prices {
					sc := scenario{BuyPriceUSD: p}
					x, err := dca.BuyToTargetAvg(r.BalanceHuman, r.AvgCostUSD, T, p)
					if err != nil {
						sc.Error = err.Error()
					} else {
						sc.TokensToBuy = x
						newQ := r.BalanceHuman + x
						newAvg := (r.BalanceHuman*r.AvgCostUSD + x*p) / newQ
						sc.NewCostBasisUSD = newQ * newAvg
					}
					ro.Scenarios = append(ro.Scenarios, sc)
				}
				payload.Rows = append(payload.Rows, ro)
			}

			if jsonOutput {
				return printJSON(payload)
			}
			tw := newTabWriter()
			for _, ro := range payload.Rows {
				if ro.SkipReason != "" {
					printfTable(tw, "%s\tSKIP: %s\n", ro.Symbol, ro.SkipReason)
					continue
				}
				printfTable(tw, "%s\tbalance=%.6f spot=$%.4f avg=$%.4f target_avg=$%.4f\n",
					ro.Symbol, ro.Balance, ro.SpotUSD, ro.AvgCostUSD, ro.TargetAvgUSD)
				for _, sc := range ro.Scenarios {
					if sc.Error != "" {
						printfTable(tw, "  buy@$%.4f\tERR %s\n", sc.BuyPriceUSD, sc.Error)
					} else {
						printfTable(tw, "  buy@$%.4f\tbuy %.6f tokens (est. new basis $%.2f)\n",
							sc.BuyPriceUSD, sc.TokensToBuy, sc.NewCostBasisUSD)
					}
				}
			}
			flushTable(tw)
			return nil
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Ethereum address (required)")
	cmd.Flags().StringVar(&token, "token", "", "Filter by contract 0x... or symbol (default: all tokens)")
	cmd.Flags().Float64Var(&targetAvg, "target-avg", -1, "Target average USD per token (-1 = use current spot)")
	cmd.Flags().StringVar(&buyPrices, "buy-price", "", "Comma-separated candidate limit prices (USD per token)")
	_ = cmd.MarkFlagRequired("address")
	return cmd
}

func parseFloatList(s string) ([]float64, error) {
	parts := strings.Split(s, ",")
	var out []float64
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("parse price %q: %w", p, err)
		}
		out = append(out, v)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no prices parsed")
	}
	return out, nil
}
