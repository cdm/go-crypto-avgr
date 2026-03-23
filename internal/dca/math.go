package dca

import (
	"fmt"
	"math"
)

// BuyToTargetAvg returns how many tokens to buy at buyPricePerToken (USD)
// so that the new volume-weighted average equals targetAvgUSD.
// B = current balance (tokens), A = current avg cost (USD/token), T = target avg, p = buy price.
// x = B * (A - T) / (T - p)
func BuyToTargetAvg(balance, avgCostUSD, targetAvgUSD, buyPricePerToken float64) (float64, error) {
	if balance <= 0 {
		return 0, fmt.Errorf("balance must be positive")
	}
	if buyPricePerToken <= 0 || targetAvgUSD <= 0 {
		return 0, fmt.Errorf("buy price and target avg must be positive")
	}
	den := targetAvgUSD - buyPricePerToken
	if math.Abs(den) < 1e-18 {
		return 0, fmt.Errorf("buy price equals target avg: no unique solution")
	}
	num := balance * (avgCostUSD - targetAvgUSD)
	x := num / den
	if x < 0 {
		return 0, fmt.Errorf("no positive buy size: need (target-buyPrice) and (avg-target) same sign; got avg=%.6f target=%.6f buy=%.6f", avgCostUSD, targetAvgUSD, buyPricePerToken)
	}
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return 0, fmt.Errorf("invalid numeric result")
	}
	return x, nil
}

// AlreadyAtOrBelowTarget returns true if avg <= target (for "average down to spot" when underwater).
func AlreadyAtOrBelowTarget(avgCostUSD, targetAvgUSD float64) bool {
	return avgCostUSD <= targetAvgUSD+1e-12
}
