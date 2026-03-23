package notknowntokens

import (
	"path/filepath"
	"testing"
)

func TestLoadRecordDedup(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, FileName)
	m, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 0 {
		t.Fatalf("want empty, got %d", len(m))
	}
	addr := "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"
	if err := Record(p, addr); err != nil {
		t.Fatal(err)
	}
	if err := Record(p, addr); err != nil {
		t.Fatal(err)
	}
	m, err = Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 1 {
		t.Fatalf("want 1 entry, got %d", len(m))
	}
}
