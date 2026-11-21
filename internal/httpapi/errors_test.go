package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"torment/internal/ledger"
)

func TestWriteError(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantBody   string
	}{
		{name: "decode", err: ledger.Errorf(ledger.ErrInvalid, "bad json"), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"bad json"}}`},
		{name: "invalid", err: ledger.Errorf(ledger.ErrInvalid, "x"), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"x"}}`},
		{name: "batch too large", err: ledger.Errorf(ledger.ErrBatchTooLarge, "x"), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"batch_too_large","message":"x"}}`},
		{name: "pattern too broad", err: ledger.Errorf(ledger.ErrPatternTooBroad, "x"), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"pattern_too_broad","message":"x"}}`},
		{name: "account not found", err: ledger.Errorf(ledger.ErrAccountNotFound, "x"), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"account_not_found","message":"x"}}`},
		{name: "currency not found", err: ledger.Errorf(ledger.ErrCurrencyNotFound, "x"), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"currency_not_found","message":"x"}}`},
		{name: "kind not found", err: ledger.Errorf(ledger.ErrKindNotFound, "x"), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"kind_not_found","message":"x"}}`},
		{name: "conflict", err: ledger.Errorf(ledger.ErrConflict, "x"), wantStatus: http.StatusConflict, wantBody: `{"error":{"code":"conflict","message":"x"}}`},
		{name: "invalid inside a wrap", err: fmt.Errorf("chain 0: %w", ledger.Errorf(ledger.ErrInvalid, "x")), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"chain 0: x"}}`},
		{name: "code with no status", err: ledger.Errorf(ledger.ErrInsufficientFunds, "x"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
		{name: "unexpected", err: errors.New("boom"), wantStatus: http.StatusInternalServerError, wantBody: `{"error":{"code":"internal","message":"internal error"}}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeError(rec, httptest.NewRequest(http.MethodGet, "/", nil), tt.err)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if got := strings.TrimSpace(rec.Body.String()); got != tt.wantBody {
				t.Fatalf("body = %s, want %s", got, tt.wantBody)
			}
		})
	}
}
