package main

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	env := map[string]string{
		"CODEXLINK_RELAY_DESKTOP_TOKEN": "0123456789abcdef0123456789abcdef",
	}
	cfg, listen, err := loadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if listen != defaultListen || cfg.DataDir != "./data" {
		t.Fatalf("unexpected defaults: listen=%s data=%s", listen, cfg.DataDir)
	}
	if cfg.PairTTL != 10*time.Minute || cfg.SessionTTL != 90*24*time.Hour {
		t.Fatalf("unexpected TTL defaults: %v %v", cfg.PairTTL, cfg.SessionTTL)
	}
}

func TestLoadConfigRejectsShortToken(t *testing.T) {
	env := map[string]string{"CODEXLINK_RELAY_DESKTOP_TOKEN": "short"}
	if _, _, err := loadConfig(func(key string) string { return env[key] }); err == nil {
		t.Fatal("short token was accepted")
	}
}

func TestLoadConfigOverrides(t *testing.T) {
	env := map[string]string{
		"CODEXLINK_RELAY_DESKTOP_TOKEN": "0123456789abcdef0123456789abcdef",
		"CODEXLINK_RELAY_DATA_DIR":      "/var/lib/codexlink",
		"CODEXLINK_RELAY_LISTEN":        "127.0.0.1:9999",
		"CODEXLINK_RELAY_PAIR_TTL":      "5m",
		"CODEXLINK_RELAY_SESSION_TTL":   "24h",
		"CODEXLINK_RELAY_TRUST_PROXY":   "1",
		"CODEXLINK_RELAY_ORIGINS":       "https://a.example, https://b.example",
	}
	cfg, listen, err := loadConfig(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	if listen != "127.0.0.1:9999" || cfg.DataDir != "/var/lib/codexlink" {
		t.Fatalf("overrides not applied: %+v", cfg)
	}
	if cfg.PairTTL != 5*time.Minute || cfg.SessionTTL != 24*time.Hour || !cfg.TrustProxy {
		t.Fatalf("duration or proxy overrides not applied: %+v", cfg)
	}
	if len(cfg.AllowedOrigins) != 2 || cfg.AllowedOrigins[1] != "https://b.example" {
		t.Fatalf("origins not parsed: %v", cfg.AllowedOrigins)
	}
}

func TestLoadTLSOptions(t *testing.T) {
	env := map[string]string{
		"CODEXLINK_RELAY_TLS_DOMAIN": "Relay.Example.com",
		"CODEXLINK_RELAY_TLS_EMAIL":  "admin@example.com",
	}
	options := loadTLSOptions(func(key string) string { return env[key] })
	if options.Domain != "relay.example.com" || options.Email != "admin@example.com" || options.HTTPAddr != ":80" {
		t.Fatalf("unexpected TLS options: %+v", options)
	}
	manager, config := newAutocertManager(options, t.TempDir())
	if manager == nil || config == nil || config.MinVersion == 0 {
		t.Fatal("autocert manager was not configured")
	}
	if !strings.Contains(strings.Join(config.NextProtos, ","), "http/1.1") {
		t.Fatalf("HTTP/1.1 was not advertised for WebSocket compatibility: %v", config.NextProtos)
	}
}
