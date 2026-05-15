package tsdb

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

// VMConfig holds VictoriaMetrics connection settings.
type VMConfig struct {
	URL          string        // e.g. http://host:8428
	Username     string        // optional basic auth
	Password     string
	Timeout      time.Duration // default 10s
	MaxIdleConns int           // default 16
}

// VMAdapter writes DataPoints to VictoriaMetrics using gzip-compressed
// InfluxDB line protocol. VM's /write endpoint is influx-compatible and
// handles 1M+ pts/sec on modest hardware.
type VMAdapter struct {
	cfg         VMConfig
	client      *http.Client
	writeCount  atomic.Int64
	errorCount  atomic.Int64
	bytesTotal  atomic.Int64
	lastErrMsg  atomic.Value
	circuitOpen atomic.Bool
	rateTracker *rateTracker
}

func NewVMAdapter(cfg VMConfig) *VMAdapter {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 16
	}
	return &VMAdapter{
		cfg: cfg,
		client: &http.Client{
			Timeout: cfg.Timeout,
			Transport: &http.Transport{
				MaxIdleConnsPerHost: cfg.MaxIdleConns,
				IdleConnTimeout:     90 * time.Second,
				DisableCompression:  true, // we compress manually
				ForceAttemptHTTP2:   true,
			},
		},
		rateTracker: newRateTracker(10 * time.Second),
	}
}

func (v *VMAdapter) Name() string { return "victoriametrics" }

// WriteBatch encodes batch as gzip line protocol and POSTs to /write.
func (v *VMAdapter) WriteBatch(ctx context.Context, batch []DataPoint) error {
	if len(batch) == 0 {
		return nil
	}
	body, n, err := encodeLineProtocolGzip(batch)
	if err != nil {
		return fmt.Errorf("vm encode: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		v.cfg.URL+"/write?precision=ns", body)
	if err != nil {
		return fmt.Errorf("vm request: %w", err)
	}
	req.Header.Set("Content-Type", "text/plain")
	req.Header.Set("Content-Encoding", "gzip")
	if v.cfg.Username != "" {
		req.SetBasicAuth(v.cfg.Username, v.cfg.Password)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		v.errorCount.Add(1)
		v.lastErrMsg.Store(err.Error())
		return fmt.Errorf("vm write: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		errBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		msg := fmt.Sprintf("http %d: %s", resp.StatusCode, strings.TrimSpace(string(errBody)))
		v.errorCount.Add(1)
		v.lastErrMsg.Store(msg)
		log.Printf("tsdb [victoriametrics] write rejected: %s", msg)
		return fmt.Errorf("vm write: %s", msg)
	}
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	v.writeCount.Add(int64(len(batch)))
	v.bytesTotal.Add(int64(n))
	v.rateTracker.record(int64(len(batch)))
	return nil
}

func (v *VMAdapter) HealthCheck(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.URL+"/health", nil)
	resp, err := v.client.Do(req)
	if err != nil {
		v.circuitOpen.Store(true)
		v.lastErrMsg.Store(err.Error())
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) //nolint:errcheck
	open := resp.StatusCode != http.StatusOK
	v.circuitOpen.Store(open)
	return nil
}

func (v *VMAdapter) Status() BackendStatus {
	lastErr, _ := v.lastErrMsg.Load().(string)
	return BackendStatus{
		Name:        v.Name(),
		Type:        "victoriametrics",
		Healthy:     !v.circuitOpen.Load(),
		WriteRate:   v.rateTracker.rate(),
		ErrorRate:   float64(v.errorCount.Load()),
		BytesSent:   v.bytesTotal.Load(),
		CircuitOpen: v.circuitOpen.Load(),
		LastError:   lastErr,
	}
}

// WriteBatchSafe is identical to WriteBatch for VictoriaMetrics: the
// line-protocol /write endpoint is naturally idempotent (last-write-wins
// on the same timestamp+label set), so no special handling is needed.
func (v *VMAdapter) WriteBatchSafe(ctx context.Context, batch []DataPoint) error {
	return v.WriteBatch(ctx, batch)
}

func (v *VMAdapter) Close() error {
	v.client.CloseIdleConnections()
	return nil
}

// encodeLineProtocolGzip encodes batch as gzip-compressed InfluxDB line protocol.
// Format: measurement,tag1=v1,tag2=v2 field1=v1,field2=v2 timestamp_ns
func encodeLineProtocolGzip(batch []DataPoint) (io.Reader, int, error) {
	var buf bytes.Buffer
	gz, _ := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	skipped := 0
	for _, p := range batch {
		if p.Measurement == "" {
			skipped++
			continue
		}
		fmt.Fprint(gz, escLP(p.Measurement))
		for k, val := range p.Tags {
			fmt.Fprintf(gz, ",%s=%s", escLP(k), escLP(val))
		}
		gz.Write([]byte(" ")) //nolint:errcheck
		first := true
		for k, val := range p.Fields {
			if !first {
				gz.Write([]byte(",")) //nolint:errcheck
			}
			fmt.Fprintf(gz, "%s=%g", escLP(k), val)
			first = false
		}
		fmt.Fprintf(gz, " %d\n", p.Timestamp.UnixNano())
	}
	if err := gz.Close(); err != nil {
		return nil, 0, err
	}
	if skipped > 0 {
		log.Printf("tsdb [victoriametrics] skipped %d points with empty measurement (IEC104Type not set)", skipped)
	}
	return &buf, buf.Len(), nil
}

// escLP escapes spaces, commas, and equals signs in line protocol tag/measurement names.
func escLP(s string) string {
	var b bytes.Buffer
	for _, c := range s {
		if c == ' ' || c == ',' || c == '=' {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}
