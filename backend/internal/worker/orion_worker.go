package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"goGateway/internal/fiware"
	"goGateway/internal/nats"
)

// OrionWorker consumes decoded SSFV samples off the NATS spine and upserts them
// into a FIWARE Orion context broker. It is a durable JetStream consumer: if the
// gateway (or Orion) restarts, unacked samples are redelivered. Upsert is
// idempotent, so at-least-once delivery is safe.
type OrionWorker struct {
	client     *nats.Client
	streamName string
	orion      fiware.OrionClient
}

func NewOrionWorker(client *nats.Client, streamName string, orion fiware.OrionClient) *OrionWorker {
	return &OrionWorker{client: client, streamName: streamName, orion: orion}
}

func (w *OrionWorker) Run(ctx context.Context) error {
	js := w.client.JetStream()
	if js == nil {
		return fmt.Errorf("orion worker: nats jetstream not initialized")
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, w.streamName, jetstream.ConsumerConfig{
		Durable:       "orion-worker",
		Description:   "Upserts SSFV samples into FIWARE Orion (NGSIv2)",
		FilterSubject: SSFVSubjectFilter(w.streamName),
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("orion worker: create consumer: %w", err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return err
	}
	defer iter.Stop()

	log.Printf("nats: Orion worker started")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, jetstream.ErrMsgIteratorClosed) {
				return nil
			}
			log.Printf("orion worker: message error: %v", err)
			continue
		}

		var s SSFVSample
		if err := json.Unmarshal(msg.Data(), &s); err != nil {
			log.Printf("orion worker: unmarshal error: %v", err)
			msg.Term() // poison message — don't retry
			continue
		}

		upCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		err = w.orion.Upsert(upCtx, fiware.Reading{
			Entity: s.Entity, Codigo: s.Codigo, Instance: s.Instance,
			Value: s.Value, Quality: s.Quality, Time: s.Timestamp,
		})
		cancel()
		if err != nil {
			log.Printf("orion worker: upsert %s/%s: %v", s.Entity, s.Codigo, err)
			msg.Nak() // transient — redeliver
			continue
		}
		msg.Ack()
	}
}
