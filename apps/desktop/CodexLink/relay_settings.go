package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/shuaigehuang/codexlink/internal/dpapi"
)

const (
	relayConfigVersion = 1
	relayMinTokenBytes = 32
)

var (
	errRelayNotConfigured = errors.New("中继尚未配置")
	relayConfigEntropy    = []byte("CodexLink.Relay.Desktop.v1")
	renameRelayFile       = os.Rename
)

type relayConfig struct {
	Version    int    `json:"version"`
	URL        string `json:"url"`
	Token      string `json:"token"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
}

type relayConfigStore struct {
	path string
}

func defaultRelayConfigStore() (*relayConfigStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("定位应用数据目录失败: %w", err)
	}
	return &relayConfigStore{
		path: filepath.Join(dir, "CodexLink", "relay.dat"),
	}, nil
}

func (s *relayConfigStore) Load() (relayConfig, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return relayConfig{}, errRelayNotConfigured
	}
	if err != nil {
		return relayConfig{}, fmt.Errorf("读取中继配置失败: %w", err)
	}
	decoded, err := dpapi.Unprotect(raw, relayConfigEntropy)
	if err != nil {
		return relayConfig{}, fmt.Errorf("解密中继配置失败: %w", err)
	}
	var cfg relayConfig
	if err := json.Unmarshal(decoded, &cfg); err != nil {
		return relayConfig{}, fmt.Errorf("解析中继配置失败: %w", err)
	}
	if cfg.Version != relayConfigVersion || strings.TrimSpace(cfg.Token) == "" {
		return relayConfig{}, errors.New("中继配置格式无效")
	}
	normalized, err := normalizeRelayURL(cfg.URL)
	if err != nil {
		return relayConfig{}, err
	}
	cfg.URL = normalized
	cfg.Token = strings.TrimSpace(cfg.Token)
	if !validRelayDeviceID(cfg.DeviceID) {
		cfg.DeviceID = newRelayDeviceID()
	}
	if strings.TrimSpace(cfg.DeviceName) == "" {
		cfg.DeviceName = defaultRelayDeviceName()
	}
	return cfg, nil
}

func (s *relayConfigStore) Save(cfg relayConfig) error {
	cfg.Version = relayConfigVersion
	cfg.URL = strings.TrimSpace(cfg.URL)
	cfg.Token = strings.TrimSpace(cfg.Token)
	cfg.DeviceName = limitRunes(strings.TrimSpace(cfg.DeviceName), 80)
	if cfg.DeviceName == "" {
		cfg.DeviceName = defaultRelayDeviceName()
	}
	if !validRelayDeviceID(cfg.DeviceID) {
		cfg.DeviceID = newRelayDeviceID()
	}
	if len(cfg.Token) < relayMinTokenBytes {
		return fmt.Errorf("中继密钥至少需要 %d 个字符", relayMinTokenBytes)
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("编码中继配置失败: %w", err)
	}
	encrypted, err := dpapi.Protect(raw, relayConfigEntropy)
	if err != nil {
		return fmt.Errorf("使用 Windows 数据保护加密失败: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建中继配置目录失败: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".relay-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置失败: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(encrypted); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := renameRelayFile(tempPath, s.path); err != nil {
		if writeErr := writeRelayFile(s.path, encrypted); writeErr != nil {
			return fmt.Errorf("保存中继配置失败: %w", writeErr)
		}
	}
	return nil
}

func (s *relayConfigStore) Delete() error {
	err := os.Remove(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func writeRelayFile(path string, raw []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		file.Close()
		if !complete {
			os.Remove(path)
		}
	}()
	if err := file.Chmod(0o600); err != nil {
		return err
	}
	if _, err := file.Write(raw); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func normalizeRelayURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("请填写中继地址")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("中继地址格式无效")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("中继地址不能包含账号、查询参数或锚点")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("中继地址需使用 https:// 或 http://")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.Scheme + "://" + parsed.Host + parsed.Path, nil
}

func relayURLInsecure(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	return !isLoopbackHost(parsed.Hostname())
}

func relayWebSocketURL(base, deviceID, deviceName string) (string, error) {
	parsed, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch parsed.Scheme {
	case "https":
		parsed.Scheme = "wss"
	case "http":
		parsed.Scheme = "ws"
	default:
		return "", errors.New("中继地址协议无效")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/v1/desktop/connect"
	query := parsed.Query()
	query.Set("device", deviceID)
	query.Set("name", deviceName)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func newRelayDeviceID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	return hex.EncodeToString(raw)
}

func validRelayDeviceID(deviceID string) bool {
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

func defaultRelayDeviceName() string {
	name, err := os.Hostname()
	if err != nil || strings.TrimSpace(name) == "" {
		return "Windows PC"
	}
	return limitRunes(name, 80)
}

func limitRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) > max {
		return string(runes[:max])
	}
	return value
}
