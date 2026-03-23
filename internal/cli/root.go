package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	etherscanAPIKey string
	coingeckoAPIKey string
	jsonOutput      bool
)

func Execute() error {
	rootCmd := newRootCmd()
	return rootCmd.Execute()
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "crypto-avgr",
		Short: "Ethereum wallet token balances, P/L, and DCA suggestions",
		Long:  "Reads ERC-20 holdings via Etherscan, prices via CoinGecko, and estimates average cost from transfer history.",
	}

	root.PersistentFlags().StringVar(&etherscanAPIKey, "etherscan-api-key", "", "Etherscan API key (or ETHERSCAN_API_KEY)")
	root.PersistentFlags().StringVar(&coingeckoAPIKey, "coingecko-api-key", "", "CoinGecko API key for higher rate limits (or COINGECKO_API_KEY)")
	root.PersistentFlags().BoolVar(&jsonOutput, "json", false, "Print machine-readable JSON")

	root.AddCommand(newListCmd(), newPnLCmd(), newDCACmd())

	return root
}

func loadAPIKeys() {
	if etherscanAPIKey == "" {
		etherscanAPIKey = os.Getenv("ETHERSCAN_API_KEY")
	}
	if coingeckoAPIKey == "" {
		coingeckoAPIKey = os.Getenv("COINGECKO_API_KEY")
	}
}

func requireEtherscanKey() error {
	loadAPIKeys()
	if etherscanAPIKey == "" {
		return fmt.Errorf("Etherscan API key required: use --etherscan-api-key or set ETHERSCAN_API_KEY")
	}
	return nil
}
