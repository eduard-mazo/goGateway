package worker

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"goGateway/internal/iec104"
	"goGateway/internal/nats"
)

// InternalPoint is the unified message format published to NATS.
type InternalPoint struct {
	MappingID  int64     `json:"mapping_id"`
	ServerID   int64     `json:"server_id"`
	Topic      string    `json:"topic"`
	IOA        int       `json:"ioa"`
	TypeID     string    `json:"type_id"`
	Value      float64   `json:"value"`
	Quality    int       `json:"quality"`
	Timestamp  time.Time `json:"timestamp"`
	SignalPath string    `json:"signal_path"`
	Business   string    `json:"business"`
	Company    string    `json:"company"`
}

// Dispatcher abstracts the destination for decoded samples.
type Dispatcher interface {
	Dispatch(tm TopicMapping, val float64, quality int, ts time.Time)
}

// DirectDispatcher sends samples immediately to IEC-104 and the history logger (which feeds TSDB).
type DirectDispatcher struct {
	srv  iec104.Server
	hist *HistoryLogger
}

func NewDirectDispatcher(srv iec104.Server, hist *HistoryLogger) *DirectDispatcher {
	return &DirectDispatcher{srv: srv, hist: hist}
}

func (d *DirectDispatcher) Dispatch(tm TopicMapping, val float64, quality int, ts time.Time) {
	d.srv.Dispatch(tm.ServerID, iec104.Point{
		IOA:       tm.IOA,
		TypeID:    tm.IEC104Type,
		Value:     val,
		Quality:   quality,
		Timestamp: ts,
	})

	if !d.hist.Log(HistoryEvent{
		MappingID:  tm.MappingID,
		SignalPath: tm.SignalPath,
		Value:      val,
		Quality:    quality,
		Timestamp:  ts,
		IOA:        tm.IOA,
		Business:   tm.Business,
		Company:    tm.Company,
	}) {
		log.Printf("history buffer full, dropped %s", tm.SignalPath)
	}
}

// NatsDispatcher publishes samples to a NATS JetStream topic.
type NatsDispatcher struct {
	client     *nats.Client
	streamName string
}

func NewNatsDispatcher(client *nats.Client, streamName string) *NatsDispatcher {
	return &NatsDispatcher{client: client, streamName: streamName}
}

func (d *NatsDispatcher) Dispatch(tm TopicMapping, val float64, quality int, ts time.Time) {
	pt := InternalPoint{
		MappingID:  tm.MappingID,
		ServerID:   tm.ServerID,
		Topic:      tm.Topic,
		IOA:        tm.IOA,
		TypeID:     tm.IEC104Type,
		Value:      val,
		Quality:    quality,
		Timestamp:  ts,
		SignalPath: tm.SignalPath,
		Business:   tm.Business,
		Company:    tm.Company,
	}

	data, err := json.Marshal(pt)
	if err != nil {
		log.Printf("nats: marshal error: %v", err)
		return
	}

	// Subject format: {STREAM}.metrics.{server_id}.{ioa}
	subject := fmt.Sprintf("%s.metrics.%d.%d", d.streamName, tm.ServerID, tm.IOA)
	if err := d.client.Publish(subject, data); err != nil {
		log.Printf("nats: publish error on subject %s: %v", subject, err)
	}
}
