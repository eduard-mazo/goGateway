package config

import "os"

// DefaultHTTPListen can be overridden at build time with
//
//	-ldflags "-X goGateway/internal/config.DefaultHTTPListen=:9090"
//
// GW_HTTP env still wins at runtime.
var DefaultHTTPListen = ":8080"

// BuildTime and GitCommit are injected at link time via -ldflags.
// Defaults signal a local dev build not stamped by the Makefile.
var BuildTime = "dev"
var GitCommit = "unknown"

// Runtime config from env, with defaults.
type Runtime struct {
	DBPath     string
	HTTPListen string
	// JWTSecret is the HMAC-SHA256 signing key for access tokens.
	// Set GW_JWT_SECRET in production; the dev default must not be used in prod.
	JWTSecret string
}

func Load() Runtime {
	return Runtime{
		DBPath:     getenv("GW_DB", "gateway.db"),
		HTTPListen: getenv("GW_HTTP", DefaultHTTPListen),
		JWTSecret:  getenv("GW_JWT_SECRET", "change-me-in-production-min-32-chars"),
	}
}

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
