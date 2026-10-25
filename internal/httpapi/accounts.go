package httpapi

import (
	"net/http"
	"time"

	"torment/internal/ledger"
)

// addressJSON is the response body for a registered address.
type addressJSON struct {
	Address       string    `json:"address"`
	Type          string    `json:"type"`
	AllowNegative bool      `json:"allow_negative"`
	CreatedAt     time.Time `json:"created_at"`
}

// postAccount handles POST /accounts. It responds 201 with the stored address, or 200 when the
// identical address was already registered.
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
		addr, created, err := l.RegisterAddress(r.Context(), ledger.RegisterAddressParams{
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
		writeJSON(w, status, addressJSON{
			Address:       addr.Address,
			Type:          string(addr.Type),
			AllowNegative: addr.AllowNegative,
			CreatedAt:     addr.CreatedAt,
		})
	}
}
