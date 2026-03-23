package main

import (
	"os"

	"github.com/christian/crypto-avgr/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
