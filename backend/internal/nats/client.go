package nats

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"goGateway/internal/models"
)

// Client manages the NATS connection and JetStream context.
type Client struct {
	cfg models.NATSConfig
	nc  *nats.Conn
	js  jetstream.JetStream
	mu  sync.RWMutex
}

func NewClient(cfg models.NATSConfig) *Client {
	return &Client{cfg: cfg}
}

func (c *Client) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	url := fmt.Sprintf("nats://%s:%d", c.cfg.Host, c.cfg.Port)
	nc, err := nats.Connect(url,
		nats.Name("goGateway"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("nats: disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("nats: reconnected to %s", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		return fmt.Errorf("nats connect %s: %w", url, err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return fmt.Errorf("nats jetstream: %w", err)
	}

	c.nc = nc
	c.js = js

	if err := c.initStream(); err != nil {
		nc.Close()
		return err
	}

	log.Printf("nats: connected to %s, stream %s ready", url, c.cfg.StreamName)
	return nil
}

func (c *Client) initStream() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	streamCfg := jetstream.StreamConfig{
		Name: c.cfg.StreamName,
		// .metrics.> = IEC-104 fan-out (NatsDispatcher);
		// .ssfv.>    = decoded SSFV samples for the fan-out spine (Orion etc.).
		Subjects: []string{c.cfg.StreamName + ".metrics.>", c.cfg.StreamName + ".ssfv.>"},
		// Retention: limits size/age to prevent disk exhaustion in edge scenarios.
		MaxAge:    24 * time.Hour,
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
	}

	_, err := c.js.CreateOrUpdateStream(ctx, streamCfg)
	if err != nil {
		return fmt.Errorf("nats create stream %s: %w", c.cfg.StreamName, err)
	}
	return nil
}

func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.nc != nil {
		c.nc.Close()
	}
}

func (c *Client) JetStream() jetstream.JetStream {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.js
}

func (c *Client) Publish(subject string, data []byte) error {
	c.mu.RLock()
	js := c.js
	c.mu.RUnlock()

	if js == nil {
		return fmt.Errorf("nats: jetstream not initialized")
	}

	// Direct publish (optimistic). JetStream handles persistence.
	_, err := js.Publish(context.Background(), subject, data)
	return err
}
