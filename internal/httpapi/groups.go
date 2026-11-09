package httpapi

import (
	"net/http"
	"strings"
	"time"

	"torment/internal/ledger"
)

// transferRecordJSON is one committed transfer. ID and GroupID are UUIDs; Amount is a decimal string.
type transferRecordJSON struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	Debit     string    `json:"debit"`
	Credit    string    `json:"credit"`
	Currency  string    `json:"currency"`
	Amount    string    `json:"amount"`
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
}

func newTransferRecordsJSON(transfers []ledger.TransferRecord) []transferRecordJSON {
	out := make([]transferRecordJSON, 0, len(transfers))
	for _, t := range transfers {
		out = append(out, transferRecordJSON{
			ID:        ledger.FormatID(t.ID),
			GroupID:   ledger.FormatID(t.GroupID),
			Debit:     t.Debit,
			Credit:    t.Credit,
			Currency:  t.Currency,
			Amount:    t.Amount.String(),
			Kind:      t.Kind,
			Timestamp: t.Timestamp,
		})
	}
	return out
}

// getGroupTransfers handles GET /groups/{group_id}/transfers?kind=&cursor=&limit=. It responds 200 with
// the group's transfers, newest first, and next set to the last timestamp when older ones remain; 400 for
// a malformed group id or query; or 404 for an unregistered kind.
func getGroupTransfers(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := ledger.GroupTransfersParams{GroupID: r.PathValue("group_id"), Kind: r.URL.Query().Get("kind")}
		var err error
		if p.Cursor, err = parseTime(r, "cursor"); err != nil {
			writeError(w, r, err)
			return
		}
		if p.Limit, err = parseLimit(r); err != nil {
			writeError(w, r, err)
			return
		}
		transfers, more, err := l.GroupTransfers(r.Context(), p)
		if err != nil {
			writeError(w, r, err)
			return
		}
		out := struct {
			GroupID   string               `json:"group_id"`
			Transfers []transferRecordJSON `json:"transfers"`
			Next      *time.Time           `json:"next,omitempty"`
		}{GroupID: strings.ToLower(p.GroupID), Transfers: newTransferRecordsJSON(transfers)}
		if more {
			out.Next = &transfers[len(transfers)-1].Timestamp
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// getAccountTransfers handles GET /accounts/{address}/transfers?currency=&kind=&from=&cursor=&limit=. It
// responds 200 with the account's transfers, newest first, and next set to the last timestamp when older
// ones remain; 400 for a malformed query or a pattern matching too many wallets; or 404 when the account
// does not exist or the kind is not registered.
func getAccountTransfers(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := ledger.AccountTransfersParams{Address: r.PathValue("address"), Currency: r.URL.Query().Get("currency"), Kind: r.URL.Query().Get("kind")}
		var err error
		if p.From, err = parseTime(r, "from"); err != nil {
			writeError(w, r, err)
			return
		}
		if p.Cursor, err = parseTime(r, "cursor"); err != nil {
			writeError(w, r, err)
			return
		}
		if p.Limit, err = parseLimit(r); err != nil {
			writeError(w, r, err)
			return
		}
		transfers, more, err := l.AccountTransfers(r.Context(), p)
		if err != nil {
			writeError(w, r, err)
			return
		}
		out := struct {
			Address   string               `json:"address"`
			Transfers []transferRecordJSON `json:"transfers"`
			Next      *time.Time           `json:"next,omitempty"`
		}{Address: p.Address, Transfers: newTransferRecordsJSON(transfers)}
		if more {
			out.Next = &transfers[len(transfers)-1].Timestamp
		}
		writeJSON(w, http.StatusOK, out)
	}
}
