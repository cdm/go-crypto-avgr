package notknowntokens

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileName is the default denylist file in the working directory.
const FileName = ".notknowntokens"

var recordMu sync.Mutex

// DefaultPath returns filepath.Join(os.Getwd(), FileName).
func DefaultPath() string {
	wd, err := os.Getwd()
	if err != nil || wd == "" {
		wd = "."
	}
	return filepath.Join(wd, FileName)
}

// Load reads path line-by-line into a set of lower-case 0x addresses. Missing file => empty set, no error.
func Load(path string) (map[string]struct{}, error) {
	out := make(map[string]struct{})
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		addr, err := NormalizeAddress(line)
		if err != nil {
			continue
		}
		out[addr] = struct{}{}
	}
	return out, sc.Err()
}

// NormalizeAddress returns lower-case 0x + 40 hex or an error.
func NormalizeAddress(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "0x") {
		s = "0x" + s
	}
	raw := strings.TrimPrefix(s, "0x")
	if len(raw) != 40 {
		return "", fmt.Errorf("invalid address length")
	}
	if _, err := hex.DecodeString(raw); err != nil {
		return "", err
	}
	return strings.ToLower(s), nil
}

// Record appends contract to path if not already present (line per address).
func Record(path, contract string) error {
	if path == "" {
		return nil
	}
	addr, err := NormalizeAddress(contract)
	if err != nil {
		return fmt.Errorf("notknowntokens: %w", err)
	}
	recordMu.Lock()
	defer recordMu.Unlock()

	existing, err := Load(path)
	if err != nil {
		return err
	}
	if _, ok := existing[addr]; ok {
		return nil
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("notknowntokens write %s: %w", path, err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, addr); err != nil {
		return err
	}
	return nil
}
