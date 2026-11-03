package httpapi

import (
	"fmt"
	"net/http"
	"time"

	"torment/internal/ledger"
)

// balanceJSON is an account's balance in one currency. Amount is in minor units as a decimal string.
type balanceJSON struct {
	Currency  string `json:"currency"`
	Precision int16  `json:"precision"`
	Amount    string `json:"amount"`
}

// getBalances handles GET /accounts/{address}/balances?at=. It responds 200 with the balance of each of
// the account's wallets, as of at when given, 400 when at is not an RFC 3339 time, or 404 when the
// account does not exist.
func getBalances(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := r.PathValue("address")
		var at time.Time
		if s := r.URL.Query().Get("at"); s != "" {
			var err error
			if at, err = time.Parse(time.RFC3339Nano, s); err != nil {
				writeError(w, fmt.Errorf("%w: at %q must be an RFC 3339 time", ledger.ErrInvalid, s))
				return
			}
		}
		balances, err := l.Balances(r.Context(), address, at)
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]balanceJSON, 0, len(balances))
		for _, b := range balances {
			out = append(out, balanceJSON{Currency: b.Currency, Precision: b.Precision, Amount: b.Amount.String()})
		}
		writeJSON(w, http.StatusOK, struct {
			Address  string        `json:"address"`
			Balances []balanceJSON `json:"balances"`
		}{address, out})
	}
}
