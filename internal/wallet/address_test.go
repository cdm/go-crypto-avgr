package wallet

import "testing"

func TestNormalizeAddress(t *testing.T) {
	got, err := NormalizeAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48")
	if err != nil {
		t.Fatal(err)
	}
	want := "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestNormalizeAddress_invalid(t *testing.T) {
	_, err := NormalizeAddress("not-an-address")
	if err == nil {
		t.Fatal("expected error")
	}
}
