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
		{name: "decode", err: decodeError{errors.New("bad json")}, wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"bad json"}}`},
		{name: "invalid", err: fmt.Errorf("%w: x", ledger.ErrInvalid), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"x"}}`},
		{name: "batch too large", err: fmt.Errorf("%w: x", ledger.ErrBatchTooLarge), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"batch_too_large","message":"x"}}`},
		{name: "pattern too broad", err: fmt.Errorf("%w: x", ledger.ErrPatternTooBroad), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"pattern_too_broad","message":"x"}}`},
		{name: "account not found", err: fmt.Errorf("%w: x", ledger.ErrAccountNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"account_not_found","message":"x"}}`},
		{name: "currency not found", err: fmt.Errorf("%w: x", ledger.ErrCurrencyNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"currency_not_found","message":"x"}}`},
		{name: "kind not found", err: fmt.Errorf("%w: x", ledger.ErrKindNotFound), wantStatus: http.StatusNotFound, wantBody: `{"error":{"code":"kind_not_found","message":"x"}}`},
		{name: "conflict", err: fmt.Errorf("%w: x", ledger.ErrConflict), wantStatus: http.StatusConflict, wantBody: `{"error":{"code":"conflict","message":"x"}}`},
		{name: "invalid inside a wrap", err: fmt.Errorf("chain 0: %w", fmt.Errorf("%w: x", ledger.ErrInvalid)), wantStatus: http.StatusBadRequest, wantBody: `{"error":{"code":"invalid_request","message":"chain 0: x"}}`},
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
