package httpapi

import "net/http"

// kindJSON is the response body for a kind.
type kindJSON struct {
	Name string `json:"name"`
}

// postKind handles POST /kinds. It responds 201 with the stored kind, or 200 when the kind was already
// registered.
func postKind(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req kindJSON
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, err)
			return
		}
		kind, created, err := l.RegisterKind(r.Context(), req.Name)
		if err != nil {
			writeError(w, r, err)
			return
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, kindJSON{Name: kind.Name})
	}
}

// getKinds handles GET /kinds and responds 200 with every registered kind, ordered by name.
func getKinds(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kinds, err := l.Kinds(r.Context())
		if err != nil {
			writeError(w, r, err)
			return
		}
		out := struct {
			Kinds []kindJSON `json:"kinds"`
		}{make([]kindJSON, 0, len(kinds))}
		for _, k := range kinds {
			out.Kinds = append(out.Kinds, kindJSON{Name: k.Name})
		}
		writeJSON(w, http.StatusOK, out)
	}
}
