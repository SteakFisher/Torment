package cdc

import (
	"context"
	"errors"
	"math/big"
	"os"
	"strconv"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	tb "github.com/tigerbeetle/tigerbeetle-go"

	"torment/internal/ledger"
)

// fakeLedger turns each message, a decimal number n, into an event with id n, or fails every batch
// when fail is set.
type fakeLedger struct{ fail bool }

var _ Ledger = fakeLedger{}

func (f fakeLedger) Events(_ context.Context, raw [][]byte) ([]ledger.Event, error) {
	if f.fail {
		return nil, errors.New("broken message")
	}
	events := make([]ledger.Event, len(raw))
	for i, r := range raw {
		n, err := strconv.ParseUint(string(r), 10, 64)
		if err != nil {
			return nil, err
		}
		events[i] = ledger.Event{
			TransferRecord: ledger.TransferRecord{
				ID: tb.ToUint128(n), GroupID: tb.ToUint128(n), Debit: "world", Credit: "rider:1:wallet",
				Currency: "USD", Precision: 2, Amount: big.NewInt(1), Kind: "top_up", Timestamp: time.Unix(1, 0).UTC(),
			},
			DebitBalance:  big.NewInt(-1),
			CreditBalance: big.NewInt(1),
		}
	}
	return events, nil
}

// broker holds the test vhosts: the source queue, emptied, and a queue bound to every event.
type broker struct {
	sourceURL, eventsURL string
	source               *amqp.Channel
	events               <-chan amqp.Delivery
}

// newBroker connects to the vhosts at TEST_CDC_SOURCE_URL and TEST_CDC_EVENTS_URL, and skips the test
// when either is unset.
func newBroker(t *testing.T) *broker {
	b := &broker{sourceURL: os.Getenv("TEST_CDC_SOURCE_URL"), eventsURL: os.Getenv("TEST_CDC_EVENTS_URL")}
	if b.sourceURL == "" || b.eventsURL == "" {
		t.Skip("TEST_CDC_SOURCE_URL or TEST_CDC_EVENTS_URL is not set")
	}
	b.source = channel(t, b.sourceURL)
	if _, err := b.source.QueuePurge(sourceQueue, false); err != nil {
		t.Fatal(err)
	}
	if err := b.source.Confirm(false); err != nil {
		t.Fatal(err)
	}
	events := channel(t, b.eventsURL)
	q, err := events.QueueDeclare("", false, true, true, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := events.QueueBind(q.Name, "#", eventsExchange, false, nil); err != nil {
		t.Fatal(err)
	}
	if b.events, err = events.Consume(q.Name, "", true, true, false, false, nil); err != nil {
		t.Fatal(err)
	}
	return b
}

// channel opens a channel on the vhost at url, closed when the test ends.
func channel(t *testing.T, url string) *amqp.Channel {
	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	ch, err := conn.Channel()
	if err != nil {
		t.Fatal(err)
	}
	return ch
}

// send puts one message per body on the source queue and waits for RabbitMQ to confirm them.
func (b *broker) send(t *testing.T, bodies ...string) {
	for _, body := range bodies {
		c, err := b.source.PublishWithDeferredConfirm("", sourceQueue, true, false, amqp.Publishing{Body: []byte(body)})
		if err != nil {
			t.Fatal(err)
		}
		if !c.Wait() {
			t.Fatalf("rabbitmq rejected message %s", body)
		}
	}
}

// waiting returns the number of messages waiting in the source queue once it has been want for up to
// 5 seconds; unacknowledged messages return to it a moment after the exporter disconnects.
func (b *broker) waiting(t *testing.T, want int) int {
	var n int
	for range 50 {
		q, err := b.source.QueueDeclarePassive(sourceQueue, true, false, false, false, nil)
		if err != nil {
			t.Fatal(err)
		}
		if n = q.Messages; n == want {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return n
}

// received returns the message ids of the next n events, failing after 10 seconds.
func (b *broker) received(t *testing.T, n int) []string {
	var ids []string
	timeout := time.After(10 * time.Second)
	for len(ids) < n {
		select {
		case d := <-b.events:
			if d.RoutingKey != "USD.top_up" {
				t.Fatalf("routing key = %q; want USD.top_up", d.RoutingKey)
			}
			ids = append(ids, d.MessageId)
		case <-timeout:
			t.Fatalf("got events %v; want %d", ids, n)
		}
	}
	return ids
}

// start runs the exporter with l in the background until cancel is called or the test ends; done gets
// what Run returned.
func (b *broker) start(t *testing.T, l Ledger) (cancel context.CancelFunc, done <-chan error) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	errs := make(chan error, 1)
	go func() { errs <- Run(ctx, l, b.sourceURL, b.eventsURL) }()
	return cancel, errs
}

// stop ends an exporter started by start and fails unless it returned nil.
func stop(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestRunExportsAndAcknowledges(t *testing.T) {
	b := newBroker(t)
	b.send(t, "1", "2", "3")

	cancel, done := b.start(t, fakeLedger{})

	got := b.received(t, 3)
	want := []string{"00000000-0000-0000-0000-000000000001", "00000000-0000-0000-0000-000000000002", "00000000-0000-0000-0000-000000000003"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("events = %v; want %v in order", got, want)
		}
	}
	stop(t, cancel, done)
	if n := b.waiting(t, 0); n != 0 {
		t.Fatalf("%d messages still waiting; want all acknowledged", n)
	}
}

func TestRunKeepsMessagesItCannotExport(t *testing.T) {
	b := newBroker(t)
	b.send(t, "1", "2")

	_, failed := b.start(t, fakeLedger{fail: true})
	select {
	case err := <-failed:
		if err == nil {
			t.Fatal("Run returned nil; want the ledger's error")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Run did not stop on the failing batch")
	}
	if n := b.waiting(t, 2); n != 2 {
		t.Fatalf("%d messages waiting after the failure; want 2", n)
	}

	// The next exporter exports them.
	cancel, done := b.start(t, fakeLedger{})
	if got := b.received(t, 2); len(got) != 2 {
		t.Fatalf("events = %v; want 2", got)
	}
	stop(t, cancel, done)
	if n := b.waiting(t, 0); n != 0 {
		t.Fatalf("%d messages still waiting; want all acknowledged", n)
	}
}
