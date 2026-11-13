package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	relayproto "github.com/shuaigehuang/codexlink/internal/relay"

	"github.com/coder/websocket"
)

const desktopTestToken = "desktop-test-token-0123456789abcdef"

func startRelayForDesktop(t *testing.T) *httptest.Server {
	t.Helper()
	service, err := relayproto.NewServer(relayproto.Config{
		DesktopToken: desktopTestToken,
		DataDir:      t.TempDir(),
		Logger:       log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(service.Handler())
	t.Cleanup(func() {
		service.Close()
		server.Close()
	})
	return server
}

func waitForDesktopRelay(t *testing.T, service *CodexLinkService) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !service.relayStatusSnapshot().Connected {
		if time.Now().After(deadline) {
			t.Fatalf("desktop did not connect: %+v", service.relayStatusSnapshot())
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestDesktopRelayPairingAndPing(t *testing.T) {
	relayServer := startRelayForDesktop(t)
	service := &CodexLinkService{}
	service.startRelay(relayConfig{
		URL:        relayServer.URL,
		Token:      desktopTestToken,
		DeviceID:   "desktop-test-01",
		DeviceName: "Test PC",
	})
	t.Cleanup(service.stopRelay)
	waitForDesktopRelay(t, service)

	pairing, err := service.CreateRelayPairing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(pairing.Code) != 8 {
		t.Fatalf("pairing code length = %d", len(pairing.Code))
	}
	if !strings.HasPrefix(pairing.QRDataURL, "data:image/png;base64,") {
		t.Fatalf("unexpected QR data URL: %.40s", pairing.QRDataURL)
	}
	if pairing.ExpiresAt == "" || !strings.Contains(pairing.Link, "codexlink://pair?") {
		t.Fatalf("incomplete pairing payload: %+v", pairing)
	}

	claimBody, _ := json.Marshal(relayproto.PairClaimRequest{Code: pairing.Code, Name: "Test Phone"})
	response, err := http.Post(relayServer.URL+relayproto.PathPairClaim, "application/json", bytes.NewReader(claimBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("claim status=%d body=%s", response.StatusCode, raw)
	}
	var claimed relayproto.PairClaimResponse
	if err := json.NewDecoder(response.Body).Decode(&claimed); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	mobile, _, err := websocket.Dial(ctx,
		strings.Replace(relayServer.URL, "http://", "ws://", 1)+relayproto.PathMobileConnect,
		&websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": []string{"Bearer " + claimed.Token}},
		})
	if err != nil {
		t.Fatal(err)
	}
	defer mobile.Close(websocket.StatusNormalClosure, "")

	hello := readRelayTestMessage(t, mobile)
	if hello.Type != relayproto.MessageTypeHello {
		t.Fatalf("unexpected hello: %+v", hello)
	}
	writeRelayTestMessage(t, mobile, relayproto.Message{
		Type:      relayproto.MessageTypeRPC,
		RequestID: "ping-1",
		Method:    "ping",
	})
	result := readRelayTestMessage(t, mobile)
	if !result.OK || result.RequestID != "ping-1" {
		t.Fatalf("unexpected ping result: %+v", result)
	}
	var payload map[string]any
	if err := json.Unmarshal(result.Result, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["pong"] != true {
		t.Fatalf("unexpected ping payload: %v", payload)
	}
}

func TestDesktopRelaySessionManagement(t *testing.T) {
	relayServer := startRelayForDesktop(t)
	service := &CodexLinkService{}
	service.startRelay(relayConfig{
		URL:        relayServer.URL,
		Token:      desktopTestToken,
		DeviceID:   "desktop-test-01",
		DeviceName: "Test PC",
	})
	t.Cleanup(service.stopRelay)
	waitForDesktopRelay(t, service)

	pairing, err := service.CreateRelayPairing(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	claimBody, _ := json.Marshal(relayproto.PairClaimRequest{Code: pairing.Code, Name: "Phone One"})
	response, err := http.Post(relayServer.URL+relayproto.PathPairClaim, "application/json", bytes.NewReader(claimBody))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(response.Body)
		t.Fatalf("claim status=%d body=%s", response.StatusCode, raw)
	}

	list, err := service.ListRelaySessions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if list.Count != 1 || len(list.Sessions) != 1 || list.Sessions[0].DeviceName != "Phone One" {
		t.Fatalf("unexpected authorized sessions: %+v", list)
	}
	if service.relayStatusSnapshot().PairedSessions != 1 {
		t.Fatalf("relay status was not refreshed: %+v", service.relayStatusSnapshot())
	}

	revoked, err := service.RevokeRelaySessions(context.Background(), "", true)
	if err != nil {
		t.Fatal(err)
	}
	if revoked.Count != 0 || len(revoked.Sessions) != 0 {
		t.Fatalf("sessions remain after revoke all: %+v", revoked)
	}
	if service.relayStatusSnapshot().PairedSessions != 0 {
		t.Fatalf("relay status count did not update: %+v", service.relayStatusSnapshot())
	}
}

func TestRelayConfigEncryptedRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relay.dat")
	store := &relayConfigStore{path: path}
	cfg := relayConfig{
		URL:        "https://relay.example.com",
		Token:      "0123456789abcdef0123456789abcdef",
		DeviceID:   "desktop-test-01",
		DeviceName: "Test PC",
	}
	if err := store.Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(cfg.Token)) {
		t.Fatal("relay token was stored as plaintext")
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Token != cfg.Token || loaded.DeviceID != cfg.DeviceID || loaded.URL != cfg.URL {
		t.Fatalf("round trip changed config: %+v", loaded)
	}
}

func TestNormalizeRelayURL(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"relay.example.com", "https://relay.example.com", false},
		{"https://relay.example.com/", "https://relay.example.com", false},
		{"https://relay.example.com/codex", "https://relay.example.com/codex", false},
		{"http://127.0.0.1:8787", "http://127.0.0.1:8787", false},
		{"http://relay.example.com", "http://relay.example.com", false},
		{"ftp://relay.example.com", "", true},
		{"https://user:pass@relay.example.com", "", true},
	}
	for _, test := range tests {
		got, err := normalizeRelayURL(test.input)
		if test.wantErr {
			if err == nil {
				t.Fatalf("%q was accepted as %q", test.input, got)
			}
			continue
		}
		if err != nil || got != test.want {
			t.Fatalf("normalize %q = %q, %v; want %q", test.input, got, err, test.want)
		}
	}
}

func TestRelayURLInsecure(t *testing.T) {
	tests := []struct {
		url  string
		want bool
	}{
		{"http://203.0.113.10", true},
		{"http://relay.example.com", true},
		{"http://127.0.0.1:8787", false},
		{"http://localhost:8787", false},
		{"https://203.0.113.10", false},
	}
	for _, test := range tests {
		if got := relayURLInsecure(test.url); got != test.want {
			t.Fatalf("relayURLInsecure(%q) = %v; want %v", test.url, got, test.want)
		}
	}
}

func TestRelayWebSocketURL(t *testing.T) {
	got, err := relayWebSocketURL("https://relay.example.com/base", "desktop-test-01", "My PC")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "wss://relay.example.com/base/v1/desktop/connect?") ||
		!strings.Contains(got, "device=desktop-test-01") ||
		!strings.Contains(got, "name=My+PC") {
		t.Fatalf("unexpected websocket URL: %s", got)
	}
}

func readRelayTestMessage(t *testing.T, conn *websocket.Conn) relayproto.Message {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, raw, err := conn.Read(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var message relayproto.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	return message
}

func writeRelayTestMessage(t *testing.T, conn *websocket.Conn, message relayproto.Message) {
	t.Helper()
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatal(err)
	}
}
