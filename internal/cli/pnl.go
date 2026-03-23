package cli

import (
	"fmt"

	"github.com/christian/crypto-avgr/internal/coingecko"
	"github.com/christian/crypto-avgr/internal/etherscan"
	"github.com/christian/crypto-avgr/internal/notknowntokens"
	"github.com/christian/crypto-avgr/internal/portfolio"
	"github.com/christian/crypto-avgr/internal/wallet"
	"github.com/spf13/cobra"
)

func newPnLCmd() *cobra.Command {
	var address, token string
	cmd := &cobra.Command{
		Use:   "pnl",
		Short: "Estimated average buy and unrealized P/L from transfer history",
		Long:  "Cost basis is a heuristic from ERC-20 transfers (average-cost model). Not tax advice. Skips contracts listed in .notknowntokens in the working directory (see list --hide-unlisted).",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireEtherscanKey(); err != nil {
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
				filter = func(tr portfolio.TokenRow) bool {
					return portfolio.MatchToken(tr, sel)
				}
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

			type pnlOut struct {
				Address string             `json:"address"`
				Rows    []portfolio.PnLRow `json:"rows"`
				Totals  struct {
					ValueUSD      float64 `json:"value_usd"`
					CostBasisUSD  float64 `json:"cost_basis_usd"`
					UnrealizedUSD float64 `json:"unrealized_pnl_usd"`
					UnrealizedPct float64 `json:"unrealized_pnl_pct_weighted"`
				} `json:"totals"`
			}
			var out pnlOut
			out.Address = addr
			out.Rows = rows
			var sumV, sumC float64
			for _, r := range rows {
				if r.HistoryWarning != "" && r.AvgCostUSD == 0 {
					continue
				}
				sumV += r.ValueUSD
				sumC += r.CostBasisUSD
			}
			out.Totals.ValueUSD = sumV
			out.Totals.CostBasisUSD = sumC
			out.Totals.UnrealizedUSD = sumV - sumC
			if sumC != 0 {
				out.Totals.UnrealizedPct = (out.Totals.UnrealizedUSD / sumC) * 100
			}

			if jsonOutput {
				return printJSON(out)
			}
			tw := newTabWriter()
			printfTable(tw, "SYMBOL\tBALANCE\tSPOT\tAVG_BUY\tVALUE\tCOST_BASIS\tP/L\tP/L%%\tNOTE\n")
			for _, r := range rows {
				note := r.HistoryWarning
				if len(note) > 40 {
					note = note[:37] + "..."
				}
				printfTable(tw, "%s\t%.6f\t$%.4f\t$%.4f\t$%.2f\t$%.2f\t$%.2f\t%.2f%%\t%s\n",
					r.Symbol, r.BalanceHuman, r.SpotUSD, r.AvgCostUSD, r.ValueUSD, r.CostBasisUSD,
					r.UnrealizedUSD, r.UnrealizedPct, note)
			}
			flushTable(tw)
			fmt.Printf("\nTOTAL  value $%.2f  cost $%.2f  unrealized $%.2f (%.2f%% on cost)\n",
				out.Totals.ValueUSD, out.Totals.CostBasisUSD, out.Totals.UnrealizedUSD, out.Totals.UnrealizedPct)
			return nil
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Ethereum address (required)")
	cmd.Flags().StringVar(&token, "token", "", "Filter by contract 0x... or symbol (default: all tokens)")
	_ = cmd.MarkFlagRequired("address")
	return cmd
}
