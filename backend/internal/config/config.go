package config

import "os"

// DefaultHTTPListen can be overridden at build time with
//
//	-ldflags "-X goGateway/internal/config.DefaultHTTPListen=:9090"
//
// GW_HTTP env still wins at runtime.
var DefaultHTTPListen = ":8080"

// Runtime config from env, with defaults.
type Runtime struct {
	DBPath     string
	HTTPListen string
}

func Load() Runtime {
	return Runtime{
		DBPath:     getenv("GW_DB", "gateway.db"),
		HTTPListen: getenv("GW_HTTP", DefaultHTTPListen),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
