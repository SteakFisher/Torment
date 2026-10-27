package httpapi

import "net/http"

// balanceJSON is an account's balance in one currency. Amount is in minor units as a decimal string.
type balanceJSON struct {
	Currency  string `json:"currency"`
	Precision int16  `json:"precision"`
	Amount    string `json:"amount"`
}

// getBalances handles GET /accounts/{address}/balances. It responds 200 with the balance of each of the
// account's wallets, or 404 when the account does not exist.
func getBalances(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address := r.PathValue("address")
		balances, err := l.Balances(r.Context(), address)
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
