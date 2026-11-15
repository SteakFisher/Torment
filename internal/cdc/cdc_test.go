package cdc

import (
	"math/big"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

func TestPublishing(t *testing.T) {
	e := ledger.Event{
		TransferRecord: ledger.TransferRecord{
			ID: tb.ToUint128(5), GroupID: tb.ToUint128(9), Debit: "world", Credit: "rider:1:wallet",
			Currency: "USD", Precision: 2, Amount: big.NewInt(100), Kind: "top_up", Timestamp: time.Unix(1, 500).UTC(),
		},
		DebitBalance:  big.NewInt(-100),
		CreditBalance: big.NewInt(100),
	}
	key, msg, err := publishing(e)
	if err != nil {
		t.Fatal(err)
	}
	wantBody := `{"id":"00000000-0000-0000-0000-000000000005","group_id":"00000000-0000-0000-0000-000000000009",` +
		`"debit":"world","credit":"rider:1:wallet","currency":"USD","precision":2,"amount":"100","kind":"top_up",` +
		`"timestamp":"1970-01-01T00:00:01.0000005Z","debit_balance":"-100","credit_balance":"100"}`
	if key != "USD.top_up" || string(msg.Body) != wantBody {
		t.Fatalf("key = %q, body = %s; want USD.top_up, %s", key, msg.Body, wantBody)
	}
	if msg.MessageId != "00000000-0000-0000-0000-000000000005" || msg.DeliveryMode != amqp.Persistent || msg.ContentType != "application/json" {
		t.Fatalf("message = %+v; want the transfer id, persistent, application/json", msg)
	}
}
