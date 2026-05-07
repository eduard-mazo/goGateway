package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/nats-io/nats.go/jetstream"

	"goGateway/internal/iec104"
	"goGateway/internal/nats"
	"goGateway/internal/tsdb"
)

// SCADAWorker consumes from NATS and dispatches to IEC-104.
type SCADAWorker struct {
	client     *nats.Client
	streamName string
	srv        iec104.Server
}

func NewSCADAWorker(client *nats.Client, streamName string, srv iec104.Server) *SCADAWorker {
	return &SCADAWorker{client: client, streamName: streamName, srv: srv}
}

func (w *SCADAWorker) Run(ctx context.Context) error {
	js := w.client.JetStream()
	if js == nil {
		return fmt.Errorf("scada worker: nats jetstream not initialized")
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, w.streamName, jetstream.ConsumerConfig{
		Durable:       "scada-worker",
		Description:   "Forwards metrics to IEC-104 SCADA",
		FilterSubject: w.streamName + ".metrics.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("scada worker: create consumer: %w", err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return err
	}
	defer iter.Stop()

	log.Printf("nats: SCADA worker started")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("scada worker: message error: %v", err)
			continue
		}

		var pt InternalPoint
		if err := json.Unmarshal(msg.Data(), &pt); err != nil {
			log.Printf("scada worker: unmarshal error: %v", err)
			msg.Term() // don't retry bad data
			continue
		}

		w.srv.Dispatch(pt.ServerID, iec104.Point{
			IOA:       pt.IOA,
			TypeID:    pt.TypeID,
			Value:     pt.Value,
			Quality:   pt.Quality,
			Timestamp: pt.Timestamp,
		})

		msg.Ack()
	}
}

// TSDBWorker consumes from NATS and pushes to the TSDB pipeline.
type TSDBWorker struct {
	client     *nats.Client
	streamName string
	tsdbPipe   *tsdb.WritePipeline
}

func NewTSDBWorker(client *nats.Client, streamName string, pipe *tsdb.WritePipeline) *TSDBWorker {
	return &TSDBWorker{client: client, streamName: streamName, tsdbPipe: pipe}
}

func (w *TSDBWorker) Run(ctx context.Context) error {
	js := w.client.JetStream()
	if js == nil {
		return fmt.Errorf("tsdb worker: nats jetstream not initialized")
	}

	consumer, err := js.CreateOrUpdateConsumer(ctx, w.streamName, jetstream.ConsumerConfig{
		Durable:       "tsdb-worker",
		Description:   "Pushes metrics to TSDB (VictoriaMetrics/Timescale)",
		FilterSubject: w.streamName + ".metrics.>",
		AckPolicy:     jetstream.AckExplicitPolicy,
	})
	if err != nil {
		return fmt.Errorf("tsdb worker: create consumer: %w", err)
	}

	iter, err := consumer.Messages()
	if err != nil {
		return err
	}
	defer iter.Stop()

	log.Printf("nats: TSDB worker started")

	for {
		msg, err := iter.Next()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			log.Printf("tsdb worker: message error: %v", err)
			continue
		}

		var pt InternalPoint
		if err := json.Unmarshal(msg.Data(), &pt); err != nil {
			log.Printf("tsdb worker: unmarshal error: %v", err)
			msg.Term()
			continue
		}

		// Push to TSDB pipeline (matches HistoryLogger logic)
		err = w.tsdbPipe.Push(tsdb.DataPoint{
			Measurement: lastPathSegment(pt.SignalPath),
			Timestamp:   pt.Timestamp,
			Tags: map[string]string{
				"path":      pt.SignalPath,
				"server_id": fmt.Sprintf("%d", pt.ServerID),
			},
			Fields: map[string]float64{
				"value":   pt.Value,
				"quality": float64(pt.Quality),
			},
		})

		if err != nil {
			// Pipeline buffer full. NATS will redeliver later if we don't ACK.
			// But we don't want to block the loop. 
			// In high load, we might want to wait or just log.
			log.Printf("tsdb worker: pipeline push error: %v", err)
			continue 
		}

		msg.Ack()
	}
}
