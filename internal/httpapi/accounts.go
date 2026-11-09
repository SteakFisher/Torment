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
			writeError(w, r, err)
			return
		}
		acct, created, err := l.RegisterAccount(r.Context(), ledger.RegisterAccountParams{
			Address:       req.Address,
			Type:          req.Type,
			AllowNegative: req.AllowNegative,
			Currencies:    req.Currencies,
		})
		if err != nil {
			writeError(w, r, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, newAccountJSON(acct))
	}
}

// getAccounts handles GET /accounts?cursor=&limit= and GET /accounts/{address}. It responds 200 with the
// accounts matching the address pattern, or every account when there is none, ordered by address, and
// next set to the last address when more match; or 400 for a malformed query. For an exact address the
// accounts are that one account, or it responds 404 when it does not exist.
func getAccounts(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := r.PathValue("address")
		var accounts []ledger.Account
		var more bool
		if address != "" && !ledger.IsPattern(address) {
			acct, err := l.Account(r.Context(), address)
			if err != nil {
				writeError(w, r, err)
				return
			}
			accounts = []ledger.Account{acct}
		} else {
			limit, err := parseLimit(r)
			if err != nil {
				writeError(w, r, err)
				return
			}
			if accounts, more, err = l.ListAccounts(r.Context(), address, r.URL.Query().Get("cursor"), limit); err != nil {
				writeError(w, r, err)
				return
			}
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

// parseLimit returns the request's limit query parameter, or 0 when it is absent. It returns
// ledger.ErrInvalid when limit is not an integer.
func parseLimit(r *http.Request) (int, error) {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(s)
	if err != nil || limit < 1 {
		return 0, fmt.Errorf("%w: limit %q must be a whole number from 1", ledger.ErrInvalid, s)
	}
	return limit, nil
}
