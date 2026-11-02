package httpapi

import (
	"net/http"

	"torment/internal/ledger"
)

// transferJSON is one posted transfer. ID and GroupID are decimal strings.
type transferJSON struct {
	ID      string `json:"id"`
	GroupID string `json:"group_id"`
}

// postTransfer handles POST /transfers. It responds 201 with the posted transfers, 200 when they were
// already posted, 400 for invalid input, 404 for a missing wallet, 409 when an id exists with different
// fields or belongs to a failed transfer, or 422 for insufficient funds.
func postTransfer(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Postings []struct {
				ID          string `json:"id"`
				Source      string `json:"source"`
				Destination string `json:"destination"`
				Currency    string `json:"currency"`
				Amount      string `json:"amount"`
				GroupID     string `json:"group_id"`
			} `json:"postings"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		postings := make([]ledger.Posting, 0, len(req.Postings))
		for _, p := range req.Postings {
			postings = append(postings, ledger.Posting{
				ID:          p.ID,
				Source:      p.Source,
				Destination: p.Destination,
				Currency:    p.Currency,
				Amount:      p.Amount,
				GroupID:     p.GroupID,
			})
		}
		transfers, created, err := l.Transfer(r.Context(), postings)
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]transferJSON, 0, len(transfers))
		for _, t := range transfers {
			out = append(out, transferJSON{ID: t.ID.BigInt().String(), GroupID: t.GroupID.BigInt().String()})
		}
		status := http.StatusOK
		if created {
			status = http.StatusCreated
		}
		writeJSON(w, status, struct {
			Transfers []transferJSON `json:"transfers"`
		}{out})
	}
}
