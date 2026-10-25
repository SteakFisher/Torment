package httpapi

import "net/http"

// currencyJSON is the response body for a currency.
type currencyJSON struct {
	Name      string `json:"name"`
	Precision int16  `json:"precision"`
	Ledger    int32  `json:"ledger"`
}

// postCurrency handles POST /currencies and responds 201 with the stored currency.
func postCurrency(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code   string `json:"code"`
			Ledger int32  `json:"ledger"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		currency, err := l.RegisterCurrency(r.Context(), req.Code, req.Ledger)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, currencyJSON{Name: currency.Name, Precision: currency.Precision, Ledger: currency.Ledger})
	}
}
