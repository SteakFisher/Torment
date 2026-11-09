package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"torment/internal/ledger"
)

// postingJSON is one posting in a request.
type postingJSON struct {
	ID       string `json:"id"`
	Debit    string `json:"debit"`
	Credit   string `json:"credit"`
	Currency string `json:"currency"`
	Amount   string `json:"amount"`
	GroupID  string `json:"group_id"`
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
func newTransferJSON(ctx context.Context, t ledger.Transfer) transferJSON {
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
		case errors.Is(t.Err, ledger.ErrOverflow):
			out.Error = "overflow"
		default:
			out.Error, out.Message = "internal", "internal error"
			slog.Error("transfer failed", "request_id", requestID(ctx), "id", out.ID, "err", t.Err)
		}
	}
	return out
}

// postTransfer handles POST /transfers. It responds 200 with a result for each posting, nested in chains as in
// the request; 400 for invalid input; or 404 for an unregistered account or currency.
func postTransfer(l Ledger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Chains [][]postingJSON `json:"chains"`
		}
		if err := decodeJSON(w, r, &req); err != nil {
			writeError(w, r, err)
			return
		}
		chains := make([][]ledger.Posting, len(req.Chains))
		for i, c := range req.Chains {
			for _, p := range c {
				chains[i] = append(chains[i], ledger.Posting(p))
			}
		}
		results, err := l.Transfer(r.Context(), chains)
		if err != nil {
			writeError(w, r, err)
			return
		}
		out := make([][]transferJSON, len(results))
		for i, chain := range results {
			for _, t := range chain {
				out[i] = append(out[i], newTransferJSON(r.Context(), t))
			}
		}
		writeJSON(w, http.StatusOK, struct {
			Chains [][]transferJSON `json:"chains"`
		}{out})
	}
}
