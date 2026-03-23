package costbasis

import (
	"fmt"
	"math/big"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/christian/crypto-avgr/internal/etherscan"
)

// Transfer is a normalized ERC-20 transfer for replay.
type Transfer struct {
	BlockNumber     uint64
	TxIndex         uint64
	Time            time.Time
	From            string
	To              string
	ValueRaw        *big.Int
	Decimals        int
	ContractAddress string
	TokenSymbol     string
}

// SortTransfers sorts ascending by block, then transaction index.
func SortTransfers(t []Transfer) {
	sort.Slice(t, func(i, j int) bool {
		if t[i].BlockNumber != t[j].BlockNumber {
			return t[i].BlockNumber < t[j].BlockNumber
		}
		return t[i].TxIndex < t[j].TxIndex
	})
}

// FromEtherscan converts Etherscan rows for one contract into Transfer slice (both directions involving user).
func FromEtherscan(rows []etherscan.TokenTransfer, userAddress string) ([]Transfer, error) {
	user := strings.ToLower(userAddress)
	var out []Transfer
	for _, r := range rows {
		from := strings.ToLower(r.From)
		to := strings.ToLower(r.To)
		if from != user && to != user {
			continue
		}
		dec, err := strconv.Atoi(strings.TrimSpace(r.TokenDecimal))
		if err != nil || dec < 0 || dec > 36 {
			return nil, fmt.Errorf("token %s: bad decimals %q", r.ContractAddress, r.TokenDecimal)
		}
		v := new(big.Int)
		if _, ok := v.SetString(strings.TrimSpace(r.Value), 10); !ok {
			return nil, fmt.Errorf("token %s: bad value %q", r.ContractAddress, r.Value)
		}
		bn, err := strconv.ParseUint(strings.TrimSpace(r.BlockNumber), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad blockNumber: %w", err)
		}
		txi, _ := strconv.ParseUint(strings.TrimSpace(r.TransactionIndex), 10, 64)
		ts, err := parseUnix(r.TimeStamp)
		if err != nil {
			return nil, err
		}
		out = append(out, Transfer{
			BlockNumber:     bn,
			TxIndex:         txi,
			Time:            ts,
			From:            from,
			To:              to,
			ValueRaw:        v,
			Decimals:        dec,
			ContractAddress: strings.ToLower(r.ContractAddress),
			TokenSymbol:     r.TokenSymbol,
		})
	}
	return out, nil
}

func parseUnix(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	u, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("timestamp: %w", err)
	}
	return time.Unix(u, 0).UTC(), nil
}

// HumanAmount converts raw token units to float64 human amount (may lose precision for very large values).
func HumanAmount(raw *big.Int, decimals int) float64 {
	if raw == nil || raw.Sign() == 0 {
		return 0
	}
	den := pow10Big(decimals)
	r := new(big.Rat).SetFrac(raw, den)
	f, _ := r.Float64()
	return f
}

func pow10Big(n int) *big.Int {
	ten := big.NewInt(10)
	p := big.NewInt(1)
	for i := 0; i < n; i++ {
		p.Mul(p, ten)
	}
	return p
}

// ReplayAverageCost applies average-cost rules with USD price at each inbound transfer.
// priceAt returns USD per 1 token at transfer time t (caller typically uses daily history).
func ReplayAverageCost(user string, transfers []Transfer, priceAt func(t time.Time) (float64, error)) (qty float64, avgUSD float64, err error) {
	user = strings.ToLower(user)
	SortTransfers(transfers)
	var q, a float64
	for _, tr := range transfers {
		amt := HumanAmount(tr.ValueRaw, tr.Decimals)
		if amt <= 0 {
			continue
		}
		switch {
		case tr.To == user:
			px, e := priceAt(tr.Time)
			if e != nil {
				return 0, 0, e
			}
			if q == 0 {
				q = amt
				a = px
				continue
			}
			newQ := q + amt
			a = (q*a + amt*px) / newQ
			q = newQ
		case tr.From == user:
			if amt >= q {
				q = 0
				a = 0
			} else {
				q -= amt
			}
		default:
			// skip
		}
	}
	return q, a, nil
}
