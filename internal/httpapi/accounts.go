package httpapi

import (
	"net/http"
	"time"

	"torment/internal/ledger"
)

// accountJSON is the response body for a registered account.
type accountJSON struct {
	Address       string       `json:"address"`
	Type          string       `json:"type"`
	AllowNegative bool         `json:"allow_negative"`
	CreatedAt     time.Time    `json:"created_at"`
	Wallets       []walletJSON `json:"wallets"`
}

// walletJSON is one wallet of an account.
type walletJSON struct {
	Currency  string `json:"currency"`
	Precision int16  `json:"precision"`
}

// postAccount handles POST /accounts. It responds 201 with the stored account and its wallets in the
// requested currencies, or 200 when the identical account was already registered.
func postAccount(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Address       string   `json:"address"`
			Type          string   `json:"type"`
			AllowNegative bool     `json:"allow_negative"`
			Currencies    []string `json:"currencies"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		acct, created, err := l.RegisterAccount(r.Context(), ledger.RegisterAccountParams{
			Address:       req.Address,
			Type:          req.Type,
			AllowNegative: req.AllowNegative,
			Currencies:    req.Currencies,
		})
		if err != nil {
			writeError(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		wallets := make([]walletJSON, 0, len(acct.Wallets))
		for _, wl := range acct.Wallets {
			wallets = append(wallets, walletJSON{Currency: wl.Currency, Precision: wl.Precision})
		}
		writeJSON(w, status, accountJSON{
			Address:       acct.Address,
			Type:          string(acct.Type),
			AllowNegative: acct.AllowNegative,
			CreatedAt:     acct.CreatedAt,
			Wallets:       wallets,
		})
	}
}
