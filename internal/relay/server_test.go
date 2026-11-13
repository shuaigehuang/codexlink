package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
)

const testDesktopToken = "test-desktop-token-0123456789abcdef"

func newTestRelay(t *testing.T, pairTTL, sessionTTL time.Duration) (*Server, *httptest.Server) {
	t.Helper()
	server, err := NewServer(Config{
		DesktopToken: testDesktopToken,
		DataDir:      t.TempDir(),
		PairTTL:      pairTTL,
		SessionTTL:   sessionTTL,
		Logger:       testLogger(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(server.Handler())
	t.Cleanup(func() {
		server.Close()
		httpServer.Close()
	})
	return server, httpServer
}

func testLogger(t *testing.T) *log.Logger {
	t.Helper()
	return log.New(io.Discard, "", 0)
}

func wsURL(httpServer *httptest.Server, path string) string {
	return strings.Replace(httpServer.URL, "http://", "ws://", 1) + path
}

func dialWebSocket(t *testing.T, url string, token string) *websocket.Conn {
	t.Helper()
	header := http.Header{}
	if token != "" {
		header.Set("Authorization", "Bearer "+token)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, url, &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("dial websocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close(websocket.StatusNormalClosure, "")
	})
	return conn
}

func readWebSocket(t *testing.T, conn *websocket.Conn) Message {
	t.Helper()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, raw, err := conn.Read(ctx)
		cancel()
		if err != nil {
			t.Fatalf("read websocket: %v", err)
		}
		var message Message
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatalf("decode websocket message: %v", err)
		}
		if message.Type == MessageTypeSessions {
			continue
		}
		return message
	}
}

func writeWebSocket(t *testing.T, conn *websocket.Conn, message Message) {
	t.Helper()
	raw, err := json.Marshal(message)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, raw); err != nil {
		t.Fatalf("write websocket: %v", err)
	}
}

func connectDesktop(t *testing.T, httpServer *httptest.Server, deviceID string) *websocket.Conn {
	t.Helper()
	conn := dialWebSocket(t, wsURL(httpServer, PathDesktopConnect+"?device="+deviceID+"&name=Test+PC"), testDesktopToken)
	hello := readWebSocket(t, conn)
	if hello.Type != MessageTypeHello || hello.DeviceID != deviceID {
		t.Fatalf("unexpected desktop hello: %+v", hello)
	}
	return conn
}

func createPairCode(t *testing.T, desktop *websocket.Conn) Message {
	t.Helper()
	writeWebSocket(t, desktop, Message{Type: MessageTypePair, RequestID: "pair-request"})
	message := readWebSocket(t, desktop)
	if message.Type != MessageTypePaired || message.Code == "" {
		t.Fatalf("unexpected pair response: %+v", message)
	}
	return message
}

func claimPair(t *testing.T, httpServer *httptest.Server, code string) *http.Response {
	t.Helper()
	raw, err := json.Marshal(PairClaimRequest{Code: code, Name: "Test Phone"})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(httpServer.URL+PathPairClaim, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("claim pair: %v", err)
	}
	t.Cleanup(func() {
		_ = response.Body.Close()
	})
	return response
}

func claimPairToken(t *testing.T, httpServer *httptest.Server, code string) PairClaimResponse {
	t.Helper()
	response := claimPair(t, httpServer, code)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("claim pair status=%d body=%s", response.StatusCode, body)
	}
	var claimed PairClaimResponse
	if err := json.NewDecoder(response.Body).Decode(&claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.Token == "" || claimed.DeviceID == "" {
		t.Fatalf("incomplete pair response: %+v", claimed)
	}
	return claimed
}

func claimNamedPairToken(t *testing.T, httpServer *httptest.Server, code, name string) PairClaimResponse {
	t.Helper()
	raw, err := json.Marshal(PairClaimRequest{Code: code, Name: name})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(httpServer.URL+PathPairClaim, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("claim pair: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("claim pair status=%d body=%s", response.StatusCode, body)
	}
	var claimed PairClaimResponse
	if err := json.NewDecoder(response.Body).Decode(&claimed); err != nil {
		t.Fatal(err)
	}
	return claimed
}

func desktopManagementRequest(t *testing.T, httpServer *httptest.Server, method, path string, body any) *http.Response {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, httpServer.URL+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+testDesktopToken)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatalf("desktop management request: %v", err)
	}
	t.Cleanup(func() {
		_ = response.Body.Close()
	})
	return response
}

func listMobileSessions(t *testing.T, httpServer *httptest.Server, deviceID string) MobileSessionList {
	t.Helper()
	response := desktopManagementRequest(t, httpServer, http.MethodGet,
		PathDesktopSessions+"?device="+deviceID, nil)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("list sessions status=%d body=%s", response.StatusCode, body)
	}
	var list MobileSessionList
	if err := json.NewDecoder(response.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	if list.Count != len(list.Sessions) {
		t.Fatalf("session count mismatch: %+v", list)
	}
	return list
}

func revokeMobileSessions(t *testing.T, httpServer *httptest.Server, request RevokeSessionsRequest) RevokeSessionsResponse {
	t.Helper()
	response := desktopManagementRequest(t, httpServer, http.MethodPost, PathDesktopRevoke, request)
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("revoke sessions status=%d body=%s", response.StatusCode, body)
	}
	var revoked RevokeSessionsResponse
	if err := json.NewDecoder(response.Body).Decode(&revoked); err != nil {
		t.Fatal(err)
	}
	return revoked
}

func expectWebSocketClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, _, err := conn.Read(ctx); err == nil {
		t.Fatal("websocket remained open after session revoke")
	}
}

func TestHealth(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	response, err := http.Get(httpServer.URL + PathHealth)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var payload map[string]any
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["service"] != "codexlink-relay" || payload["ok"] != true {
		t.Fatalf("unexpected health payload: %v", payload)
	}
}

func TestDesktopRejectsBadToken(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, response, err := websocket.Dial(ctx,
		wsURL(httpServer, PathDesktopConnect+"?device=device-test-01"),
		&websocket.DialOptions{
			HTTPHeader: http.Header{"Authorization": []string{"Bearer wrong-token"}},
		})
	if err == nil {
		t.Fatal("bad token was accepted")
	}
	if response == nil || response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %+v", response)
	}
}

func TestPairingAndRPCRoundTrip(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	claimed := claimPairToken(t, httpServer, pair.Code)

	secondClaim := claimPair(t, httpServer, pair.Code)
	if secondClaim.StatusCode != http.StatusGone {
		t.Fatalf("pairing code was reusable, status=%d", secondClaim.StatusCode)
	}

	mobile := dialWebSocket(t,
		wsURL(httpServer, PathMobileConnect+"?device=device-test-01"),
		claimed.Token)
	hello := readWebSocket(t, mobile)
	if hello.Type != MessageTypeHello || hello.DeviceID != "device-test-01" {
		t.Fatalf("unexpected mobile hello: %+v", hello)
	}

	writeWebSocket(t, mobile, Message{
		Type:      MessageTypeRPC,
		RequestID: "mobile-request-1",
		Method:    "threads.list",
		Params:    json.RawMessage(`{"limit":20}`),
	})
	relayed := readWebSocket(t, desktop)
	if relayed.Type != MessageTypeRPC || relayed.Method != "threads.list" || relayed.RequestID == "" {
		t.Fatalf("unexpected relayed request: %+v", relayed)
	}
	if string(relayed.Params) != `{"limit":20}` {
		t.Fatalf("params changed in transit: %s", relayed.Params)
	}

	writeWebSocket(t, desktop, Message{
		Type:      MessageTypeRPCResult,
		RequestID: relayed.RequestID,
		OK:        true,
		Result:    json.RawMessage(`{"threads":[]}`),
	})
	result := readWebSocket(t, mobile)
	if result.Type != MessageTypeRPCResult || result.RequestID != "mobile-request-1" || !result.OK {
		t.Fatalf("unexpected rpc result: %+v", result)
	}
	if string(result.Result) != `{"threads":[]}` {
		t.Fatalf("result changed in transit: %s", result.Result)
	}
}

func TestHTTPMobileRPCRoundTrip(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	claimed := claimPairToken(t, httpServer, pair.Code)

	type httpReply struct {
		status int
		body   Message
		err    error
	}
	done := make(chan httpReply, 1)
	go func() {
		raw, _ := json.Marshal(map[string]any{
			"method": "threads.list",
			"params": map[string]any{"limit": 20},
		})
		request, err := http.NewRequest(http.MethodPost, httpServer.URL+PathMobileRPC, bytes.NewReader(raw))
		if err != nil {
			done <- httpReply{err: err}
			return
		}
		request.Header.Set("Authorization", "Bearer "+claimed.Token)
		request.Header.Set("Content-Type", "application/json")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			done <- httpReply{err: err}
			return
		}
		defer response.Body.Close()
		var message Message
		if err := json.NewDecoder(response.Body).Decode(&message); err != nil {
			done <- httpReply{status: response.StatusCode, err: err}
			return
		}
		done <- httpReply{status: response.StatusCode, body: message}
	}()

	relayed := readWebSocket(t, desktop)
	if relayed.Type != MessageTypeRPC || relayed.Method != "threads.list" {
		t.Fatalf("unexpected relayed request: %+v", relayed)
	}
	if string(relayed.Params) != `{"limit":20}` {
		t.Fatalf("params changed in transit: %s", relayed.Params)
	}
	writeWebSocket(t, desktop, Message{
		Type:      MessageTypeRPCResult,
		RequestID: relayed.RequestID,
		OK:        true,
		Result:    json.RawMessage(`{"threads":[]}`),
	})

	reply := <-done
	if reply.err != nil {
		t.Fatal(reply.err)
	}
	if reply.status != http.StatusOK || !reply.body.OK || string(reply.body.Result) != `{"threads":[]}` {
		t.Fatalf("unexpected HTTP RPC reply: status=%d body=%+v", reply.status, reply.body)
	}
}

func TestHTTPMobileRPCDesktopOffline(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	claimed := claimPairToken(t, httpServer, pair.Code)
	_ = desktop.Close(websocket.StatusNormalClosure, "")

	raw, _ := json.Marshal(map[string]any{"method": "node.status"})
	request, err := http.NewRequest(http.MethodPost, httpServer.URL+PathMobileRPC, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+claimed.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload["error"] != ErrorDesktopOffline {
		t.Fatalf("unexpected error: %v", payload)
	}
}

func TestMobileMethodWhitelist(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	claimed := claimPairToken(t, httpServer, pair.Code)
	mobile := dialWebSocket(t, wsURL(httpServer, PathMobileConnect), claimed.Token)
	_ = readWebSocket(t, mobile)

	writeWebSocket(t, mobile, Message{
		Type:      MessageTypeRPC,
		RequestID: "blocked-1",
		Method:    "magpie.admin.keys",
	})
	result := readWebSocket(t, mobile)
	if result.OK || result.RequestID != "blocked-1" || result.Error != ErrorMethodBlocked {
		t.Fatalf("blocked method was not rejected: %+v", result)
	}

	writeWebSocket(t, mobile, Message{
		Type:      MessageTypeRPC,
		RequestID: "allowed-1",
		Method:    "ping",
	})
	relayed := readWebSocket(t, desktop)
	if relayed.Method != "ping" {
		t.Fatalf("allowed method was not relayed: %+v", relayed)
	}
}

func TestPairCodeExpires(t *testing.T) {
	_, httpServer := newTestRelay(t, 40*time.Millisecond, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	time.Sleep(80 * time.Millisecond)
	response := claimPair(t, httpServer, pair.Code)
	if response.StatusCode != http.StatusGone {
		t.Fatalf("expired code status = %d", response.StatusCode)
	}
}

func TestPairCodeWaitsForDesktop(t *testing.T) {
	server, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	_ = desktop.Close(websocket.StatusNormalClosure, "")
	waitFor(t, func() bool { return server.desktopFor("device-test-01") == nil })

	response := claimPair(t, httpServer, pair.Code)
	if response.StatusCode != http.StatusConflict {
		t.Fatalf("offline desktop status = %d", response.StatusCode)
	}

	_ = connectDesktop(t, httpServer, "device-test-01")
	claimed := claimPairToken(t, httpServer, pair.Code)
	if claimed.DeviceID != "device-test-01" {
		t.Fatalf("unexpected claimed device: %+v", claimed)
	}
}

func TestMobileGetsDesktopOffline(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")
	pair := createPairCode(t, desktop)
	claimed := claimPairToken(t, httpServer, pair.Code)
	_ = desktop.Close(websocket.StatusNormalClosure, "")

	mobile := dialWebSocket(t, wsURL(httpServer, PathMobileConnect), claimed.Token)
	_ = readWebSocket(t, mobile)
	writeWebSocket(t, mobile, Message{
		Type:      MessageTypeRPC,
		RequestID: "offline-1",
		Method:    "ping",
	})
	result := readWebSocket(t, mobile)
	if result.OK || result.Error != ErrorDesktopOffline || result.RequestID != "offline-1" {
		t.Fatalf("unexpected offline response: %+v", result)
	}
}

func TestPairClaimRateLimit(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	for attempt := 0; attempt < pairAttemptLimit; attempt++ {
		response := claimPair(t, httpServer, fmt.Sprintf("INVALID%d", attempt))
		if response.StatusCode != http.StatusBadRequest && response.StatusCode != http.StatusGone {
			t.Fatalf("attempt %d status = %d", attempt, response.StatusCode)
		}
	}
	response := claimPair(t, httpServer, "INVALIDX")
	if response.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("rate limit status = %d", response.StatusCode)
	}
}

func TestPairCodeNormalization(t *testing.T) {
	code, err := randomPairCode()
	if err != nil {
		t.Fatal(err)
	}
	if normalizePairCode(strings.ToLower(code)) != code {
		t.Fatalf("lowercase code did not normalize: %s", code)
	}
	spaced := code[:4] + "-" + code[4:]
	if normalizePairCode(spaced) != code {
		t.Fatalf("hyphenated code did not normalize: %s", spaced)
	}
}

func TestSessionStoreRenameFallback(t *testing.T) {
	original := renameSessionFile
	renameSessionFile = func(string, string) error { return errors.New("simulated rename failure") }
	t.Cleanup(func() { renameSessionFile = original })

	dir := t.TempDir()
	store, err := newSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	record := sessionRecord{
		TokenHash:  hashToken("session-token"),
		DeviceID:   "device-test-01",
		DeviceName: "Test Phone",
		ExpiresAt:  time.Now().Add(time.Hour).UTC().Format(time.RFC3339),
	}
	if err := store.Add(record); err != nil {
		t.Fatal(err)
	}
	reloaded, err := newSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reloaded.Lookup(record.TokenHash); !ok {
		t.Fatal("session was not persisted through fallback")
	}
}

func TestDesktopSessionManagement(t *testing.T) {
	_, httpServer := newTestRelay(t, 0, 0)
	desktop := connectDesktop(t, httpServer, "device-test-01")

	pairA := createPairCode(t, desktop)
	phoneA := claimNamedPairToken(t, httpServer, pairA.Code, "Phone A")
	pairB := createPairCode(t, desktop)
	phoneB := claimNamedPairToken(t, httpServer, pairB.Code, "Phone B")

	unauthorized, err := http.Get(httpServer.URL + PathDesktopSessions + "?device=device-test-01")
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthenticated list status = %d", unauthorized.StatusCode)
	}

	list := listMobileSessions(t, httpServer, "device-test-01")
	if list.Count != 2 {
		t.Fatalf("expected 2 mobile sessions, got %+v", list)
	}
	byName := make(map[string]MobileSession)
	for _, session := range list.Sessions {
		if session.ID == "" {
			t.Fatalf("session has no management id: %+v", session)
		}
		if session.CreatedAt == "" || session.ExpiresAt == "" {
			t.Fatalf("session timestamps missing: %+v", session)
		}
		byName[session.DeviceName] = session
	}
	if byName["Phone A"].ID == "" || byName["Phone B"].ID == "" {
		t.Fatalf("unexpected session names: %+v", list.Sessions)
	}

	crossDevice := revokeMobileSessions(t, httpServer, RevokeSessionsRequest{
		DeviceID:  "device-test-02",
		SessionID: byName["Phone A"].ID,
	})
	if crossDevice.Revoked != 0 {
		t.Fatalf("cross-device revoke removed sessions: %+v", crossDevice)
	}
	if remaining := listMobileSessions(t, httpServer, "device-test-01"); remaining.Count != 2 {
		t.Fatalf("cross-device revoke changed sessions: %+v", remaining)
	}

	mobileA := dialWebSocket(t, wsURL(httpServer, PathMobileConnect), phoneA.Token)
	_ = readWebSocket(t, mobileA)
	mobileB := dialWebSocket(t, wsURL(httpServer, PathMobileConnect), phoneB.Token)
	_ = readWebSocket(t, mobileB)
	waitFor(t, func() bool {
		active := listMobileSessions(t, httpServer, "device-test-01")
		return active.Count == 2 && active.Sessions[0].Active && active.Sessions[1].Active
	})

	revokedA := revokeMobileSessions(t, httpServer, RevokeSessionsRequest{
		DeviceID:  "device-test-01",
		SessionID: byName["Phone A"].ID,
	})
	if revokedA.Revoked != 1 {
		t.Fatalf("expected one revoked session, got %+v", revokedA)
	}
	expectWebSocketClosed(t, mobileA)
	remaining := listMobileSessions(t, httpServer, "device-test-01")
	if remaining.Count != 1 || remaining.Sessions[0].DeviceName != "Phone B" {
		t.Fatalf("unexpected remaining sessions: %+v", remaining)
	}

	writeWebSocket(t, mobileB, Message{
		Type:      MessageTypeRPC,
		RequestID: "phone-b-ping",
		Method:    "ping",
	})
	relayed := readWebSocket(t, desktop)
	if relayed.Type != MessageTypeRPC || relayed.Method != "ping" {
		t.Fatalf("remaining phone lost access: %+v", relayed)
	}
	writeWebSocket(t, desktop, Message{
		Type:      MessageTypeRPCResult,
		RequestID: relayed.RequestID,
		OK:        true,
		Result:    json.RawMessage(`{"pong":true}`),
	})
	if result := readWebSocket(t, mobileB); !result.OK {
		t.Fatalf("remaining phone rpc failed: %+v", result)
	}

	raw, _ := json.Marshal(map[string]any{"method": "node.status"})
	request, err := http.NewRequest(http.MethodPost, httpServer.URL+PathMobileRPC, bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Authorization", "Bearer "+phoneA.Token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked token HTTP status = %d", response.StatusCode)
	}

	revokedAll := revokeMobileSessions(t, httpServer, RevokeSessionsRequest{
		DeviceID: "device-test-01",
		All:      true,
	})
	if revokedAll.Revoked != 1 {
		t.Fatalf("expected one session revoked by all, got %+v", revokedAll)
	}
	expectWebSocketClosed(t, mobileB)
	if final := listMobileSessions(t, httpServer, "device-test-01"); final.Count != 0 {
		t.Fatalf("sessions remain after revoke all: %+v", final)
	}
}

func TestSessionStoreMigratesLegacySessions(t *testing.T) {
	dir := t.TempDir()
	legacy := []sessionRecord{{
		TokenHash:  hashToken("legacy-phone-token"),
		DeviceID:   "device-test-01",
		DeviceName: "Old Phone",
		ExpiresAt:  time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339),
	}}
	raw, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := newSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	records := store.ListDevice("device-test-01")
	if len(records) != 1 {
		t.Fatalf("legacy session missing: %+v", records)
	}
	wantID := sessionIDForToken(legacy[0].TokenHash)
	if records[0].SessionID != wantID {
		t.Fatalf("legacy session id = %q, want %q", records[0].SessionID, wantID)
	}
	reloaded, err := newSessionStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.ListDevice("device-test-01"); len(got) != 1 || got[0].SessionID != wantID {
		t.Fatalf("legacy session migration was not persisted: %+v", got)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition was not met before timeout")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
