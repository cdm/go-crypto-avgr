package cli

import (
	"fmt"
	"strings"

	"github.com/christian/crypto-avgr/internal/coingecko"
	"github.com/christian/crypto-avgr/internal/etherscan"
	"github.com/christian/crypto-avgr/internal/portfolio"
	"github.com/christian/crypto-avgr/internal/wallet"
	"github.com/spf13/cobra"
)

func newListCmd() *cobra.Command {
	var address string
	var noAvgCost bool
	var hideUnlisted bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List ERC-20 holdings and native ETH with spot USD values",
		Long:  "By default includes estimated average buy (USD per token) using the same transfer replay as pnl; use --no-avg-cost for a faster run without historical CoinGecko calls. CoinGecko-unlisted contracts are hidden by default and appended to .notknowntokens in the current directory so list, pnl, and dca skip them next time.",
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
			snap, err := portfolio.BuildSnapshot(es, cg, addr, portfolio.SnapshotOptions{
				IncludeAvgCost:  !noAvgCost,
				HideUnlisted:    hideUnlisted,
				DenylistOptions: deny,
			})
			if err != nil {
				return err
			}
			if jsonOutput {
				return printJSON(snap)
			}
			tw := newTabWriter()
			printfTable(tw, "ADDRESS\t%s\n", snap.Address)
			flushTable(tw)
			if snap.Native != nil {
				tw = newTabWriter()
				printfTable(tw, "ASSET\tBALANCE\tSPOT_USD\tAVG_COST_USD\tVALUE_USD\t24H_%%\n")
				printfTable(tw, "ETH\t%.6f\t$%.4f\t-\t$%.2f\t%.2f%%\n",
					snap.Native.BalanceHuman, snap.Native.SpotUSD, snap.Native.ValueUSD, snap.Native.Change24hPct)
				flushTable(tw)
			}
			tw = newTabWriter()
			printfTable(tw, "SYMBOL\tCONTRACT\tBALANCE\tSPOT_USD\tAVG_COST_USD\tVALUE_USD\t24H_%%\tNOTE\n")
			var unlisted int
			for _, t := range snap.Tokens {
				note := t.PriceNote
				if t.AvgCostNote != "" {
					if note != "" {
						note += "; "
					}
					note += t.AvgCostNote
				}
				if strings.Contains(t.PriceNote, "not listed on CoinGecko") {
					unlisted++
				}
				if len(note) > 56 {
					note = note[:53] + "..."
				}
				avgStr := "-"
				if t.AvgCostUSD > 0 {
					avgStr = fmt.Sprintf("$%.6f", t.AvgCostUSD)
				}
				printfTable(tw, "%s\t%s\t%.6f\t$%.6f\t%s\t$%.2f\t%.2f%%\t%s\n",
					t.Symbol, t.Contract, t.BalanceHuman, t.SpotUSD, avgStr, t.ValueUSD, t.Change24hPct, note)
			}
			flushTable(tw)
			if len(snap.Tokens) == 0 {
				fmt.Println("(no non-zero ERC-20 balances from indexed transfers)")
			} else if unlisted > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "\n%d token(s) have no CoinGecko listing (HTTP 404 on contract lookup); USD price is unavailable. This is normal for small or new tokens.\n", unlisted)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&address, "address", "", "Ethereum address (required)")
	cmd.Flags().BoolVar(&noAvgCost, "no-avg-cost", false, "Skip average buy estimate (faster; no extra CoinGecko history calls)")
	cmd.Flags().BoolVar(&hideUnlisted, "hide-unlisted", true, "Hide tokens not on CoinGecko; record their contract addresses in .notknowntokens")
	_ = cmd.MarkFlagRequired("address")
	return cmd
}
