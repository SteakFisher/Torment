// Package cdc exports committed transfers: it reads the messages of TigerBeetle's CDC job from RabbitMQ
// and publishes each one as an event with the names the API uses.
package cdc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"torment/internal/apijson"
	"torment/internal/ledger"
)

const (
	// sourceQueue holds the messages of TigerBeetle's CDC job.
	sourceQueue = "torment.cdc"
	// eventsExchange is the topic exchange the events are published to.
	eventsExchange = "torment.events"
	// batchMax is the most messages exported at once, and the most RabbitMQ hands out unacknowledged.
	batchMax = 500
)

// Ledger is the ledger operation the exporter calls.
type Ledger interface {
	Events(ctx context.Context, raw [][]byte) ([]ledger.Event, error)
}

// Fails to compile when *ledger.Names no longer satisfies Ledger.
var _ Ledger = (*ledger.Names)(nil)

// Run exports messages from sourceQueue on the RabbitMQ vhost at sourceURL to eventsExchange on the
// vhost at eventsURL until ctx ends, and then returns nil once the batch in flight is exported. A message
// is acknowledged only after RabbitMQ confirms its event, so a message is never lost and may be exported
// twice. Run returns an error when a connection fails or a message can't be turned into an event; that
// message stays in the queue.
func Run(ctx context.Context, l Ledger, sourceURL, eventsURL string) error {
	source, err := amqp.Dial(sourceURL)
	if err != nil {
		return fmt.Errorf("error connecting to the cdc source: %w", err)
	}
	defer source.Close()
	in, err := source.Channel()
	if err != nil {
		return fmt.Errorf("error opening the cdc source channel: %w", err)
	}
	if err := in.Qos(batchMax, 0, false); err != nil {
		return fmt.Errorf("error setting the cdc source prefetch: %w", err)
	}
	deliveries, err := in.Consume(sourceQueue, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("error consuming %s: %w", sourceQueue, err)
	}

	events, err := amqp.Dial(eventsURL)
	if err != nil {
		return fmt.Errorf("error connecting to the events broker: %w", err)
	}
	defer events.Close()
	out, err := events.Channel()
	if err != nil {
		return fmt.Errorf("error opening the events channel: %w", err)
	}
	if err := out.Confirm(false); err != nil {
		return fmt.Errorf("error enabling publisher confirms: %w", err)
	}

	slog.Info("exporting", "queue", sourceQueue, "exchange", eventsExchange)
	for {
		var batch []amqp.Delivery
		select {
		case <-ctx.Done():
			return nil
		case d, ok := <-deliveries:
			if !ok {
				return errors.New("the cdc source closed the consumer")
			}
			batch = append(batch, d)
		}
		// Take what is already waiting, without waiting for more.
	waiting:
		for len(batch) < batchMax {
			select {
			case d, ok := <-deliveries:
				if !ok {
					break waiting
				}
				batch = append(batch, d)
			default:
				break waiting
			}
		}

		// A batch already being exported gets up to 10 seconds to finish after ctx ends, so a shutdown
		// doesn't export it twice.
		exportCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		stop := context.AfterFunc(ctx, func() { time.AfterFunc(10*time.Second, cancel) })
		err := export(exportCtx, l, out, batch)
		stop()
		cancel()
		if err != nil {
			return err
		}
		if err := batch[len(batch)-1].Ack(true); err != nil {
			return fmt.Errorf("error acknowledging cdc messages: %w", err)
		}
	}
}

// export publishes the event of each message in batch on out and waits for RabbitMQ to confirm them all.
func export(ctx context.Context, l Ledger, out *amqp.Channel, batch []amqp.Delivery) error {
	raw := make([][]byte, len(batch))
	for i, d := range batch {
		raw[i] = d.Body
	}
	events, err := l.Events(ctx, raw)
	if err != nil {
		return err
	}
	confirms := make([]*amqp.DeferredConfirmation, len(events))
	for i, e := range events {
		key, msg, err := publishing(e)
		if err != nil {
			return err
		}
		if confirms[i], err = out.PublishWithDeferredConfirmWithContext(ctx, eventsExchange, key, false, false, msg); err != nil {
			return fmt.Errorf("error publishing event %s: %w", msg.MessageId, err)
		}
	}
	for i, c := range confirms {
		acked, err := c.WaitContext(ctx)
		if err != nil {
			return fmt.Errorf("error waiting for the confirm of event %s: %w", ledger.FormatID(events[i].ID), err)
		}
		if !acked {
			return fmt.Errorf("rabbitmq rejected event %s", ledger.FormatID(events[i].ID))
		}
	}
	return nil
}

// publishing returns the message for e and its routing key, "<currency>.<kind>". The message is
// persistent JSON whose message id is the transfer's id.
func publishing(e ledger.Event) (key string, msg amqp.Publishing, err error) {
	body, err := apijson.MarshalEvent(e)
	if err != nil {
		return "", amqp.Publishing{}, fmt.Errorf("error encoding event %s: %w", ledger.FormatID(e.ID), err)
	}
	return e.Currency + "." + e.Kind, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		MessageId:    ledger.FormatID(e.ID),
		Timestamp:    e.Timestamp,
		AppId:        "torment",
		Body:         body,
	}, nil
}
