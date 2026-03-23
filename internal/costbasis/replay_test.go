package costbasis

import (
	"math/big"
	"testing"
	"time"

	"github.com/christian/crypto-avgr/internal/etherscan"
)

func TestReplayAverageCost_simpleBuys(t *testing.T) {
	user := "0xabc"
	// Two inbound 1 token each at $10 and $20 => avg $15
	transfers := []Transfer{
		{BlockNumber: 1, TxIndex: 0, Time: time.Unix(100, 0).UTC(), From: "0xother", To: user, ValueRaw: mustInt("1000000000000000000"), Decimals: 18},
		{BlockNumber: 2, TxIndex: 0, Time: time.Unix(200, 0).UTC(), From: "0xother", To: user, ValueRaw: mustInt("1000000000000000000"), Decimals: 18},
	}
	prices := map[int64]float64{100: 10, 200: 20}
	qty, avg, err := ReplayAverageCost(user, transfers, func(tm time.Time) (float64, error) {
		return prices[tm.Unix()], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := qty, 2.0; abs(got-want) > 1e-9 {
		t.Fatalf("qty got %v want %v", got, want)
	}
	if got, want := avg, 15.0; abs(got-want) > 1e-9 {
		t.Fatalf("avg got %v want %v", got, want)
	}
}

func TestReplayAverageCost_sellReducesQtyKeepsAvg(t *testing.T) {
	user := "0xabc"
	transfers := []Transfer{
		{BlockNumber: 1, TxIndex: 0, Time: time.Unix(1, 0).UTC(), From: "0xo", To: user, ValueRaw: mustInt("10"), Decimals: 0, ContractAddress: "0xt"},
		{BlockNumber: 2, TxIndex: 0, Time: time.Unix(2, 0).UTC(), From: user, To: "0xo", ValueRaw: mustInt("4"), Decimals: 0, ContractAddress: "0xt"},
	}
	prices := map[int64]float64{1: 5, 2: 999}
	qty, avg, err := ReplayAverageCost(user, transfers, func(tm time.Time) (float64, error) {
		return prices[tm.Unix()], nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := qty, 6.0; abs(got-want) > 1e-9 {
		t.Fatalf("qty got %v want %v", got, want)
	}
	if got, want := avg, 5.0; abs(got-want) > 1e-9 {
		t.Fatalf("avg got %v want %v", got, want)
	}
}

func TestFromEtherscan_normalizesAddresses(t *testing.T) {
	user := "0xAbC"
	rows := []etherscan.TokenTransfer{
		{
			BlockNumber: "1", TimeStamp: "10", From: "0xOTHER", To: "0xabc",
			ContractAddress: "0xT", Value: "1", TokenDecimal: "0", TokenSymbol: "TKN", TransactionIndex: "0",
		},
	}
	tr, err := FromEtherscan(rows, user)
	if err != nil {
		t.Fatal(err)
	}
	if len(tr) != 1 || tr[0].From != "0xother" || tr[0].To != "0xabc" {
		t.Fatalf("got %+v", tr)
	}
}

func mustInt(s string) *big.Int {
	n := new(big.Int)
	if _, ok := n.SetString(s, 10); !ok {
		panic(s)
	}
	return n
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
