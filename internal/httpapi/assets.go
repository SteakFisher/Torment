package httpapi

import "net/http"

// assetJSON is the response body for an asset.
type assetJSON struct {
	Name      string `json:"name"`
	Precision int16  `json:"precision"`
	Ledger    int32  `json:"ledger"`
}

// postAsset handles POST /assets and responds 201 with the stored asset.
func postAsset(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Code   string `json:"code"`
			Ledger int32  `json:"ledger"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		asset, err := l.RegisterAsset(r.Context(), req.Code, req.Ledger)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, assetJSON{Name: asset.Name, Precision: asset.Precision, Ledger: asset.Ledger})
	}
}
