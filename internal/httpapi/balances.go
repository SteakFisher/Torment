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

// totalJSON is the sum of one account type's balances in one currency. Amount is a decimal string.
type totalJSON struct {
	Type      string `json:"type"`
	Currency  string `json:"currency"`
	Precision int16  `json:"precision"`
	Amount    string `json:"amount"`
}

// accountBalancesJSON is one account's balances.
type accountBalancesJSON struct {
	Address  string        `json:"address"`
	Type     string        `json:"type"`
	Balances []balanceJSON `json:"balances"`
}

// getBalances handles GET /accounts/{address}/balances?currency=&at=, with balances only in currency when
// given and as of at when given. It responds 200 with one total per account type and currency plus the
// balances of the account, or of each account matching an address pattern; 400 for a malformed query or
// a pattern matching too many wallets; or 404 when the exact account does not exist.
func getBalances(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address, currency := r.PathValue("address"), r.URL.Query().Get("currency")
		at, err := parseTime(r, "at")
		if err != nil {
			writeError(w, r, err)
			return
		}
		totals, accounts, err := l.Balances(r.Context(), address, currency, at)
		if err != nil {
			writeError(w, r, err)
			return
		}
		out := struct {
			Address  string                `json:"address"`
			Totals   []totalJSON           `json:"totals"`
			Accounts []accountBalancesJSON `json:"accounts"`
		}{address, make([]totalJSON, 0, len(totals)), make([]accountBalancesJSON, 0, len(accounts))}
		for _, t := range totals {
			out.Totals = append(out.Totals, totalJSON{Type: string(t.Type), Currency: t.Currency, Precision: t.Precision, Amount: t.Amount.String()})
		}
		for _, a := range accounts {
			out.Accounts = append(out.Accounts, accountBalancesJSON{Address: a.Address, Type: string(a.Type), Balances: newBalancesJSON(a.Balances)})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func newBalancesJSON(balances []ledger.Balance) []balanceJSON {
	out := make([]balanceJSON, 0, len(balances))
	for _, b := range balances {
		out = append(out, balanceJSON{Currency: b.Currency, Precision: b.Precision, Amount: b.Amount.String()})
	}
	return out
}

// parseTime returns the request's query parameter name as a time, or the zero time when it is absent. It
// returns ledger.ErrInvalid when the value is not an RFC 3339 time.
func parseTime(r *http.Request, name string) (time.Time, error) {
	s := r.URL.Query().Get(name)
	if s == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: %s %q must be an RFC 3339 time", ledger.ErrInvalid, name, s)
	}
	return t, nil
}
