package dca

import (
	"math"
	"testing"
)

func TestBuyToTargetAvg_lowersAverage(t *testing.T) {
	// B=10, A=2, T=1.5, p=1 => x = 10*(2-1.5)/(1.5-1) = 10
	x, err := BuyToTargetAvg(10, 2, 1.5, 1)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(x-10) > 1e-9 {
		t.Fatalf("got %v", x)
	}
	newAvg := (10*2 + x*1) / (10 + x)
	if math.Abs(newAvg-1.5) > 1e-9 {
		t.Fatalf("newAvg %v", newAvg)
	}
}

func TestBuyToTargetAvg_invalidSign(t *testing.T) {
	_, err := BuyToTargetAvg(10, 2, 1.5, 2)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestAlreadyAtOrBelowTarget(t *testing.T) {
	if !AlreadyAtOrBelowTarget(1, 2) {
		t.Fatal()
	}
	if AlreadyAtOrBelowTarget(2, 1) {
		t.Fatal()
	}
}
