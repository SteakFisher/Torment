package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"torment/internal/ledger"
)

// postingJSON is one posting in a request.
type postingJSON struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Currency    string `json:"currency"`
	Amount      string `json:"amount"`
	GroupID     string `json:"group_id"`
}

// chainJSON is one element of a request's postings: a single posting, or an array of postings that form
// one chain. array reports which form it was.
type chainJSON struct {
	postings []postingJSON
	array    bool
}

// UnmarshalJSON decodes an array of postings or a single posting, rejecting unknown fields.
func (c *chainJSON) UnmarshalJSON(b []byte) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if c.array = bytes.HasPrefix(bytes.TrimSpace(b), []byte("[")); c.array {
		return dec.Decode(&c.postings)
	}
	c.postings = make([]postingJSON, 1)
	return dec.Decode(&c.postings[0])
}

// transferJSON is the outcome of one posting. ID and GroupID are decimal strings; Error and Message are
// set when Status is "failed".
type transferJSON struct {
	ID      string `json:"id"`
	GroupID string `json:"group_id"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Message string `json:"message,omitempty"`
}

// newTransferJSON returns the response for t. An error it does not recognise is logged and reported as
// "internal".
func newTransferJSON(t ledger.Transfer) transferJSON {
	out := transferJSON{ID: t.ID.BigInt().String(), GroupID: t.GroupID.BigInt().String(), Status: "exists"}
	switch {
	case t.Err == nil && t.Created:
		out.Status = "created"
	case t.Err == nil:
	default:
		out.Status, out.Message = "failed", t.Err.Error()
		switch {
		case errors.Is(t.Err, ledger.ErrInsufficientFunds):
			out.Error = "insufficient_funds"
		case errors.Is(t.Err, ledger.ErrConflict):
			out.Error = "conflict"
		case errors.Is(t.Err, ledger.ErrIDAlreadyFailed):
			out.Error = "id_already_failed"
		case errors.Is(t.Err, ledger.ErrChainFailed):
			out.Error = "chain_failed"
		default:
			out.Error, out.Message = "internal", "internal error"
			slog.Error("transfer failed", "id", out.ID, "err", t.Err)
		}
	}
	return out
}

// postTransfer handles POST /transfers. It responds 200 with a result for each posting, 400 for invalid
// input, or 404 for a missing wallet.
func postTransfer(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Postings []chainJSON `json:"postings"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, err)
			return
		}
		chains := make([][]ledger.Posting, len(req.Postings))
		for i, c := range req.Postings {
			for _, p := range c.postings {
				chains[i] = append(chains[i], ledger.Posting(p))
			}
		}
		results, err := l.Transfer(r.Context(), chains)
		if err != nil {
			writeError(w, err)
			return
		}
		// each element is a transferJSON, or a []transferJSON when the request element was an array
		out := make([]any, len(results))
		for i, chain := range results {
			if !req.Postings[i].array {
				out[i] = newTransferJSON(chain[0])
				continue
			}
			arr := make([]transferJSON, len(chain))
			for j, t := range chain {
				arr[j] = newTransferJSON(t)
			}
			out[i] = arr
		}
		writeJSON(w, http.StatusOK, struct {
			Transfers []any `json:"transfers"`
		}{out})
	}
}
