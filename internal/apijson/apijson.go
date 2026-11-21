// Package apijson holds the JSON shapes that both the HTTP API and the event export write, so a transfer
// reads the same in both.
package apijson

import (
	"encoding/json"
	"time"

	"torment/internal/ledger"
)

// TransferRecord is one committed transfer. ID and GroupID are UUIDs; Amount is a decimal string.
type TransferRecord struct {
	ID        string    `json:"id"`
	GroupID   string    `json:"group_id"`
	Debit     string    `json:"debit"`
	Credit    string    `json:"credit"`
	Currency  string    `json:"currency"`
	Precision int16     `json:"precision"`
	Amount    string    `json:"amount"`
	Kind      string    `json:"kind"`
	Timestamp time.Time `json:"timestamp"`
}

// NewTransferRecord returns t in its JSON shape.
func NewTransferRecord(t ledger.TransferRecord) TransferRecord {
	return TransferRecord{
		ID:        ledger.FormatID(t.ID),
		GroupID:   ledger.FormatID(t.GroupID),
		Debit:     t.Debit,
		Credit:    t.Credit,
		Currency:  t.Currency,
		Precision: t.Precision,
		Amount:    t.Amount.String(),
		Kind:      t.Kind,
		Timestamp: t.Timestamp,
	}
}

// MarshalEvent returns e as the JSON of an exported event: the fields of a TransferRecord, then
// debit_balance and credit_balance as decimal strings.
func MarshalEvent(e ledger.Event) ([]byte, error) {
	return json.Marshal(struct {
		TransferRecord
		DebitBalance  string `json:"debit_balance"`
		CreditBalance string `json:"credit_balance"`
	}{NewTransferRecord(e.TransferRecord), e.DebitBalance.String(), e.CreditBalance.String()})
}
