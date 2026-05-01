package config

import "os"

// DefaultHTTPListen can be overridden at build time with
//   -ldflags "-X goGateway/internal/config.DefaultHTTPListen=:9090"
// GW_HTTP env still wins at runtime.
var DefaultHTTPListen = ":8080"

// Runtime config from env, with defaults.
type Runtime struct {
	DBPath     string
	HTTPListen string
	TSDB       TSDBConfig
}

// TSDBConfig holds optional time-series database settings.
// Both fields are empty by default; a backend is only started when its
// connection string is set via environment variable.
type TSDBConfig struct {
	// VictoriaMetrics write endpoint, e.g. http://vm-host:8428
	VMUrl string
	// TimescaleDB DSN, e.g. postgres://user:pass@host:5432/gateway
	TimescaleDSN string
	// WAL and DLQ paths (relative to working directory)
	WALPath string
	DLQPath string
}

func Load() Runtime {
	return Runtime{
		DBPath:     getenv("GW_DB", "gateway.db"),
		HTTPListen: getenv("GW_HTTP", DefaultHTTPListen),
		TSDB: TSDBConfig{
			VMUrl:        os.Getenv("GW_VM_URL"),
			TimescaleDSN: os.Getenv("GW_TIMESCALE_DSN"),
			WALPath:      getenv("GW_TSDB_WAL", "data/wal.bolt"),
			DLQPath:      getenv("GW_TSDB_DLQ", "data/dlq.bolt"),
		},
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
