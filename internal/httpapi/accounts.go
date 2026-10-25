package httpapi

import (
	"net/http"
	"time"

	"torment/internal/ledger"
)

// accountJSON is the response body for a registered account.
type accountJSON struct {
	Address       string    `json:"address"`
	Type          string    `json:"type"`
	AllowNegative bool      `json:"allow_negative"`
	CreatedAt     time.Time `json:"created_at"`
}

// postAccount handles POST /accounts. It responds 201 with the stored account, or 200 when the
// identical account was already registered.
func postAccount(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address       string `json:"address"`
			Type          string `json:"type"`
			AllowNegative bool   `json:"allow_negative"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		acct, created, err := l.RegisterAccount(r.Context(), ledger.RegisterAccountParams{
			Address:       req.Address,
			Type:          req.Type,
			AllowNegative: req.AllowNegative,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, accountJSON{
			Address:       acct.Address,
			Type:          string(acct.Type),
			AllowNegative: acct.AllowNegative,
			CreatedAt:     acct.CreatedAt,
		})
	}
}
