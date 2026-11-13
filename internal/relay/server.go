package relay

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

const (
	defaultPairTTL    = 10 * time.Minute
	defaultSessionTTL = 90 * 24 * time.Hour
	maxMessageBytes   = 8 << 20
	maxPendingPerPeer = 32
	httpRPCTimeout    = 2 * time.Minute
	pairAttemptLimit  = 10
	pairAttemptWindow = 10 * time.Minute
)

type Config struct {
	DesktopToken   string
	AdminToken     string
	PublicURLs     []string
	DataDir        string
	PairTTL        time.Duration
	SessionTTL     time.Duration
	AllowedOrigins []string
	TrustProxy     bool
	Logger         *log.Logger
}

type Server struct {
	cfg       Config
	mux       *http.ServeMux
	store     *sessionStore
	tokens    *desktopTokenStore
	mu        sync.Mutex
	desktops  map[string]*peer
	mobiles   map[*peer]struct{}
	pending   map[string]pendingRoute
	pairs     map[string]pairRecord
	attempts  map[string]attemptWindow
	adminMu   sync.Mutex
	admins    map[string]adminSession
	closed    bool
	closeOnce sync.Once
}

type peer struct {
	conn      *websocket.Conn
	kind      string
	deviceID  string
	name      string
	sessionID string
	tokenHash string
	tokenID   string
	writeMu   sync.Mutex
}

type pendingRoute struct {
	deviceID      string
	mobile        *peer
	mobileRequest string
	method        string
	resultCh      chan Message
	expiresAt     time.Time
}

type pairRecord struct {
	deviceID  string
	expiresAt time.Time
}

type attemptWindow struct {
	count   int
	resetAt time.Time
}

func NewServer(cfg Config) (*Server, error) {
	cfg.DesktopToken = strings.TrimSpace(cfg.DesktopToken)
	if cfg.DesktopToken == "" {
		return nil, errors.New("desktop token is required")
	}
	cfg.AdminToken = strings.TrimSpace(cfg.AdminToken)
	if cfg.AdminToken != "" && len(cfg.AdminToken) < minDesktopTokenLen {
		return nil, errors.New("admin token must be at least 32 characters")
	}
	cfg.PublicURLs = normalizePublicURLs(cfg.PublicURLs)
	if strings.TrimSpace(cfg.DataDir) == "" {
		cfg.DataDir = "data"
	}
	if cfg.PairTTL <= 0 {
		cfg.PairTTL = defaultPairTTL
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = defaultSessionTTL
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	store, err := newSessionStore(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("load relay sessions: %w", err)
	}
	tokens, err := newDesktopTokenStore(cfg.DataDir, cfg.DesktopToken)
	if err != nil {
		return nil, fmt.Errorf("load desktop tokens: %w", err)
	}
	server := &Server{
		cfg:      cfg,
		store:    store,
		tokens:   tokens,
		desktops: make(map[string]*peer),
		mobiles:  make(map[*peer]struct{}),
		pending:  make(map[string]pendingRoute),
		pairs:    make(map[string]pairRecord),
		attempts: make(map[string]attemptWindow),
		admins:   make(map[string]adminSession),
	}
	server.mux = http.NewServeMux()
	server.mux.HandleFunc(PathHealth, server.handleHealth)
	server.mux.HandleFunc(PathDesktopConnect, server.handleDesktop)
	server.mux.HandleFunc(PathDesktopSessions, server.handleDesktopSessions)
	server.mux.HandleFunc(PathDesktopRevoke, server.handleDesktopRevoke)
	server.mux.HandleFunc(PathMobileConnect, server.handleMobile)
	server.mux.HandleFunc(PathMobileRPC, server.handleMobileRPC)
	server.mux.HandleFunc(PathPairClaim, server.handlePairClaim)
	server.mux.HandleFunc(PathAdmin, server.handleAdmin)
	server.mux.HandleFunc(PathAdmin+"/login", server.handleAdminLogin)
	server.mux.HandleFunc(PathAdmin+"/logout", server.handleAdminLogout)
	server.mux.HandleFunc(PathAdmin+"/api/state", server.handleAdminState)
	server.mux.HandleFunc(PathAdmin+"/api/tokens", server.handleAdminTokens)
	server.mux.HandleFunc(PathAdmin+"/api/tokens/", server.handleAdminTokenDelete)
	return server, nil
}

func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		s.mux.ServeHTTP(w, r)
	})
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		peers := make([]*peer, 0, len(s.desktops)+len(s.mobiles))
		for _, desktop := range s.desktops {
			peers = append(peers, desktop)
		}
		for mobile := range s.mobiles {
			peers = append(peers, mobile)
		}
		s.mu.Unlock()
		for _, item := range peers {
			_ = item.conn.Close(websocket.StatusGoingAway, "relay shutting down")
		}
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	s.mu.Lock()
	desktops := len(s.desktops)
	mobiles := len(s.mobiles)
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"service":        "codexlink-relay",
		"activeDesktops": desktops,
		"activeMobiles":  mobiles,
		"time":           time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *Server) handleDesktop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	token, ok := s.tokens.Authenticate(bearerToken(r))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device"))
	if !validDeviceID(deviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}
	name := limitText(r.URL.Query().Get("name"), 80)
	conn, err := s.acceptWebSocket(w, r)
	if err != nil {
		return
	}
	desktop := &peer{conn: conn, kind: "desktop", deviceID: deviceID, name: name, tokenID: token.ID}
	s.registerDesktop(desktop)
	defer s.unregisterDesktop(desktop)
	if err := s.tokens.MarkUsed(token.ID); err != nil {
		s.cfg.Logger.Printf("desktop token last-used update failed id=%s err=%v", token.ID, err)
	}
	s.cfg.Logger.Printf("desktop connected device=%s name=%q", deviceID, name)

	_ = desktop.send(r.Context(), Message{
		Type:           MessageTypeHello,
		DeviceID:       deviceID,
		DeviceName:     name,
		PairedSessions: s.store.CountDevice(deviceID),
	})

	for {
		message, err := readMessage(r.Context(), conn)
		if err != nil {
			s.cfg.Logger.Printf("desktop read ended device=%s status=%d err=%v", deviceID, websocket.CloseStatus(err), err)
			return
		}
		switch message.Type {
		case MessageTypePair:
			if message.RequestID == "" {
				_ = desktop.send(r.Context(), Message{Type: MessageTypeError, Error: ErrorBadRequest})
				continue
			}
			code, expires, err := s.createPairCode(deviceID)
			if err != nil {
				_ = desktop.send(r.Context(), Message{Type: MessageTypeError, RequestID: message.RequestID, Error: err.Error()})
				continue
			}
			_ = desktop.send(r.Context(), Message{
				Type:      MessageTypePaired,
				RequestID: message.RequestID,
				Code:      code,
				ExpiresAt: expires,
			})
		case MessageTypeRPCResult:
			s.routeRPCResult(desktop, message)
		default:
			_ = desktop.send(r.Context(), Message{Type: MessageTypeError, Error: "unsupported desktop message"})
		}
	}
}

func (s *Server) handleDesktopSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, ok := s.tokens.Authenticate(bearerToken(r)); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	deviceID := strings.TrimSpace(r.URL.Query().Get("device"))
	if !validDeviceID(deviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}
	records := s.store.ListDevice(deviceID)
	active := s.activeMobileSessions(deviceID)
	sessions := make([]MobileSession, 0, len(records))
	for _, record := range records {
		sessions = append(sessions, MobileSession{
			ID:         record.SessionID,
			DeviceName: record.DeviceName,
			CreatedAt:  record.CreatedAt,
			ExpiresAt:  record.ExpiresAt,
			Active:     active[record.SessionID],
		})
	}
	writeJSON(w, http.StatusOK, MobileSessionList{
		Sessions: sessions,
		Count:    len(sessions),
	})
}

func (s *Server) handleDesktopRevoke(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if _, ok := s.tokens.Authenticate(bearerToken(r)); !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ErrorBadRequest})
		return
	}
	var request RevokeSessionsRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ErrorBadRequest})
		return
	}
	request.DeviceID = strings.TrimSpace(request.DeviceID)
	request.SessionID = strings.TrimSpace(request.SessionID)
	if !validDeviceID(request.DeviceID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid device id"})
		return
	}
	if !request.All && !validSessionID(request.SessionID) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid session id"})
		return
	}
	revoked, err := s.store.Revoke(request.DeviceID, request.SessionID, request.All)
	if err != nil {
		s.cfg.Logger.Printf("session revoke failed device=%s all=%v err=%v", request.DeviceID, request.All, err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not persist session changes"})
		return
	}
	s.disconnectMobileSessions(request.DeviceID, revoked)
	s.notifyDesktopSessionCount(request.DeviceID)
	scope := "session"
	if request.All {
		scope = "all"
	}
	s.cfg.Logger.Printf("desktop revoked mobile sessions device=%s scope=%s count=%d remote=%s",
		request.DeviceID, scope, len(revoked), requestIP(r, s.cfg.TrustProxy))
	writeJSON(w, http.StatusOK, RevokeSessionsResponse{Revoked: len(revoked)})
}

func (s *Server) handleMobile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	token := bearerToken(r)
	record, ok := s.store.Lookup(hashToken(token))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}
	if deviceID := strings.TrimSpace(r.URL.Query().Get("device")); deviceID != "" && deviceID != record.DeviceID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device mismatch"})
		return
	}
	conn, err := s.acceptWebSocket(w, r)
	if err != nil {
		return
	}
	mobile := &peer{
		conn:      conn,
		kind:      "mobile",
		deviceID:  record.DeviceID,
		name:      record.DeviceName,
		sessionID: record.SessionID,
		tokenHash: hashToken(token),
	}
	s.registerMobile(mobile)
	defer s.unregisterMobile(mobile)
	s.cfg.Logger.Printf("mobile connected device=%s name=%q", mobile.deviceID, mobile.name)

	_ = mobile.send(r.Context(), Message{
		Type:       MessageTypeHello,
		DeviceID:   record.DeviceID,
		DeviceName: record.DeviceName,
	})

	for {
		message, err := readMessage(r.Context(), conn)
		if err != nil {
			s.cfg.Logger.Printf("mobile read ended device=%s status=%d err=%v", mobile.deviceID, websocket.CloseStatus(err), err)
			return
		}
		if message.Type != MessageTypeRPC {
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, "unsupported mobile message"))
			continue
		}
		if message.RequestID == "" || !allowedMobileMethods[message.Method] {
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, ErrorMethodBlocked))
			continue
		}
		if len(message.Params) > 1<<20 {
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, "request too large"))
			continue
		}
		s.cfg.Logger.Printf("mobile rpc device=%s method=%s transport=ws", mobile.deviceID, message.Method)
		desktop := s.desktopFor(record.DeviceID)
		if desktop == nil {
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, ErrorDesktopOffline))
			continue
		}
		relayRequestID := randomID(18)
		s.mu.Lock()
		s.cleanupPendingLocked(time.Now())
		if s.countPendingForMobileLocked(mobile) >= maxPendingPerPeer {
			s.mu.Unlock()
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, "too many pending requests"))
			continue
		}
		s.pending[relayRequestID] = pendingRoute{
			deviceID:      record.DeviceID,
			mobile:        mobile,
			mobileRequest: message.RequestID,
			method:        message.Method,
			expiresAt:     time.Now().Add(httpRPCTimeout),
		}
		s.mu.Unlock()
		if err := desktop.send(r.Context(), Message{
			Type:      MessageTypeRPC,
			RequestID: relayRequestID,
			Method:    message.Method,
			Params:    message.Params,
		}); err != nil {
			s.removePending(relayRequestID)
			_ = mobile.send(r.Context(), rpcResultMessage(message.RequestID, false, nil, ErrorDesktopOffline))
		}
	}
}

func (s *Server) handleMobileRPC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	token := bearerToken(r)
	record, ok := s.store.Lookup(hashToken(token))
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": ErrorUnauthorized})
		return
	}
	if deviceID := strings.TrimSpace(r.URL.Query().Get("device")); deviceID != "" && deviceID != record.DeviceID {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "device mismatch"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ErrorBadRequest})
		return
	}
	var request struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": ErrorBadRequest})
		return
	}
	request.Method = strings.TrimSpace(request.Method)
	if !allowedMobileMethods[request.Method] {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": ErrorMethodBlocked})
		return
	}
	if len(request.Params) > 1<<20 {
		writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request too large"})
		return
	}
	desktop := s.desktopFor(record.DeviceID)
	if desktop == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": ErrorDesktopOffline})
		return
	}

	resultCh := make(chan Message, 1)
	relayRequestID := randomID(18)
	s.mu.Lock()
	s.cleanupPendingLocked(time.Now())
	if s.countPendingForDeviceLocked(record.DeviceID) >= maxPendingPerPeer {
		s.mu.Unlock()
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many pending requests"})
		return
	}
	s.pending[relayRequestID] = pendingRoute{
		deviceID:  record.DeviceID,
		method:    request.Method,
		resultCh:  resultCh,
		expiresAt: time.Now().Add(httpRPCTimeout),
	}
	s.mu.Unlock()
	s.cfg.Logger.Printf("mobile rpc device=%s method=%s transport=http", record.DeviceID, request.Method)

	if err := desktop.send(r.Context(), Message{
		Type:      MessageTypeRPC,
		RequestID: relayRequestID,
		Method:    request.Method,
		Params:    request.Params,
	}); err != nil {
		s.removePending(relayRequestID)
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": ErrorDesktopOffline})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), httpRPCTimeout)
	defer cancel()
	select {
	case result := <-resultCh:
		writeJSONCompressed(w, r, http.StatusOK, result)
	case <-ctx.Done():
		s.removePending(relayRequestID)
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "desktop request timed out"})
		}
	}
}

func (s *Server) handlePairClaim(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	remoteIP := requestIP(r, s.cfg.TrustProxy)
	if !s.allowPairAttempt(remoteIP) {
		writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many pairing attempts"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 8<<10))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	var request PairClaimRequest
	if err := json.Unmarshal(body, &request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
		return
	}
	code := normalizePairCode(request.Code)
	if code == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pairing code is required"})
		return
	}
	s.mu.Lock()
	pair, ok := s.pairs[code]
	now := time.Now()
	if ok && !pair.expiresAt.After(now) {
		delete(s.pairs, code)
		ok = false
	}
	desktopOnline := ok && pair.deviceID != "" && s.desktops[pair.deviceID] != nil
	if ok && desktopOnline {
		delete(s.pairs, code)
	}
	s.mu.Unlock()
	if !ok {
		writeJSON(w, http.StatusGone, map[string]string{"error": "pairing code expired or invalid"})
		return
	}
	if !desktopOnline {
		writeJSON(w, http.StatusConflict, map[string]string{"error": ErrorDesktopOffline})
		return
	}

	token := randomToken(32)
	expires := time.Now().UTC().Add(s.cfg.SessionTTL)
	record := sessionRecord{
		TokenHash:  hashToken(token),
		SessionID:  randomID(12),
		DeviceID:   pair.deviceID,
		DeviceName: limitText(request.Name, 80),
		CreatedAt:  time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:  expires.Format(time.RFC3339),
	}
	if err := s.store.Add(record); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not persist session"})
		return
	}
	s.notifyDesktopSessionCount(pair.deviceID)
	s.clearPairAttempts(remoteIP)
	writeJSON(w, http.StatusOK, PairClaimResponse{
		Token:      token,
		DeviceID:   pair.deviceID,
		DeviceName: record.DeviceName,
		ExpiresAt:  record.ExpiresAt,
	})
}

func (s *Server) acceptWebSocket(w http.ResponseWriter, r *http.Request) (*websocket.Conn, error) {
	options := &websocket.AcceptOptions{
		OriginPatterns: s.cfg.AllowedOrigins,
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return nil, err
	}
	conn.SetReadLimit(maxMessageBytes)
	return conn, nil
}

func (s *Server) registerDesktop(desktop *peer) {
	s.mu.Lock()
	previous := s.desktops[desktop.deviceID]
	s.desktops[desktop.deviceID] = desktop
	s.mu.Unlock()
	if previous != nil && previous != desktop {
		_ = previous.conn.Close(websocket.StatusGoingAway, "replaced by a newer connection")
	}
}

func (s *Server) unregisterDesktop(desktop *peer) {
	s.mu.Lock()
	if s.desktops[desktop.deviceID] == desktop {
		delete(s.desktops, desktop.deviceID)
	}
	routes := make([]pendingRoute, 0)
	for requestID, route := range s.pending {
		if route.deviceID == desktop.deviceID {
			delete(s.pending, requestID)
			routes = append(routes, route)
		}
	}
	s.mu.Unlock()
	for _, route := range routes {
		if route.resultCh != nil {
			select {
			case route.resultCh <- rpcResultMessage("", false, nil, ErrorDesktopOffline):
			default:
			}
			continue
		}
		if route.mobile != nil {
			_ = route.mobile.send(context.Background(), rpcResultMessage(route.mobileRequest, false, nil, ErrorDesktopOffline))
		}
	}
	s.cfg.Logger.Printf("desktop disconnected device=%s", desktop.deviceID)
}

func (s *Server) registerMobile(mobile *peer) {
	s.mu.Lock()
	s.mobiles[mobile] = struct{}{}
	s.mu.Unlock()
}

func (s *Server) unregisterMobile(mobile *peer) {
	s.mu.Lock()
	delete(s.mobiles, mobile)
	for requestID, route := range s.pending {
		if route.mobile == mobile {
			delete(s.pending, requestID)
		}
	}
	s.mu.Unlock()
	s.cfg.Logger.Printf("mobile disconnected device=%s", mobile.deviceID)
}

func (s *Server) desktopFor(deviceID string) *peer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.desktops[deviceID]
}

func (s *Server) activeMobileSessions(deviceID string) map[string]bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	active := make(map[string]bool)
	for mobile := range s.mobiles {
		if mobile.deviceID == deviceID && mobile.sessionID != "" {
			active[mobile.sessionID] = true
		}
	}
	return active
}

func (s *Server) notifyDesktopSessionCount(deviceID string) {
	desktop := s.desktopFor(deviceID)
	if desktop == nil {
		return
	}
	_ = desktop.send(context.Background(), Message{
		Type:           MessageTypeSessions,
		PairedSessions: s.store.CountDevice(deviceID),
	})
}

func (s *Server) disconnectMobileSessions(deviceID string, records []sessionRecord) {
	if len(records) == 0 {
		return
	}
	sessionIDs := make(map[string]struct{}, len(records))
	tokenHashes := make(map[string]struct{}, len(records))
	for _, record := range records {
		sessionIDs[record.SessionID] = struct{}{}
		tokenHashes[record.TokenHash] = struct{}{}
	}
	s.mu.Lock()
	victims := make([]*peer, 0)
	for mobile := range s.mobiles {
		if mobile.deviceID != deviceID {
			continue
		}
		_, byID := sessionIDs[mobile.sessionID]
		_, byHash := tokenHashes[mobile.tokenHash]
		if byID || byHash {
			victims = append(victims, mobile)
		}
	}
	s.mu.Unlock()
	for _, mobile := range victims {
		_ = mobile.conn.Close(websocket.StatusPolicyViolation, "session revoked")
	}
	if len(victims) > 0 {
		s.cfg.Logger.Printf("mobile connections closed after revoke device=%s count=%d", deviceID, len(victims))
	}
}

func (s *Server) createPairCode(deviceID string) (string, string, error) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for code, pair := range s.pairs {
		if !pair.expiresAt.After(now) {
			delete(s.pairs, code)
		}
	}
	for code, pair := range s.pairs {
		if pair.deviceID == deviceID {
			delete(s.pairs, code)
		}
	}
	for attempts := 0; attempts < 8; attempts++ {
		code, err := randomPairCode()
		if err != nil {
			return "", "", err
		}
		if _, exists := s.pairs[code]; exists {
			continue
		}
		expires := now.Add(s.cfg.PairTTL)
		s.pairs[code] = pairRecord{deviceID: deviceID, expiresAt: expires}
		return code, expires.UTC().Format(time.RFC3339), nil
	}
	return "", "", errors.New("could not allocate pairing code")
}

func (s *Server) routeRPCResult(desktop *peer, message Message) {
	s.mu.Lock()
	route, ok := s.pending[message.RequestID]
	if ok && route.deviceID == desktop.deviceID {
		delete(s.pending, message.RequestID)
	}
	s.mu.Unlock()
	if !ok || route.deviceID != desktop.deviceID {
		return
	}
	out := rpcResultMessage(route.mobileRequest, message.OK, nil, message.Error)
	out.Result = message.Result
	s.cfg.Logger.Printf("rpc result device=%s method=%s ok=%v error=%s", route.deviceID, route.method, message.OK, message.Error)
	if route.resultCh != nil {
		select {
		case route.resultCh <- out:
		default:
		}
		return
	}
	if route.mobile == nil {
		return
	}
	_ = route.mobile.send(context.Background(), out)
}

func (s *Server) removePending(requestID string) {
	s.mu.Lock()
	delete(s.pending, requestID)
	s.mu.Unlock()
}

func (s *Server) cleanupPendingLocked(now time.Time) {
	for requestID, route := range s.pending {
		if !route.expiresAt.After(now) {
			delete(s.pending, requestID)
		}
	}
}

func (s *Server) allowPairAttempt(ip string) bool {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.attempts) > 4096 {
		for key, window := range s.attempts {
			if !window.resetAt.After(now) {
				delete(s.attempts, key)
			}
		}
	}
	window := s.attempts[ip]
	if window.resetAt.IsZero() || !window.resetAt.After(now) {
		window = attemptWindow{resetAt: now.Add(pairAttemptWindow)}
	}
	if window.count >= pairAttemptLimit {
		s.attempts[ip] = window
		return false
	}
	window.count++
	s.attempts[ip] = window
	return true
}

func (s *Server) clearPairAttempts(ip string) {
	s.mu.Lock()
	delete(s.attempts, ip)
	s.mu.Unlock()
}

func (s *Server) countPendingForMobileLocked(mobile *peer) int {
	count := 0
	for _, route := range s.pending {
		if route.mobile == mobile {
			count++
		}
	}
	return count
}

func (s *Server) countPendingForDeviceLocked(deviceID string) int {
	count := 0
	for _, route := range s.pending {
		if route.deviceID == deviceID {
			count++
		}
	}
	return count
}

func (p *peer) send(ctx context.Context, message Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	p.writeMu.Lock()
	defer p.writeMu.Unlock()
	writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return p.conn.Write(writeCtx, websocket.MessageText, raw)
}

func readMessage(ctx context.Context, conn *websocket.Conn) (Message, error) {
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return Message{}, err
	}
	var message Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return Message{}, err
	}
	return message, nil
}

func bearerToken(r *http.Request) string {
	value := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		return ""
	}
	return strings.TrimSpace(value[7:])
}

func secureEqual(got, want string) bool {
	if got == "" || want == "" {
		return false
	}
	gotHash := sha256.Sum256([]byte(got))
	wantHash := sha256.Sum256([]byte(want))
	return subtle.ConstantTimeCompare(gotHash[:], wantHash[:]) == 1
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func randomToken(bytes int) string {
	return hex.EncodeToString(randomBytes(bytes))
}

func randomID(bytes int) string {
	return hex.EncodeToString(randomBytes(bytes))
}

func randomBytes(length int) []byte {
	result := make([]byte, length)
	if _, err := rand.Read(result); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return result
}

var pairEncoding = base32.NewEncoding("0123456789ABCDEFGHJKMNPQRSTVWXYZ").WithPadding(base32.NoPadding)

func randomPairCode() (string, error) {
	raw := make([]byte, 5)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return pairEncoding.EncodeToString(raw), nil
}

func normalizePairCode(code string) string {
	code = strings.ToUpper(strings.TrimSpace(code))
	replacer := strings.NewReplacer("-", "", " ", "", "I", "1", "L", "1", "O", "0", "U", "V")
	code = replacer.Replace(code)
	if len(code) != 8 {
		return ""
	}
	for _, char := range code {
		if !strings.ContainsRune("0123456789ABCDEFGHJKMNPQRSTVWXYZ", char) {
			return ""
		}
	}
	return code
}

func validDeviceID(deviceID string) bool {
	if len(deviceID) < 8 || len(deviceID) > 64 {
		return false
	}
	for _, char := range deviceID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func validSessionID(sessionID string) bool {
	if len(sessionID) < 8 || len(sessionID) > 128 {
		return false
	}
	for _, char := range sessionID {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '-' || char == '_' {
			continue
		}
		return false
	}
	return true
}

func limitText(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return value
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeJSONCompressed(w http.ResponseWriter, r *http.Request, status int, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not encode response"})
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Add("Vary", "Accept-Encoding")

	if len(data) >= 4096 && strings.Contains(strings.ToLower(r.Header.Get("Accept-Encoding")), "gzip") {
		var compressed bytes.Buffer
		writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
		if err == nil {
			if _, err = writer.Write(data); err == nil {
				err = writer.Close()
			}
			if err == nil {
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Length", strconv.Itoa(compressed.Len()))
				w.WriteHeader(status)
				_, _ = w.Write(compressed.Bytes())
				return
			}
		}
	}

	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func requestIP(r *http.Request, trustProxy bool) string {
	if forwarded := strings.TrimSpace(r.Header.Get("X-Forwarded-For")); trustProxy && forwarded != "" {
		first, _, _ := strings.Cut(forwarded, ",")
		if value := strings.TrimSpace(first); value != "" {
			return value
		}
	}
	if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return host
	}
	return r.RemoteAddr
}
