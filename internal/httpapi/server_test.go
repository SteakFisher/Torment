package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"torment/internal/ledger"
)

// fakeLedger implements Ledger with per-test funcs; calling a method whose func is nil panics.
type fakeLedger struct {
	registerAsset   func(ctx context.Context, code string, l int32) (ledger.Asset, error)
	registerAddress func(ctx context.Context, p ledger.RegisterAddressParams) (ledger.Address, bool, error)
}

var _ Ledger = fakeLedger{}

func (f fakeLedger) RegisterAsset(ctx context.Context, code string, l int32) (ledger.Asset, error) {
	return f.registerAsset(ctx, code, l)
}

func (f fakeLedger) RegisterAddress(ctx context.Context, p ledger.RegisterAddressParams) (ledger.Address, bool, error) {
	return f.registerAddress(ctx, p)
}

func TestHealthz(t *testing.T) {
	rec := httptest.NewRecorder()
	New(nil).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
