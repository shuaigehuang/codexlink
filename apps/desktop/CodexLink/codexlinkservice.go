package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/shuaigehuang/codexlink/internal/codex"
	"github.com/shuaigehuang/codexlink/internal/magpie"
	relayproto "github.com/shuaigehuang/codexlink/internal/relay"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type CodexLinkService struct {
	mu        sync.Mutex
	opMu      sync.Mutex
	client    *codex.Client
	server    codex.InitializeResponse
	lastError string

	closeMu             sync.RWMutex
	closeStore          *closeBehaviorStore
	closeSettings       CloseBehaviorSettings
	closeSettingsLoaded bool

	relayMu          sync.Mutex
	relayCfg         relayConfig
	relayConfigured  bool
	relayCancel      context.CancelFunc
	relayDone        chan struct{}
	relayConn        *relaySession
	relayStatus      RelayStatus
	relayPairWaiter  chan relayproto.Message
	relayPairRequest string
	relayRPCSem      chan struct{}
	relayGeneration  uint64
}

type Dashboard struct {
	Codex       CodexStatus   `json:"codex"`
	Magpie      MagpieStatus  `json:"magpie"`
	Relay       RelayStatus   `json:"relay"`
	Threads     []ThreadBrief `json:"threads"`
	RefreshedAt string        `json:"refreshedAt"`
}

type CodexStatus struct {
	Connected  bool   `json:"connected"`
	Error      string `json:"error"`
	UserAgent  string `json:"userAgent"`
	CodexHome  string `json:"codexHome"`
	Executable string `json:"executable"`
	Threads    int    `json:"threads"`
}

type MagpieStatus struct {
	Connected       bool              `json:"connected"`
	Error           string            `json:"error"`
	URL             string            `json:"url"`
	Version         string            `json:"version"`
	Role            string            `json:"role"`
	Providers       []string          `json:"providers"`
	Quota           []MagpieQuota     `json:"quota"`
	GatewayKeyCount int               `json:"gatewayKeyCount"`
	Admin           MagpieAdminStatus `json:"admin"`
}

type MagpieQuota struct {
	Provider string `json:"provider"`
	Name     string `json:"name"`
	Kind     string `json:"kind"`
	Balance  string `json:"balance"`
	ReadAt   string `json:"readAt"`
}

type MagpieAdminStatus struct {
	Ready       bool                    `json:"ready"`
	KeyID       string                  `json:"keyId"`
	KeyName     string                  `json:"keyName"`
	Error       string                  `json:"error"`
	RefreshedAt string                  `json:"refreshedAt"`
	Providers   []MagpieAdminProvider   `json:"providers"`
	GatewayKeys []MagpieAdminGatewayKey `json:"gatewayKeys"`
}

type MagpieAdminProvider struct {
	ID        string                  `json:"id"`
	Name      string                  `json:"name"`
	Preset    string                  `json:"preset"`
	Host      string                  `json:"host"`
	KeySet    bool                    `json:"keySet"`
	KeyMasked string                  `json:"keyMasked"`
	Ready     bool                    `json:"ready"`
	Off       bool                    `json:"off"`
	Models    int                     `json:"models"`
	KeyRefs   []MagpieProviderKeyView `json:"keyRefs"`
}

type MagpieProviderKeyView struct {
	ID     string `json:"id"`
	Masked string `json:"masked"`
	Active bool   `json:"active"`
	On     bool   `json:"on"`
}

type MagpieAdminGatewayKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Masked    string `json:"masked"`
	Admin     string `json:"admin"`
	Off       bool   `json:"off"`
	ReadOnly  bool   `json:"readOnly"`
	Protected bool   `json:"protected"`
}

type MagpieKeySecret struct {
	KeyID   string            `json:"keyId"`
	KeyName string            `json:"keyName"`
	Secret  string            `json:"secret"`
	Status  MagpieAdminStatus `json:"status"`
}

type ProviderTestEntry struct {
	Protocol string `json:"protocol"`
	Model    string `json:"model"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status"`
	Millis   int64  `json:"millis"`
	Error    string `json:"error"`
	Queued   string `json:"queued"`
}

type ProviderTestSummary struct {
	ProviderID string              `json:"providerId"`
	OK         bool                `json:"ok"`
	Summary    string              `json:"summary"`
	Results    []ProviderTestEntry `json:"results"`
	TestedAt   string              `json:"testedAt"`
}

type SendTestResult struct {
	OK         bool     `json:"ok"`
	ThreadID   string   `json:"threadId"`
	TurnID     string   `json:"turnId"`
	Status     string   `json:"status"`
	ItemTypes  []string `json:"itemTypes"`
	AgentReply string   `json:"agentReply"`
	ElapsedMS  int64    `json:"elapsedMs"`
}

type RelayStatus struct {
	Connected       bool   `json:"connected"`
	Configured      bool   `json:"configured"`
	State           string `json:"state"`
	Detail          string `json:"detail"`
	URL             string `json:"url"`
	DeviceID        string `json:"deviceId"`
	DeviceName      string `json:"deviceName"`
	TokenSet        bool   `json:"tokenSet"`
	Insecure        bool   `json:"insecure"`
	PairedSessions  int    `json:"pairedSessions"`
	LastConnectedAt string `json:"lastConnectedAt"`
}

type ThreadBrief struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Preview      string `json:"preview"`
	Cwd          string `json:"cwd"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	UpdatedAt    int64  `json:"updatedAt"`
	UpdatedLabel string `json:"updatedLabel"`
}

type ThreadDetail struct {
	Thread          ThreadBrief      `json:"thread"`
	Items           []ThreadItemView `json:"items"`
	Turns           int              `json:"turns"`
	LatestTimestamp int64            `json:"latestTimestamp,omitempty"`
}

type ThreadItemView struct {
	ID        string `json:"id"`
	Type      string `json:"type"`
	Text      string `json:"text"`
	Command   string `json:"command"`
	Output    string `json:"output"`
	Timestamp int64  `json:"timestamp"`
	Completed bool   `json:"completed"`
}

type SendResult struct {
	ThreadID string `json:"threadId"`
	TurnID   string `json:"turnId"`
	QueueID  string `json:"queueId,omitempty"`
	Status   string `json:"status"`
}

func (s *CodexLinkService) ServiceStartup(ctx context.Context, options application.ServiceOptions) error {
	s.loadCloseBehavior()
	s.initializeRelay()
	return nil
}

func (s *CodexLinkService) ServiceShutdown() error {
	s.stopRelay()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.client == nil {
		return nil
	}
	err := s.client.Close()
	s.client = nil
	return err
}

func (s *CodexLinkService) loadCloseBehavior() {
	s.closeMu.Lock()
	defer s.closeMu.Unlock()
	if s.closeSettingsLoaded {
		return
	}

	s.closeSettings = defaultCloseBehaviorSettings()
	store, err := defaultCloseBehaviorStore()
	if err != nil {
		s.closeSettingsLoaded = true
		return
	}
	s.closeStore = store
	if settings, loadErr := store.Load(); loadErr == nil {
		s.closeSettings = settings
	}
	s.closeSettingsLoaded = true
}

func (s *CodexLinkService) GetCloseBehavior() CloseBehaviorSettings {
	s.loadCloseBehavior()
	s.closeMu.RLock()
	defer s.closeMu.RUnlock()
	return s.closeSettings
}

func (s *CodexLinkService) SetKeepRunningOnClose(enabled bool) (CloseBehaviorSettings, error) {
	s.loadCloseBehavior()
	s.closeMu.Lock()
	defer s.closeMu.Unlock()

	settings := CloseBehaviorSettings{KeepRunningOnClose: enabled}
	if s.closeStore == nil {
		store, err := defaultCloseBehaviorStore()
		if err != nil {
			return s.closeSettings, err
		}
		s.closeStore = store
	}
	if err := s.closeStore.Save(settings); err != nil {
		return s.closeSettings, err
	}
	s.closeSettings = settings
	return settings, nil
}

func (s *CodexLinkService) keepRunningOnClose() bool {
	return s.GetCloseBehavior().KeepRunningOnClose
}

func (s *CodexLinkService) Dashboard(ctx context.Context) (Dashboard, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	result := Dashboard{
		RefreshedAt: time.Now().Format("2006-01-02 15:04:05"),
		Threads:     []ThreadBrief{},
		Relay:       s.relayStatusSnapshot(),
	}

	if err := s.ensureCodex(ctx); err != nil {
		result.Codex.Error = err.Error()
	} else {
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
		} else {
			result.Codex.Threads = len(threads.Data)
			for _, thread := range threads.Data {
				result.Threads = append(result.Threads, toThreadBrief(thread))
			}
		}
	}

	result.Magpie = s.readMagpie(ctx)
	return result, nil
}

func (s *CodexLinkService) ReadThread(ctx context.Context, threadID string) (ThreadDetail, error) {
	return s.readThread(ctx, threadID, 0, false)
}

func (s *CodexLinkService) ReadThreadSince(ctx context.Context, threadID string, sinceTimestamp int64) (ThreadDetail, error) {
	return s.readThread(ctx, threadID, sinceTimestamp, false)
}

func (s *CodexLinkService) ReadThreadDetailed(ctx context.Context, threadID string, sinceTimestamp int64) (ThreadDetail, error) {
	return s.readThread(ctx, threadID, sinceTimestamp, true)
}

func (s *CodexLinkService) readThread(ctx context.Context, threadID string, sinceTimestamp int64, includeReasoning bool) (ThreadDetail, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	if err := s.ensureCodex(ctx); err != nil {
		return ThreadDetail{}, err
	}

	detail := ThreadDetail{
		Thread: ThreadBrief{ID: threadID},
		Items:  make([]ThreadItemView, 0, 200),
	}
	if sinceTimestamp <= 0 {
		read, err := s.client.ReadThread(ctx, codex.ThreadReadParams{
			ThreadID:     threadID,
			IncludeTurns: false,
		})
		if err != nil {
			return ThreadDetail{}, err
		}
		detail.Thread = toThreadBrief(read.Thread)

		summary, err := s.client.ListTurns(ctx, codex.ThreadTurnsListParams{
			ThreadID:      threadID,
			Limit:         50,
			SortDirection: "asc",
			ItemsView:     "summary",
		})
		if err != nil {
			return ThreadDetail{}, err
		}
		detail.Turns = len(summary.Data)
		detail.Items, detail.LatestTimestamp = threadTailItems(summary.Data, 0)

		if !includeReasoning {
			return detail, nil
		}

		// Summary mode omits reasoning and tool items. Add the newest turn in
		// full so the phone can show live thinking without loading all history.
		recent, err := s.client.ListTurns(ctx, codex.ThreadTurnsListParams{
			ThreadID:      threadID,
			Limit:         1,
			SortDirection: "desc",
			ItemsView:     "full",
		})
		if err == nil && len(recent.Data) > 0 {
			extra, _ := threadTailItems(recent.Data, 0)
			extra = limitInitialTurnItems(extra)
			detail.Items = mergeThreadItemViews(detail.Items, extra)
			detail.LatestTimestamp = latestItemTimestamp(detail.Items)
		}
		return detail, nil
	}

	limit := 12
	itemsView := "summary"
	if includeReasoning {
		limit = 2
		itemsView = "full"
	}
	turns, err := s.client.ListTurns(ctx, codex.ThreadTurnsListParams{
		ThreadID:      threadID,
		Limit:         limit,
		SortDirection: "desc",
		ItemsView:     itemsView,
	})
	if err != nil && includeReasoning {
		// Older Codex versions may not accept "full"; keep incremental sync
		// working with the lighter summary mode.
		turns, err = s.client.ListTurns(ctx, codex.ThreadTurnsListParams{
			ThreadID:      threadID,
			Limit:         12,
			SortDirection: "desc",
			ItemsView:     "summary",
		})
	}
	if err != nil {
		return ThreadDetail{}, err
	}
	for left, right := 0, len(turns.Data)-1; left < right; left, right = left+1, right-1 {
		turns.Data[left], turns.Data[right] = turns.Data[right], turns.Data[left]
	}

	detail.Turns = len(turns.Data)
	detail.Items, detail.LatestTimestamp = threadTailItems(turns.Data, sinceTimestamp)
	return detail, nil
}

func threadTailItems(turns []codex.Turn, sinceTimestamp int64) ([]ThreadItemView, int64) {
	items := make([]ThreadItemView, 0, 200)
	var latest int64
	const activeOverlapMS int64 = 20000
	for index, turn := range turns {
		timestamp := turnTimestamp(turn)
		// The newest turn can still be streaming even when another app-server
		// reports it as interrupted. Incomplete items are always returned, and
		// a short overlap catches the final update that completes an item.
		activeTurn := turnIsActive(turn) ||
			(index == len(turns)-1 && turn.CompletedAt == nil)
		cutoff := sinceTimestamp
		if activeTurn && cutoff > activeOverlapMS {
			cutoff -= activeOverlapMS
		}
		for _, item := range turn.Items {
			view, ok := toThreadItemView(item, timestamp)
			if !ok {
				continue
			}
			if turn.CompletedAt != nil {
				view.Completed = true
			}
			if view.Timestamp > latest {
				latest = view.Timestamp
			}
			if sinceTimestamp > 0 {
				fresh := view.Timestamp > 0 && view.Timestamp >= cutoff
				if !fresh && view.Completed {
					continue
				}
			}
			items = append(items, view)
		}
	}
	return items, latest
}

func mergeThreadItemViews(base, extra []ThreadItemView) []ThreadItemView {
	if len(extra) == 0 {
		return base
	}
	merged := make([]ThreadItemView, 0, len(base)+len(extra))
	indexes := make(map[string]int, len(base)+len(extra))
	for _, item := range base {
		indexes[threadItemViewKey(item)] = len(merged)
		merged = append(merged, item)
	}
	for _, item := range extra {
		key := threadItemViewKey(item)
		if index, ok := indexes[key]; ok {
			merged[index] = item
			continue
		}
		indexes[key] = len(merged)
		merged = append(merged, item)
	}
	sort.SliceStable(merged, func(left, right int) bool {
		return merged[left].Timestamp < merged[right].Timestamp
	})
	return merged
}

func threadItemViewKey(item ThreadItemView) string {
	if item.ID != "" {
		return "id:" + item.ID
	}
	return fmt.Sprintf("%s:%d:%s", item.Type, item.Timestamp, item.Command)
}

func latestItemTimestamp(items []ThreadItemView) int64 {
	var latest int64
	for _, item := range items {
		if item.Timestamp > latest {
			latest = item.Timestamp
		}
	}
	return latest
}

func limitInitialTurnItems(items []ThreadItemView) []ThreadItemView {
	const maxReasoning = 12
	const maxCommands = 12
	kept := make([]ThreadItemView, 0, len(items))
	reasoning := 0
	commands := 0
	for index := len(items) - 1; index >= 0; index-- {
		item := items[index]
		switch item.Type {
		case "reasoning":
			if reasoning >= maxReasoning {
				continue
			}
			reasoning++
		case "commandExecution":
			if commands >= maxCommands {
				continue
			}
			commands++
		}
		kept = append(kept, item)
	}
	for left, right := 0, len(kept)-1; left < right; left, right = left+1, right-1 {
		kept[left], kept[right] = kept[right], kept[left]
	}
	return kept
}

func turnIsActive(turn codex.Turn) bool {
	if turn.CompletedAt != nil {
		return false
	}
	switch strings.ToLower(strings.ReplaceAll(turn.Status, "_", "")) {
	case "inprogress", "running", "pending", "active":
		return true
	default:
		return false
	}
}

func (s *CodexLinkService) SendMessage(ctx context.Context, threadID string, text string) (SendResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return SendResult{}, errors.New("消息不能为空")
	}
	if threadID == "" {
		return SendResult{}, errors.New("请先选择一个会话")
	}

	s.opMu.Lock()
	defer s.opMu.Unlock()

	if err := s.ensureCodex(ctx); err != nil {
		return SendResult{}, err
	}

	queued, err := s.client.QueueThreadInput(ctx, threadID, text)
	if err == nil {
		return SendResult{
			ThreadID: threadID,
			QueueID:  queued.ID,
			Status:   "queued",
		}, nil
	}
	if !isQueueUnsupportedError(err) {
		return SendResult{}, fmt.Errorf("发送消息失败: %w", err)
	}

	return s.sendMessageLegacy(ctx, threadID, text)
}

func (s *CodexLinkService) sendMessageLegacy(ctx context.Context, threadID, text string) (SendResult, error) {
	if _, err := s.client.ResumeThread(ctx, codex.ThreadResumeParams{
		ThreadID:       threadID,
		ExcludeTurns:   true,
		ApprovalPolicy: "on-request",
	}); err != nil {
		if isActiveWriterError(err) {
			return SendResult{}, threadBusyError()
		}
		return SendResult{}, fmt.Errorf("恢复会话失败: %w", err)
	}

	turn, err := s.client.StartTurn(ctx, codex.TurnStartParams{
		ThreadID:       threadID,
		ApprovalPolicy: "on-request",
		Input: []codex.UserInput{
			{Type: "text", Text: text},
		},
	})
	if err != nil {
		if isActiveWriterError(err) {
			return SendResult{}, threadBusyError()
		}
		return SendResult{}, fmt.Errorf("发送消息失败: %w", err)
	}

	return SendResult{
		ThreadID: threadID,
		TurnID:   turn.Turn.ID,
		Status:   turn.Turn.Status,
	}, nil
}

func isQueueUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "-32601") ||
		strings.Contains(message, "method not found") ||
		strings.Contains(message, "unknown method") ||
		strings.Contains(message, "unknown variant") ||
		strings.Contains(message, "queue is unavailable") ||
		strings.Contains(message, "experimental")
}

func threadBusyError() error {
	return errors.New("thread_busy: 电脑端 Codex 正在使用这个会话。请在电脑上切换到其他会话，或等当前回复结束；手机端会自动重试，不会新建会话")
}

func isActiveWriterError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "active writer")
}

func (s *CodexLinkService) RefreshMagpie(ctx context.Context) (MagpieStatus, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()
	status := s.readMagpie(ctx)
	if !status.Connected {
		return status, errors.New(status.Error)
	}
	return status, nil
}

func (s *CodexLinkService) InitializeMagpieAdmin(ctx context.Context) (MagpieAdminStatus, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	store, err := magpie.DefaultCredentialStore()
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	result, err := magpie.EnsureAdmin(ctx, "", gatewayURL, store)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	return s.readMagpieAdmin(ctx, gatewayURL, result.Credentials)
}

func (s *CodexLinkService) CreateMagpieKey(ctx context.Context, name string, readOnly bool) (MagpieKeySecret, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return MagpieKeySecret{}, errors.New("密钥名称需为 1 到 120 个字符")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	client, credentials, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	result, err := client.AddGatewayKey(ctx, name, readOnly)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	keyID := newestGatewayKeyID(result.Keys, name)
	status, err := s.readMagpieAdmin(ctx, gatewayURL, credentials)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	return MagpieKeySecret{
		KeyID:   keyID,
		KeyName: name,
		Secret:  result.Secret,
		Status:  status,
	}, nil
}

func (s *CodexLinkService) RotateMagpieKey(ctx context.Context, keyID string) (MagpieKeySecret, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return MagpieKeySecret{}, errors.New("请选择要轮换的网关密钥")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	client, credentials, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	if keyID == credentials.KeyID {
		return MagpieKeySecret{}, errors.New("CodexLink 管理密钥不能从界面轮换")
	}
	result, err := client.RotateGatewayKey(ctx, keyID)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	status, err := s.readMagpieAdmin(ctx, gatewayURL, credentials)
	if err != nil {
		return MagpieKeySecret{}, err
	}
	name := ""
	for _, key := range status.GatewayKeys {
		if key.ID == keyID {
			name = key.Name
			break
		}
	}
	return MagpieKeySecret{
		KeyID:   keyID,
		KeyName: name,
		Secret:  result.Secret,
		Status:  status,
	}, nil
}

func (s *CodexLinkService) SetMagpieKeyEnabled(ctx context.Context, keyID string, enabled bool) (MagpieAdminStatus, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return MagpieAdminStatus{}, errors.New("请选择网关密钥")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	client, credentials, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	if keyID == credentials.KeyID {
		return MagpieAdminStatus{}, errors.New("CodexLink 管理密钥不能从界面停用")
	}
	if _, err := client.SetGatewayKeyEnabled(ctx, keyID, enabled); err != nil {
		return MagpieAdminStatus{}, err
	}
	return s.readMagpieAdmin(ctx, gatewayURL, credentials)
}

func (s *CodexLinkService) RemoveMagpieKey(ctx context.Context, keyID string) (MagpieAdminStatus, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		return MagpieAdminStatus{}, errors.New("请选择要删除的网关密钥")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	client, credentials, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	if keyID == credentials.KeyID {
		return MagpieAdminStatus{}, errors.New("CodexLink 管理密钥不能从界面删除")
	}
	if _, err := client.RemoveGatewayKey(ctx, keyID); err != nil {
		return MagpieAdminStatus{}, err
	}
	return s.readMagpieAdmin(ctx, gatewayURL, credentials)
}

func (s *CodexLinkService) ReplaceMagpieProviderKey(ctx context.Context, providerID, providerKey string) (MagpieAdminStatus, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	providerID = strings.TrimSpace(providerID)
	providerKey = strings.TrimSpace(providerKey)
	if providerID == "" {
		return MagpieAdminStatus{}, errors.New("请选择 Provider")
	}
	if len(providerKey) < 8 || strings.ContainsAny(providerKey, " \t\r\n") {
		return MagpieAdminStatus{}, errors.New("Provider Key 格式无效")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	client, credentials, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return MagpieAdminStatus{}, err
	}

	providers, err := client.Providers(ctx)
	if err != nil {
		return MagpieAdminStatus{}, err
	}
	var keyRef string
	for _, provider := range providers {
		if provider.ID != providerID {
			continue
		}
		for _, key := range provider.Keys {
			if key.Active {
				keyRef = key.ID
				break
			}
			if keyRef == "" {
				keyRef = key.ID
			}
		}
		break
	}
	if keyRef == "" {
		return MagpieAdminStatus{}, errors.New("该 Provider 没有可更新的密钥")
	}
	if err := client.ReplaceProviderKey(ctx, providerID, keyRef, providerKey); err != nil {
		return MagpieAdminStatus{}, err
	}
	providerKey = ""
	return s.readMagpieAdmin(ctx, gatewayURL, credentials)
}

func (s *CodexLinkService) TestMagpieProvider(ctx context.Context, providerID string) (ProviderTestSummary, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	providerID = strings.TrimSpace(providerID)
	if providerID == "" {
		return ProviderTestSummary{}, errors.New("请选择 Provider")
	}
	_, gatewayURL, err := s.magpieRuntime(ctx)
	if err != nil {
		return ProviderTestSummary{}, err
	}
	client, _, err := s.magpieAdminClient(gatewayURL)
	if err != nil {
		return ProviderTestSummary{}, err
	}
	results, err := client.TestProvider(ctx, providerID)
	if err != nil {
		return ProviderTestSummary{}, err
	}
	summary := ProviderTestSummary{
		ProviderID: providerID,
		OK:         len(results) > 0,
		TestedAt:   time.Now().Format("2006-01-02 15:04:05"),
		Results:    make([]ProviderTestEntry, 0, len(results)),
	}
	passed := 0
	for _, result := range results {
		if result.OK {
			passed++
		} else {
			summary.OK = false
		}
		summary.Results = append(summary.Results, ProviderTestEntry{
			Protocol: result.Protocol,
			Model:    result.Model,
			OK:       result.OK,
			Status:   result.Status,
			Millis:   result.Millis,
			Error:    result.Error,
			Queued:   result.Queued,
		})
	}
	if len(results) == 0 {
		summary.Summary = "没有可测试的接口"
	} else if summary.OK {
		summary.Summary = fmt.Sprintf("%d/%d 个接口测试通过", passed, len(results))
	} else {
		summary.Summary = fmt.Sprintf("%d/%d 个接口测试通过", passed, len(results))
	}
	return summary, nil
}

func (s *CodexLinkService) RunSendTest(ctx context.Context) (SendTestResult, error) {
	s.opMu.Lock()
	defer s.opMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	if err := s.ensureCodex(ctx); err != nil {
		return SendTestResult{}, err
	}

	started, err := s.client.StartThread(ctx, codex.ThreadStartParams{
		Ephemeral:      true,
		Cwd:            os.TempDir(),
		ApprovalPolicy: "never",
		Sandbox:        "read-only",
		ThreadSource:   "codexlink-desktop",
	})
	if err != nil {
		return SendTestResult{}, fmt.Errorf("创建临时测试会话失败: %w", err)
	}
	startedAt := time.Now()
	turn, err := s.client.StartTurn(ctx, codex.TurnStartParams{
		ThreadID:       started.Thread.ID,
		ApprovalPolicy: "never",
		Input: []codex.UserInput{{
			Type: "text",
			Text: "只回复 SEND_OK，不要调用任何工具，不要执行任何命令。",
		}},
	})
	if err != nil {
		return SendTestResult{}, fmt.Errorf("发送测试指令失败: %w", err)
	}
	completed, err := s.client.WaitForTurnCompleted(ctx, started.Thread.ID, turn.Turn.ID)
	if err != nil {
		return SendTestResult{}, fmt.Errorf("等待测试回复失败: %w", err)
	}

	seen := make(map[string]bool)
	var itemTypes []string
	var agentReply string
	var streamed strings.Builder
	for _, notification := range s.client.Notifications() {
		switch notification.Method {
		case "item/completed":
			var params codex.ItemCompletedParams
			if json.Unmarshal(notification.Params, &params) != nil ||
				params.ThreadID != started.Thread.ID ||
				params.TurnID != turn.Turn.ID {
				continue
			}
			if params.Item.Type != "" && !seen[params.Item.Type] {
				seen[params.Item.Type] = true
				itemTypes = append(itemTypes, params.Item.Type)
			}
			if params.Item.Type == "agentMessage" && params.Item.Text != "" {
				agentReply = params.Item.Text
			}
		case "item/agentMessage/delta":
			var params codex.AgentMessageDeltaParams
			if json.Unmarshal(notification.Params, &params) != nil {
				continue
			}
			if params.ThreadID == started.Thread.ID && params.TurnID == turn.Turn.ID {
				streamed.WriteString(params.Delta)
			}
		}
	}
	if agentReply == "" {
		agentReply = streamed.String()
	}
	return SendTestResult{
		OK:         true,
		ThreadID:   started.Thread.ID,
		TurnID:     turn.Turn.ID,
		Status:     completed.Turn.Status,
		ItemTypes:  itemTypes,
		AgentReply: strings.TrimSpace(agentReply),
		ElapsedMS:  time.Since(startedAt).Milliseconds(),
	}, nil
}

func (s *CodexLinkService) ensureCodex(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		if err := s.client.Err(); err == nil {
			return nil
		}
		_ = s.client.Close()
		s.client = nil
	}

	client := codex.New(codex.Options{})
	if err := client.Start(context.Background()); err != nil {
		return fmt.Errorf("启动 Codex 失败: %w", err)
	}

	server, err := client.Initialize(ctx)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("连接 Codex 失败: %w", err)
	}

	s.client = client
	s.server = server
	s.lastError = ""
	return nil
}

func (s *CodexLinkService) codexCallError(prefix string, err error) error {
	if s.client == nil {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	stderr := strings.TrimSpace(s.client.StderrTail())
	if stderr == "" {
		return fmt.Errorf("%s: %w", prefix, err)
	}
	if len(stderr) > 800 {
		stderr = stderr[len(stderr)-800:]
	}
	return fmt.Errorf("%s: %w; app-server: %s", prefix, err, stderr)
}

func (s *CodexLinkService) readMagpie(ctx context.Context) MagpieStatus {
	status := MagpieStatus{
		Providers: []string{},
		Quota:     []MagpieQuota{},
	}

	console, err := magpie.FindConsole()
	if err != nil {
		status.Error = err.Error()
		return status
	}

	client := magpie.New(console)
	serviceStatus, err := client.Status(ctx)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	status.Connected = serviceStatus.Running
	status.URL = serviceStatus.URL
	status.Version = serviceStatus.Version
	status.Role = serviceStatus.Role

	providers, err := client.Providers(ctx)
	if err == nil {
		status.Providers = providers
	}
	quota, err := client.Quota(ctx)
	if err == nil {
		for _, item := range quota {
			status.Quota = append(status.Quota, MagpieQuota{
				Provider: item.Provider,
				Name:     item.Name,
				Kind:     item.Kind,
				Balance:  item.Balance,
				ReadAt:   item.ReadAt,
			})
		}
	}
	keys, err := client.GatewayKeys(ctx)
	if err == nil {
		status.GatewayKeyCount = max(0, len(keys)-1)
	}
	if serviceStatus.Running && serviceStatus.URL != "" {
		adminStatus, _ := s.readMagpieAdmin(ctx, serviceStatus.URL, magpie.AdminCredentials{})
		status.Admin = adminStatus
		if adminStatus.Ready {
			status.GatewayKeyCount = len(adminStatus.GatewayKeys)
		}
	}
	return status
}

func (s *CodexLinkService) magpieRuntime(ctx context.Context) (string, string, error) {
	console, err := magpie.FindConsole()
	if err != nil {
		return "", "", err
	}
	serviceStatus, err := magpie.New(console).Status(ctx)
	if err != nil {
		return "", "", err
	}
	if !serviceStatus.Running || strings.TrimSpace(serviceStatus.URL) == "" {
		return "", "", errors.New("Magpie 网关未运行")
	}
	return console, serviceStatus.URL, nil
}

func (s *CodexLinkService) magpieAdminClient(gatewayURL string) (*magpie.AdminClient, magpie.AdminCredentials, error) {
	store, err := magpie.DefaultCredentialStore()
	if err != nil {
		return nil, magpie.AdminCredentials{}, err
	}
	credentials, err := store.Load()
	if errors.Is(err, magpie.ErrCredentialsNotFound) {
		return nil, magpie.AdminCredentials{}, errors.New("请先启用 Magpie 密钥管理")
	}
	if err != nil {
		return nil, magpie.AdminCredentials{}, err
	}
	return magpie.NewAdminClient(gatewayURL, credentials.Secret), credentials, nil
}

func (s *CodexLinkService) readMagpieAdmin(ctx context.Context, gatewayURL string, credentials magpie.AdminCredentials) (MagpieAdminStatus, error) {
	status := MagpieAdminStatus{
		Providers:   []MagpieAdminProvider{},
		GatewayKeys: []MagpieAdminGatewayKey{},
	}
	if credentials.Secret == "" {
		store, err := magpie.DefaultCredentialStore()
		if err != nil {
			status.Error = err.Error()
			return status, nil
		}
		credentials, err = store.Load()
		if errors.Is(err, magpie.ErrCredentialsNotFound) {
			return status, nil
		}
		if err != nil {
			status.Error = err.Error()
			return status, nil
		}
	}

	client := magpie.NewAdminClient(gatewayURL, credentials.Secret)
	snapshot, err := client.Snapshot(ctx)
	if err != nil {
		status.Error = err.Error()
		return status, nil
	}
	status.Ready = true
	status.KeyID = credentials.KeyID
	status.KeyName = credentials.KeyName
	status.RefreshedAt = snapshot.RefreshedAt
	for _, provider := range snapshot.Providers {
		view := MagpieAdminProvider{
			ID:        provider.ID,
			Name:      provider.Name,
			Preset:    provider.Preset,
			Host:      provider.Host,
			KeySet:    provider.KeySet,
			KeyMasked: provider.KeyMasked,
			Ready:     provider.Ready,
			Off:       provider.Off,
			Models:    provider.Models,
			KeyRefs:   make([]MagpieProviderKeyView, 0, len(provider.Keys)),
		}
		for _, key := range provider.Keys {
			view.KeyRefs = append(view.KeyRefs, MagpieProviderKeyView{
				ID:     key.ID,
				Masked: key.Masked,
				Active: key.Active,
				On:     key.On,
			})
		}
		status.Providers = append(status.Providers, view)
	}
	for _, key := range snapshot.GatewayKeys {
		status.GatewayKeys = append(status.GatewayKeys, MagpieAdminGatewayKey{
			ID:        key.ID,
			Name:      key.Name,
			Masked:    key.Masked,
			Admin:     key.Admin,
			Off:       key.Off,
			ReadOnly:  key.ReadOnly,
			Protected: key.ID == credentials.KeyID,
		})
	}
	return status, nil
}

func newestGatewayKeyID(keys []magpie.AdminGatewayKey, name string) string {
	for i := len(keys) - 1; i >= 0; i-- {
		if keys[i].Name == name {
			return keys[i].ID
		}
	}
	return ""
}

func toThreadBrief(thread codex.Thread) ThreadBrief {
	name := strings.TrimSpace(thread.Name)
	if name == "" {
		name = firstLine(thread.Preview)
	}
	if name == "" {
		name = "未命名会话"
	}
	return ThreadBrief{
		ID:           thread.ID,
		Name:         name,
		Preview:      thread.Preview,
		Cwd:          thread.Cwd,
		Model:        thread.Model,
		Provider:     thread.ModelProvider,
		UpdatedAt:    thread.UpdatedAt,
		UpdatedLabel: formatUnix(thread.UpdatedAt),
	}
}

func turnTimestamp(turn codex.Turn) int64 {
	if turn.CompletedAt != nil {
		return *turn.CompletedAt * 1000
	}
	if turn.StartedAt != nil {
		return *turn.StartedAt * 1000
	}
	return 0
}

func toThreadItemView(item codex.ThreadItem, fallbackTimestamp int64) (ThreadItemView, bool) {
	view := ThreadItemView{
		ID:        item.ID,
		Type:      item.Type,
		Command:   item.Command,
		Output:    trimText(item.AggregatedOutput, 12000),
		Completed: item.CompletedAtMs != nil,
	}
	if item.CompletedAtMs != nil {
		view.Timestamp = *item.CompletedAtMs
	} else if item.StartedAtMs != nil {
		view.Timestamp = *item.StartedAtMs
	} else {
		view.Timestamp = fallbackTimestamp
	}

	switch item.Type {
	case "userMessage":
		var parts []string
		for _, content := range item.Content {
			if content.Type == "text" && content.Text != "" {
				parts = append(parts, content.Text)
			}
		}
		view.Text = trimText(strings.Join(parts, "\n"), 20000)
	case "agentMessage":
		view.Text = trimText(item.Text, 20000)
	case "reasoning":
		view.Text = trimText(reasoningSummaryText(item.Summary), 16000)
	case "commandExecution":
		view.Command = trimText(item.Command, 4000)
	default:
		return ThreadItemView{}, false
	}

	if view.Text == "" && view.Command == "" {
		return ThreadItemView{}, false
	}
	return view, true
}

func reasoningSummaryText(summary []any) string {
	parts := make([]string, 0, len(summary))
	for _, entry := range summary {
		switch value := entry.(type) {
		case string:
			if text := strings.TrimSpace(value); text != "" {
				parts = append(parts, text)
			}
		case map[string]any:
			for _, key := range []string{"text", "summary_text", "summaryText"} {
				text, ok := value[key].(string)
				if !ok || strings.TrimSpace(text) == "" {
					continue
				}
				parts = append(parts, strings.TrimSpace(text))
				break
			}
		}
	}
	return strings.Join(parts, "\n\n")
}

func formatUnix(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	return time.Unix(seconds, 0).Format("01-02 15:04")
}

func firstLine(text string) string {
	text = strings.TrimSpace(text)
	if before, _, found := strings.Cut(text, "\n"); found {
		return strings.TrimSpace(before)
	}
	return text
}

func trimText(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + "\n\n…内容已截断"
}
