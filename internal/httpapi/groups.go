package httpapi

import (
	"net/http"
	"time"
)

// groupTransferJSON is one transfer in a group. ID and Amount are decimal strings.
type groupTransferJSON struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Currency    string    `json:"currency"`
	Amount      string    `json:"amount"`
	Timestamp   time.Time `json:"timestamp"`
}

// getGroupTransfers handles GET /groups/{group_id}/transfers. It responds 200 with the group's transfers,
// oldest first, or 400 when the group id is malformed.
func getGroupTransfers(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID := r.PathValue("group_id")
		transfers, err := l.GroupTransfers(r.Context(), groupID)
		if err != nil {
			writeError(w, err)
			return
		}
		out := make([]groupTransferJSON, 0, len(transfers))
		for _, t := range transfers {
			out = append(out, groupTransferJSON{
				ID:          t.ID.BigInt().String(),
				Source:      t.Source,
				Destination: t.Destination,
				Currency:    t.Currency,
				Amount:      t.Amount.String(),
				Timestamp:   t.Timestamp,
			})
		}
		writeJSON(w, http.StatusOK, struct {
			GroupID   string              `json:"group_id"`
			Transfers []groupTransferJSON `json:"transfers"`
		}{groupID, out})
	}
}
