package worker

import (
	"encoding/json"
	"log"
	"time"

	"goGateway/internal/nats"
)

// SSFVSample is one decoded SSFV reading published to the NATS fan-out spine.
// TimescaleDB is still written directly on the ingestion path (source of truth);
// additional sinks — FIWARE Orion today, others later — consume this stream
// without touching ingestion.
type SSFVSample struct {
	Entity    string    `json:"entity"`   // group/node[/device] (== tbl_equipo.nombre_topic)
	Codigo    string    `json:"codigo"`   // leaf attribute (codigo_senal)
	Instance  string    `json:"instance"` // folder path, or "default"
	Value     float64   `json:"value"`
	Quality   int       `json:"quality"`
	Timestamp time.Time `json:"ts"`
}

// ssfvSubject is the JetStream subject SSFV samples are published to. The leaf is
// fixed so a catch-all consumer filters SSFVSubjectFilter; the entity (which can
// contain '/') travels in the payload, not the subject hierarchy.
func ssfvSubject(stream string) string { return stream + ".ssfv.sample" }

// SSFVSubjectFilter is the consumer FilterSubject for SSFV samples.
func SSFVSubjectFilter(stream string) string { return stream + ".ssfv.>" }

// NatsSSFVPublisher fans accepted SSFV samples out to JetStream (best-effort).
type NatsSSFVPublisher struct {
	client  *nats.Client
	subject string
}

func NewNatsSSFVPublisher(client *nats.Client, stream string) *NatsSSFVPublisher {
	return &NatsSSFVPublisher{client: client, subject: ssfvSubject(stream)}
}

// Publish never blocks ingestion: a marshal/publish error is logged and dropped,
// since the direct TimescaleDB write remains the source of truth.
func (p *NatsSSFVPublisher) Publish(s SSFVSample) {
	data, err := json.Marshal(s)
	if err != nil {
		return
	}
	if err := p.client.Publish(p.subject, data); err != nil {
		log.Printf("nats: ssfv sample publish error: %v", err)
	}
}
