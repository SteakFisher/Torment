package httpapi

import "net/http"

// currencyJSON is the response body for a currency.
type currencyJSON struct {
	Name      string `json:"name"`
	Precision int16  `json:"precision"`
}

// postCurrency handles POST /currencies. It responds 201 with the stored currency, 200 when the identical
// currency was already registered, or 409 when the name is registered with a different precision.
func postCurrency(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code string `json:"code"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		currency, created, err := l.RegisterCurrency(r.Context(), req.Code)
		if err != nil {
			writeError(w, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, currencyJSON{Name: currency.Name, Precision: currency.Precision})
	}
}

// getCurrencies handles GET /currencies and responds 200 with every registered currency, ordered by name.
func getCurrencies(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currencies, err := l.Currencies(r.Context())
		if err != nil {
			writeError(w, err)
			return
		}
		out := struct {
			Currencies []currencyJSON `json:"currencies"`
		}{make([]currencyJSON, 0, len(currencies))}
		for _, c := range currencies {
			out.Currencies = append(out.Currencies, currencyJSON{Name: c.Name, Precision: c.Precision})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
