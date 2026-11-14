package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shuaigehuang/codexlink/internal/codex"
	"github.com/shuaigehuang/codexlink/internal/magpie"
	relayproto "github.com/shuaigehuang/codexlink/internal/relay"

	"github.com/coder/websocket"
	"github.com/skip2/go-qrcode"
)

const (
	relayDialTimeout     = 15 * time.Second
	relayPairWaitTimeout = 20 * time.Second
	relayHeartbeatPeriod = 25 * time.Second
	relayMaxRPCTime      = 3 * time.Minute
)

type RelayPairing struct {
	Code      string `json:"code"`
	ExpiresAt string `json:"expiresAt"`
	Link      string `json:"link"`
	QRDataURL string `json:"qrDataUrl"`
}

type RelaySession struct {
	ID         string `json:"id"`
	DeviceName string `json:"deviceName,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ExpiresAt  string `json:"expiresAt"`
	Active     bool   `json:"active"`
}

type RelaySessionList struct {
	Sessions []RelaySession `json:"sessions"`
	Count    int            `json:"count"`
}

type relayNodeStatusView struct {
	Online     bool         `json:"online"`
	DeviceID   string       `json:"deviceId"`
	DeviceName string       `json:"deviceName"`
	RelayURL   string       `json:"relayUrl"`
	Codex      CodexStatus  `json:"codex"`
	Magpie     MagpieStatus `json:"magpie"`
	Time       string       `json:"time"`
}

type relaySession struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

func (s *CodexLinkService) initializeRelay() {
	store, err := defaultRelayConfigStore()
	if err != nil {
		s.setRelayUnconfigured(err.Error())
		return
	}
	cfg, err := store.Load()
	if errors.Is(err, errRelayNotConfigured) {
		s.setRelayUnconfigured("填写中继地址和电脑端密钥后即可连接")
		return
	}
	if err != nil {
		s.setRelayUnconfigured(err.Error())
		return
	}
	s.startRelay(cfg)
}

func (s *CodexLinkService) startRelay(cfg relayConfig) {
	s.stopRelay()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	s.relayMu.Lock()
	s.relayCfg = cfg
	s.relayConfigured = true
	s.relayGeneration++
	generation := s.relayGeneration
	s.relayCancel = cancel
	s.relayDone = done
	s.relayStatus = RelayStatus{
		Connected:  false,
		State:      "正在连接",
		Detail:     "连接 " + cfg.URL,
		Configured: true,
		URL:        cfg.URL,
		DeviceID:   cfg.DeviceID,
		DeviceName: cfg.DeviceName,
		TokenSet:   true,
		Insecure:   relayURLInsecure(cfg.URL),
	}
	s.relayMu.Unlock()

	go func() {
		defer close(done)
		s.relayLoop(ctx, cfg, generation)
	}()
}

func (s *CodexLinkService) stopRelay() {
	s.relayMu.Lock()
	s.relayGeneration++
	cancel := s.relayCancel
	done := s.relayDone
	session := s.relayConn
	s.relayCancel = nil
	s.relayDone = nil
	s.relayConn = nil
	s.failPairLocked(errors.New("中继连接已断开"))
	s.relayMu.Unlock()

	if cancel != nil {
		cancel()
	}
	if session != nil {
		_ = session.conn.Close(websocket.StatusGoingAway, "desktop disconnecting")
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
	}
}

func (s *CodexLinkService) relayLoop(ctx context.Context, cfg relayConfig, generation uint64) {
	backoff := 2 * time.Second
	for {
		err := s.runRelaySession(ctx, cfg, generation)
		if ctx.Err() != nil {
			return
		}
		detail := "连接中断，正在自动重连"
		if err != nil {
			detail = "连接中断，正在自动重连: " + err.Error()
		}
		s.setRelayStatusIfCurrent(generation, false, "重连中", detail)

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		backoff *= 2
		if backoff > 30*time.Second {
			backoff = 30 * time.Second
		}
	}
}

func (s *CodexLinkService) runRelaySession(ctx context.Context, cfg relayConfig, generation uint64) error {
	endpoint, err := relayWebSocketURL(cfg.URL, cfg.DeviceID, cfg.DeviceName)
	if err != nil {
		return err
	}
	dialCtx, cancel := context.WithTimeout(ctx, relayDialTimeout)
	conn, _, err := websocket.Dial(dialCtx, endpoint, &websocket.DialOptions{
		HTTPHeader: map[string][]string{
			"Authorization": {"Bearer " + cfg.Token},
		},
	})
	cancel()
	if err != nil {
		return fmt.Errorf("无法连接云端中继: %w", err)
	}
	session := &relaySession{conn: conn}
	conn.SetReadLimit(8 << 20)

	s.relayMu.Lock()
	if s.relayCancel == nil || s.relayGeneration != generation {
		s.relayMu.Unlock()
		_ = conn.Close(websocket.StatusGoingAway, "relay stopped")
		return errors.New("中继连接已关闭")
	}
	s.relayConn = session
	s.relayMu.Unlock()

	defer func() {
		s.relayMu.Lock()
		if s.relayConn == session {
			s.relayConn = nil
		}
		if s.relayGeneration == generation {
			s.failPairLocked(errors.New("中继连接已断开"))
		}
		s.relayMu.Unlock()
		_ = conn.Close(websocket.StatusNormalClosure, "")
	}()

	stopHeartbeat := make(chan struct{})
	defer close(stopHeartbeat)
	go s.relayHeartbeat(ctx, conn, stopHeartbeat)

	for {
		message, err := readRelayMessage(ctx, conn)
		if err != nil {
			return err
		}
		switch message.Type {
		case relayproto.MessageTypeHello:
			s.relayMu.Lock()
			if s.relayGeneration != generation {
				s.relayMu.Unlock()
				return errors.New("中继连接已被替换")
			}
			s.relayStatus = RelayStatus{
				Connected:       true,
				State:           "已连接",
				Detail:          relayConnectedDetail(cfg.URL, message.PairedSessions),
				Configured:      true,
				URL:             cfg.URL,
				DeviceID:        cfg.DeviceID,
				DeviceName:      cfg.DeviceName,
				TokenSet:        true,
				Insecure:        relayURLInsecure(cfg.URL),
				PairedSessions:  message.PairedSessions,
				LastConnectedAt: time.Now().Format("2006-01-02 15:04:05"),
			}
			s.relayMu.Unlock()
		case relayproto.MessageTypeSessions:
			s.relayMu.Lock()
			if s.relayGeneration == generation {
				s.relayStatus.PairedSessions = message.PairedSessions
				if s.relayStatus.Connected {
					s.relayStatus.Detail = relayConnectedDetail(cfg.URL, message.PairedSessions)
				}
			}
			s.relayMu.Unlock()
		case relayproto.MessageTypePaired, relayproto.MessageTypeError:
			s.deliverPair(message)
		case relayproto.MessageTypeRPC:
			go s.handleRelayRPC(ctx, session, message)
		}
	}
}

func relayConnectedDetail(relayURL string, pairedSessions int) string {
	detail := fmt.Sprintf("云端中继在线 · %d 台手机已配对", pairedSessions)
	if relayURLInsecure(relayURL) {
		detail += " · 当前连接未加密"
	}
	return detail
}

func (s *CodexLinkService) relayHeartbeat(ctx context.Context, conn *websocket.Conn, stop <-chan struct{}) {
	ticker := time.NewTicker(relayHeartbeatPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
			pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := conn.Ping(pingCtx)
			cancel()
			if err != nil {
				_ = conn.Close(websocket.StatusGoingAway, "heartbeat failed")
				return
			}
		}
	}
}

func readRelayMessage(ctx context.Context, conn *websocket.Conn) (relayproto.Message, error) {
	_, raw, err := conn.Read(ctx)
	if err != nil {
		return relayproto.Message{}, err
	}
	var message relayproto.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		return relayproto.Message{}, err
	}
	return message, nil
}

func (s *relaySession) send(ctx context.Context, message relayproto.Message) error {
	raw, err := json.Marshal(message)
	if err != nil {
		return err
	}
	writeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.Write(writeCtx, websocket.MessageText, raw)
}

func (s *CodexLinkService) handleRelayRPC(ctx context.Context, session *relaySession, message relayproto.Message) {
	sem := s.relaySemaphore()
	select {
	case sem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-sem }()

	rpcCtx, cancel := context.WithTimeout(ctx, relayMaxRPCTime)
	defer cancel()
	result, err := s.dispatchRelayRPC(rpcCtx, message.Method, message.Params)
	out := relayproto.Message{
		Type:      relayproto.MessageTypeRPCResult,
		RequestID: message.RequestID,
	}
	if err != nil {
		out.OK = false
		out.Error = relayErrorText(err)
	} else {
		out.OK = true
		encoded, marshalErr := json.Marshal(result)
		if marshalErr != nil {
			out.OK = false
			out.Error = "结果编码失败"
		} else {
			out.Result = encoded
		}
	}
	_ = session.send(rpcCtx, out)
}

func (s *CodexLinkService) relaySemaphore() chan struct{} {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayRPCSem == nil {
		s.relayRPCSem = make(chan struct{}, 2)
	}
	return s.relayRPCSem
}

func (s *CodexLinkService) dispatchRelayRPC(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "ping":
		return map[string]any{
			"pong": true,
			"time": time.Now().Format(time.RFC3339),
		}, nil
	case "node.status":
		return s.relayNodeStatus(ctx), nil
	case "threads.list":
		var input struct {
			Limit int `json:"limit"`
		}
		if len(params) > 0 {
			if err := json.Unmarshal(params, &input); err != nil {
				return nil, errors.New("会话列表参数无效")
			}
		}
		if input.Limit <= 0 || input.Limit > 100 {
			input.Limit = 50
		}
		return s.relayThreadList(ctx, input.Limit)
	case "threads.read":
		var input struct {
			ThreadID         string `json:"threadId"`
			SinceTimestamp   int64  `json:"sinceTimestamp"`
			IncludeReasoning bool   `json:"includeReasoning"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("会话参数无效")
		}
		input.ThreadID = strings.TrimSpace(input.ThreadID)
		if input.ThreadID == "" {
			return nil, errors.New("缺少会话 ID")
		}
		if input.IncludeReasoning {
			return s.ReadThreadDetailed(ctx, input.ThreadID, input.SinceTimestamp)
		}
		return s.ReadThreadSince(ctx, input.ThreadID, input.SinceTimestamp)
	case "threads.send":
		var input struct {
			ThreadID string `json:"threadId"`
			Text     string `json:"text"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("发送参数无效")
		}
		return s.SendMessage(ctx, strings.TrimSpace(input.ThreadID), input.Text)
	case "magpie.status":
		s.opMu.Lock()
		defer s.opMu.Unlock()
		return s.readMagpie(ctx), nil
	case "magpie.admin.status":
		s.opMu.Lock()
		defer s.opMu.Unlock()
		return s.readMagpie(ctx).Admin, nil
	case "magpie.admin.initialize":
		return s.InitializeMagpieAdmin(ctx)
	case "magpie.keys.create":
		var input struct {
			Name     string `json:"name"`
			ReadOnly bool   `json:"readOnly"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("创建密钥参数无效")
		}
		return s.CreateMagpieKey(ctx, input.Name, input.ReadOnly)
	case "magpie.keys.rotate":
		var input struct {
			KeyID string `json:"keyId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("轮换密钥参数无效")
		}
		return s.RotateMagpieKey(ctx, input.KeyID)
	case "magpie.keys.toggle":
		var input struct {
			KeyID   string `json:"keyId"`
			Enabled bool   `json:"enabled"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("密钥启停参数无效")
		}
		return s.SetMagpieKeyEnabled(ctx, input.KeyID, input.Enabled)
	case "magpie.keys.delete":
		var input struct {
			KeyID string `json:"keyId"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, errors.New("删除密钥参数无效")
		}
		return s.RemoveMagpieKey(ctx, input.KeyID)
	default:
		return nil, errors.New("请求的方法不在允许列表内")
	}
}

func (s *CodexLinkService) relayNodeStatus(ctx context.Context) relayNodeStatusView {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	s.relayMu.Lock()
	cfg := s.relayCfg
	s.relayMu.Unlock()
	result := relayNodeStatusView{
		Online:     true,
		DeviceID:   cfg.DeviceID,
		DeviceName: cfg.DeviceName,
		RelayURL:   cfg.URL,
		Time:       time.Now().Format(time.RFC3339),
		Codex:      CodexStatus{},
		Magpie:     s.readMagpie(ctx),
	}
	if err := s.ensureCodex(ctx); err != nil {
		result.Codex.Error = err.Error()
		return result
	}
	result.Codex.Connected = true
	result.Codex.UserAgent = s.server.UserAgent
	result.Codex.CodexHome = s.server.CodexHome
	result.Codex.Executable = s.client.Executable()
	threads, err := s.client.ListThreads(ctx, codex.ThreadListParams{
		Limit:         20,
		SortKey:       "updated_at",
		SortDirection: "desc",
	})
	if err != nil {
		result.Codex.Error = err.Error()
		return result
	}
	result.Codex.Threads = len(threads.Data)
	return result
}

func (s *CodexLinkService) relayThreadList(ctx context.Context, limit int) ([]ThreadBrief, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	if err := s.ensureCodex(ctx); err != nil {
		return nil, err
	}
	threads, err := s.client.ListThreads(ctx, codex.ThreadListParams{
		Limit:         limit,
		SortKey:       "updated_at",
		SortDirection: "desc",
	})
	if err != nil {
		return nil, err
	}
	result := make([]ThreadBrief, 0, len(threads.Data))
	for _, thread := range threads.Data {
		result = append(result, toThreadBrief(thread))
	}
	return result, nil
}

func (s *CodexLinkService) setRelayStatusIfCurrent(generation uint64, connected bool, state, detail string) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayGeneration != generation {
		return
	}
	s.relayStatus.Connected = connected
	s.relayStatus.State = state
	s.relayStatus.Detail = detail
}

func (s *CodexLinkService) setRelayUnconfigured(detail string) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	s.relayConfigured = false
	s.relayCfg = relayConfig{}
	s.relayStatus = RelayStatus{
		Connected:  false,
		State:      "未配置",
		Detail:     detail,
		Configured: false,
	}
}

func (s *CodexLinkService) relayStatusSnapshot() RelayStatus {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	status := s.relayStatus
	status.Configured = s.relayConfigured
	if s.relayConfigured {
		status.URL = s.relayCfg.URL
		status.DeviceID = s.relayCfg.DeviceID
		status.DeviceName = s.relayCfg.DeviceName
		status.TokenSet = strings.TrimSpace(s.relayCfg.Token) != ""
		status.Insecure = relayURLInsecure(s.relayCfg.URL)
	} else {
		status.TokenSet = false
		status.Insecure = false
	}
	return status
}

func relayErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if len(text) > 800 {
		text = text[:800]
	}
	return magpie.Redact(text)
}

func (s *CodexLinkService) deliverPair(message relayproto.Message) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if s.relayPairRequest == "" || message.RequestID != s.relayPairRequest {
		return
	}
	ch := s.relayPairWaiter
	s.relayPairWaiter = nil
	s.relayPairRequest = ""
	if ch != nil {
		select {
		case ch <- message:
		default:
		}
	}
}

func (s *CodexLinkService) failPairLocked(err error) {
	if s.relayPairWaiter == nil {
		return
	}
	ch := s.relayPairWaiter
	s.relayPairWaiter = nil
	s.relayPairRequest = ""
	message := relayproto.Message{
		Type:  relayproto.MessageTypeError,
		Error: err.Error(),
	}
	select {
	case ch <- message:
	default:
	}
}

func (s *CodexLinkService) GetRelayConfig(ctx context.Context) RelayStatus {
	return s.relayStatusSnapshot()
}

func (s *CodexLinkService) SaveRelayConfig(ctx context.Context, relayURL, token, deviceName string) (RelayStatus, error) {
	normalized, err := normalizeRelayURL(relayURL)
	if err != nil {
		return s.relayStatusSnapshot(), err
	}
	store, err := defaultRelayConfigStore()
	if err != nil {
		return s.relayStatusSnapshot(), err
	}
	existing, loadErr := store.Load()
	if loadErr != nil && !errors.Is(loadErr, errRelayNotConfigured) {
		return s.relayStatusSnapshot(), loadErr
	}
	token = strings.TrimSpace(token)
	if token == "" {
		token = existing.Token
	}
	if len(token) < relayMinTokenBytes {
		return s.relayStatusSnapshot(), fmt.Errorf("中继密钥至少需要 %d 个字符", relayMinTokenBytes)
	}
	deviceID := existing.DeviceID
	if !validRelayDeviceID(deviceID) {
		deviceID = newRelayDeviceID()
	}
	if strings.TrimSpace(deviceName) == "" {
		deviceName = existing.DeviceName
	}
	if strings.TrimSpace(deviceName) == "" {
		deviceName = defaultRelayDeviceName()
	}
	cfg := relayConfig{
		URL:        normalized,
		Token:      token,
		DeviceID:   deviceID,
		DeviceName: strings.TrimSpace(deviceName),
	}
	if err := store.Save(cfg); err != nil {
		return s.relayStatusSnapshot(), err
	}
	s.startRelay(cfg)
	return s.relayStatusSnapshot(), nil
}

func (s *CodexLinkService) ReconnectRelay(ctx context.Context) (RelayStatus, error) {
	store, err := defaultRelayConfigStore()
	if err != nil {
		return s.relayStatusSnapshot(), err
	}
	cfg, err := store.Load()
	if err != nil {
		return s.relayStatusSnapshot(), err
	}
	s.startRelay(cfg)
	return s.relayStatusSnapshot(), nil
}

func (s *CodexLinkService) ForgetRelay(ctx context.Context) (RelayStatus, error) {
	s.stopRelay()
	store, err := defaultRelayConfigStore()
	if err != nil {
		return s.relayStatusSnapshot(), err
	}
	if err := store.Delete(); err != nil {
		return s.relayStatusSnapshot(), err
	}
	s.setRelayUnconfigured("中继配置已清除")
	return s.relayStatusSnapshot(), nil
}

func (s *CodexLinkService) ListRelaySessions(ctx context.Context) (RelaySessionList, error) {
	cfg, err := s.relayConfigForManagement()
	if err != nil {
		return RelaySessionList{Sessions: []RelaySession{}}, err
	}
	return s.fetchRelaySessions(ctx, cfg)
}

func (s *CodexLinkService) RevokeRelaySessions(ctx context.Context, sessionID string, all bool) (RelaySessionList, error) {
	cfg, err := s.relayConfigForManagement()
	if err != nil {
		return RelaySessionList{Sessions: []RelaySession{}}, err
	}
	sessionID = strings.TrimSpace(sessionID)
	if !all && sessionID == "" {
		return RelaySessionList{Sessions: []RelaySession{}}, errors.New("缺少要撤销的手机授权")
	}
	request := relayproto.RevokeSessionsRequest{
		DeviceID:  cfg.DeviceID,
		SessionID: sessionID,
		All:       all,
	}
	if _, err := s.relayManagementRequest(ctx, cfg, http.MethodPost, relayproto.PathDesktopRevoke, nil, request); err != nil {
		return RelaySessionList{Sessions: []RelaySession{}}, err
	}
	return s.fetchRelaySessions(ctx, cfg)
}

func (s *CodexLinkService) relayConfigForManagement() (relayConfig, error) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if !s.relayConfigured || !validRelayDeviceID(s.relayCfg.DeviceID) || strings.TrimSpace(s.relayCfg.Token) == "" {
		return relayConfig{}, errors.New("请先保存并连接云端中继")
	}
	return s.relayCfg, nil
}

func (s *CodexLinkService) fetchRelaySessions(ctx context.Context, cfg relayConfig) (RelaySessionList, error) {
	payload, err := s.relayManagementRequest(ctx, cfg, http.MethodGet, relayproto.PathDesktopSessions,
		url.Values{"device": {cfg.DeviceID}}, nil)
	if err != nil {
		return RelaySessionList{Sessions: []RelaySession{}}, err
	}
	var list RelaySessionList
	if err := json.Unmarshal(payload, &list); err != nil {
		return RelaySessionList{Sessions: []RelaySession{}}, errors.New("云端中继返回了无法识别的授权列表")
	}
	if list.Sessions == nil {
		list.Sessions = []RelaySession{}
	}
	list.Count = len(list.Sessions)
	s.applyRelaySessionCount(list.Count)
	return list, nil
}

func (s *CodexLinkService) relayManagementRequest(ctx context.Context, cfg relayConfig, method, apiPath string, query url.Values, body any) ([]byte, error) {
	endpoint, err := relayHTTPEndpoint(cfg.URL, apiPath, query)
	if err != nil {
		return nil, err
	}
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return nil, errors.New("无法编码中继管理请求")
		}
		reader = bytes.NewReader(raw)
	}
	requestCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, method, endpoint, reader)
	if err != nil {
		return nil, errors.New("中继管理请求地址无效")
	}
	request.Header.Set("Authorization", "Bearer "+cfg.Token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("无法连接云端中继: %w", err)
	}
	defer response.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, errors.New("读取云端中继响应失败")
	}
	if response.StatusCode != http.StatusOK {
		switch response.StatusCode {
		case http.StatusUnauthorized:
			return nil, errors.New("电脑端密钥无效，请在“云端中继”中重新保存密钥")
		case http.StatusNotFound:
			return nil, errors.New("云端中继版本较旧，请先升级服务器端")
		case http.StatusBadRequest:
			return nil, errors.New("云端中继拒绝了请求参数")
		case http.StatusTooManyRequests:
			return nil, errors.New("操作过于频繁，请稍后再试")
		default:
			if response.StatusCode >= 500 {
				return nil, errors.New("云端中继暂时不可用，请稍后再试")
			}
			return nil, fmt.Errorf("云端中继请求失败（HTTP %d）", response.StatusCode)
		}
	}
	return payload, nil
}

func relayHTTPEndpoint(base, apiPath string, query url.Values) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil || parsed.Host == "" {
		return "", errors.New("中继地址格式无效")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("中继地址协议无效")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + apiPath
	parsed.RawQuery = query.Encode()
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (s *CodexLinkService) applyRelaySessionCount(count int) {
	s.relayMu.Lock()
	defer s.relayMu.Unlock()
	if !s.relayConfigured {
		return
	}
	s.relayStatus.PairedSessions = count
	if s.relayStatus.Connected {
		s.relayStatus.Detail = relayConnectedDetail(s.relayCfg.URL, count)
	}
}

func (s *CodexLinkService) CreateRelayPairing(ctx context.Context) (RelayPairing, error) {
	s.relayMu.Lock()
	if !s.relayConfigured || s.relayConn == nil || !s.relayStatus.Connected {
		s.relayMu.Unlock()
		return RelayPairing{}, errors.New("中继尚未连接，请先保存配置并等待连接成功")
	}
	if s.relayPairWaiter != nil {
		s.relayMu.Unlock()
		return RelayPairing{}, errors.New("正在生成配对码，请稍候")
	}
	ch := make(chan relayproto.Message, 1)
	requestID := "pair-" + newRelayDeviceID()
	session := s.relayConn
	cfg := s.relayCfg
	s.relayPairWaiter = ch
	s.relayPairRequest = requestID
	s.relayMu.Unlock()

	sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	err := session.send(sendCtx, relayproto.Message{
		Type:      relayproto.MessageTypePair,
		RequestID: requestID,
	})
	cancel()
	if err != nil {
		s.relayMu.Lock()
		if s.relayPairRequest == requestID {
			s.relayPairWaiter = nil
			s.relayPairRequest = ""
		}
		s.relayMu.Unlock()
		return RelayPairing{}, errors.New("发送配对请求失败")
	}

	timer := time.NewTimer(relayPairWaitTimeout)
	defer timer.Stop()
	select {
	case message := <-ch:
		if message.Type == relayproto.MessageTypeError || message.Code == "" {
			if message.Error == "" {
				message.Error = "中继未能生成配对码"
			}
			return RelayPairing{}, errors.New(message.Error)
		}
		link := "codexlink://pair?relay=" + url.QueryEscape(cfg.URL) +
			"&code=" + url.QueryEscape(message.Code) +
			"&device=" + url.QueryEscape(cfg.DeviceID) +
			"&name=" + url.QueryEscape(cfg.DeviceName)
		if relayURLInsecure(cfg.URL) {
			link += "&insecure=1"
		}
		png, err := qrcode.Encode(link, qrcode.Medium, 512)
		if err != nil {
			return RelayPairing{}, fmt.Errorf("生成二维码失败: %w", err)
		}
		return RelayPairing{
			Code:      message.Code,
			ExpiresAt: message.ExpiresAt,
			Link:      link,
			QRDataURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
		}, nil
	case <-ctx.Done():
		s.clearPairRequest(requestID)
		return RelayPairing{}, ctx.Err()
	case <-timer.C:
		s.clearPairRequest(requestID)
		return RelayPairing{}, errors.New("生成配对码超时，请检查中继连接")
	}
}

func (s *CodexLinkService) clearPairRequest(requestID string) {
	s.relayMu.Lock()
	if s.relayPairRequest == requestID {
		s.relayPairWaiter = nil
		s.relayPairRequest = ""
	}
	s.relayMu.Unlock()
}
