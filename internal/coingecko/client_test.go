package coingecko

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetByContract_404NotListed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/coins/ethereum/contract/0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef" {
			t.Fatalf("path %s", r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient("")
	c.BaseURL = srv.URL + "/api/v3"
	_, err := c.GetByContract("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if !errors.Is(err, ErrNotListed) {
		t.Fatalf("want ErrNotListed, got %v", err)
	}
	// negative cache: second call should not hit server
	_, err = c.GetByContract("0xdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef")
	if !errors.Is(err, ErrNotListed) {
		t.Fatalf("second call: want ErrNotListed, got %v", err)
	}
}

func TestGetJSON_history404NotErrNotListed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	c := NewClient("")
	c.BaseURL = srv.URL + "/api/v3"
	var v struct{}
	err := c.getJSON(srv.URL+"/api/v3/coins/foo/history?date=01-01-2020", &v, nil)
	if errors.Is(err, ErrNotListed) {
		t.Fatal("history 404 must not use ErrNotListed")
	}
	if err == nil {
		t.Fatal("expected error")
	}
}
