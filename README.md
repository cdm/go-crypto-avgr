# go-crypto-avgr

Command-line tool for **Ethereum mainnet** wallets: list ERC-20 holdings (via [Etherscan](https://docs.etherscan.io/)), price them with [CoinGecko](https://docs.coingecko.com/), estimate **average buy** and **unrealized P/L** from ERC-20 transfer history (average-cost heuristic), and suggest **DCA buy sizes** to reach a target average (default: current spot).

**Disclaimer:** On-chain transfer history is not the same as exchange trade history. Incoming transfers are treated as acquisitions at the token’s CoinGecko **daily** USD price (UTC date). This is **not tax advice** and can be wrong for airdrops, bridges, internal transfers, and unlisted tokens.

## Requirements

- Go 1.18+
- Free **Etherscan API key** ([create one](https://etherscan.io/myapikey))
- Optional **CoinGecko demo API key** ([CoinGecko API](https://docs.coingecko.com/reference/introduction)) — improves rate limits; the CLI still works without it but may hit throttling on large histories.

## Build

```bash
cd crypto-avgr
go build -o crypto-avgr ./cmd/crypto-avgr
```

## Configuration

- `ETHERSCAN_API_KEY` or `--etherscan-api-key`
- `COINGECKO_API_KEY` or `--coingecko-api-key` (optional; sent as `x-cg-demo-api-key`)

## Usage

```bash
# Holdings + spot USD + estimated avg buy (USD/token, same model as pnl; slower — many CoinGecko history calls)
./crypto-avgr list --address 0xYourAddress

# Fast list: balances and spot only (skip average buy / history)
./crypto-avgr list --address 0xYourAddress --no-avg-cost

# Show CoinGecko-unlisted junk tokens in the table (default hides them and logs contracts in ./.notknowntokens)
./crypto-avgr list --address 0xYourAddress --hide-unlisted=false

# P/L and estimated average buy (all tokens with non-zero balance)
./crypto-avgr pnl --address 0xYourAddress

# One token by symbol or contract
./crypto-avgr pnl --address 0xYourAddress --token USDC
./crypto-avgr pnl --address 0xYourAddress --token 0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48

# DCA: how many tokens to buy at given limit prices to reach target average (default target = spot)
./crypto-avgr dca --address 0xYourAddress --buy-price 1800,1700,1600
./crypto-avgr dca --address 0xYourAddress --token MKR --target-avg 1500 --buy-price 1400,1300

# JSON output
./crypto-avgr list --address 0xYourAddress --json
```

## Limits

- **Etherscan** free tier is capped (commonly **3 calls/sec**). The client spaces requests (~400ms apart) and retries with backoff if the API still returns a rate-limit message.
- Token discovery uses **ERC-20 `tokentx` only**; tokens never transferred in the indexer’s history won’t appear.
- **`.notknowntokens`** in the **current working directory** lists contract addresses to skip on every command (`list`, `pnl`, `dca`). New CoinGecko-unlisted contracts are appended when discovered. By default, `list --hide-unlisted` (default **true**) hides those tokens from output but still records them. Delete a line from the file to include that token again.
- CoinGecko only lists a subset of ERC-20s. **HTTP 404** on `/coins/ethereum/contract/{address}` means “no listing,” not a bad API key. The CLI marks those rows as **not listed** and skips USD pricing (and uses a shorter delay so `list` finishes faster for wallets with many junk tokens).
- CoinGecko free tier: the code spaces requests (~1.1s after successful price lookups); large wallets are slow.
- Native **ETH** cost basis / P/L is not implemented (only balance and spot on `list`).

## License

MIT
