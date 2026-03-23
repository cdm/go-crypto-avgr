package wallet

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// NormalizeAddress returns a lowercase 0x-prefixed 20-byte hex address or an error.
func NormalizeAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "0x") && !strings.HasPrefix(s, "0X") {
		return "", fmt.Errorf("address must start with 0x")
	}
	s = "0x" + s[2:]
	raw := s[2:]
	if len(raw) != 40 {
		return "", fmt.Errorf("address must be 40 hex characters after 0x")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", fmt.Errorf("invalid hex: %w", err)
	}
	return strings.ToLower(s), nil
}
