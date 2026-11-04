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
		at, err := parseAt(r)
		if err != nil {
			writeError(w, err)
			return
		}
		balances, err := l.Balances(r.Context(), address, at)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, struct {
			Address  string        `json:"address"`
			Balances []balanceJSON `json:"balances"`
		}{address, newBalancesJSON(balances)})
	}
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

// getPatternBalances handles GET /balances?address=&currency=&at=. It responds 200 with one total per
// account type and currency plus each matching account's balances, or 400 for a malformed query or a
// pattern matching too many wallets.
func getPatternBalances(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		at, err := parseAt(r)
		if err != nil {
			writeError(w, err)
			return
		}
		q := r.URL.Query()
		totals, accounts, err := l.PatternBalances(r.Context(), q.Get("address"), q.Get("currency"), at)
		if err != nil {
			writeError(w, err)
			return
		}
		out := struct {
			Address  string                `json:"address"`
			Totals   []totalJSON           `json:"totals"`
			Accounts []accountBalancesJSON `json:"accounts"`
		}{q.Get("address"), make([]totalJSON, 0, len(totals)), make([]accountBalancesJSON, 0, len(accounts))}
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

// parseAt returns the request's at query parameter, or the zero time when it is absent. It returns
// ledger.ErrInvalid when at is not an RFC 3339 time.
func parseAt(r *http.Request) (time.Time, error) {
	s := r.URL.Query().Get("at")
	if s == "" {
		return time.Time{}, nil
	}
	at, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: at %q must be an RFC 3339 time", ledger.ErrInvalid, s)
	}
	return at, nil
}
