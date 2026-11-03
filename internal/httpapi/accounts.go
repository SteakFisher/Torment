package httpapi

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"torment/internal/ledger"
)

// accountJSON is an account and its wallets.
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
		writeJSON(w, status, newAccountJSON(acct))
	}
}

// getAccounts handles GET /accounts?address=&after=&limit=. It responds 200 with the matching accounts
// ordered by address, and next set to the last address when more match, or 400 for a malformed query.
func getAccounts(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		limit := 0
		if s := q.Get("limit"); s != "" {
			var err error
			if limit, err = strconv.Atoi(s); err != nil {
				writeError(w, fmt.Errorf("%w: limit %q must be an integer", ledger.ErrInvalid, s))
				return
			}
		}
		accounts, more, err := l.ListAccounts(r.Context(), q.Get("address"), q.Get("after"), limit)
		if err != nil {
			writeError(w, err)
			return
		}
		out := struct {
			Accounts []accountJSON `json:"accounts"`
			Next     string        `json:"next,omitempty"`
		}{Accounts: make([]accountJSON, 0, len(accounts))}
		for _, a := range accounts {
			out.Accounts = append(out.Accounts, newAccountJSON(a))
		}
		if more {
			out.Next = accounts[len(accounts)-1].Address
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func newAccountJSON(acct ledger.Account) accountJSON {
	wallets := make([]walletJSON, 0, len(acct.Wallets))
	for _, wl := range acct.Wallets {
		wallets = append(wallets, walletJSON{Currency: wl.Currency, Precision: wl.Precision})
	}
	return accountJSON{
		Address:       acct.Address,
		Type:          string(acct.Type),
		AllowNegative: acct.AllowNegative,
		CreatedAt:     acct.CreatedAt,
		Wallets:       wallets,
	}
}
