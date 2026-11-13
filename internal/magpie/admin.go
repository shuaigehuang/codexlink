package magpie

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const adminPath = "/v1/magpie/admin/"
const adminUserAgent = "CodexLink/0.1.0"

type AdminProviderKey struct {
	ID     string `json:"id"`
	Masked string `json:"masked"`
	Active bool   `json:"active"`
	On     bool   `json:"on"`
}

type AdminProvider struct {
	ID        string             `json:"id"`
	Name      string             `json:"name"`
	Preset    string             `json:"preset"`
	Host      string             `json:"host"`
	KeySet    bool               `json:"keySet"`
	KeyMasked string             `json:"keyMasked"`
	Ready     bool               `json:"ready"`
	Off       bool               `json:"off"`
	Models    int                `json:"models"`
	Keys      []AdminProviderKey `json:"keys"`
}

type AdminGatewayKey struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Masked   string `json:"masked"`
	Admin    string `json:"admin"`
	Off      bool   `json:"off"`
	ReadOnly bool   `json:"readOnly"`
}

type AdminSnapshot struct {
	Providers   []AdminProvider   `json:"providers"`
	GatewayKeys []AdminGatewayKey `json:"gatewayKeys"`
	RefreshedAt string            `json:"refreshedAt"`
}

type AdminKeyResult struct {
	Keys   []AdminGatewayKey `json:"keys"`
	Secret string            `json:"secret,omitempty"`
}

type ProviderTestResult struct {
	Account  string `json:"account,omitempty"`
	Protocol string `json:"protocol"`
	OK       bool   `json:"ok"`
	Status   int    `json:"status,omitempty"`
	Millis   int64  `json:"ms"`
	Model    string `json:"model,omitempty"`
	Error    string `json:"error,omitempty"`
	Queued   string `json:"queued,omitempty"`
}

type AdminClient struct {
	BaseURL   string
	Secret    string
	HTTP      *http.Client
	UserAgent string
}

func NewAdminClient(baseURL, secret string) *AdminClient {
	return &AdminClient{
		BaseURL:   strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		Secret:    strings.TrimSpace(secret),
		HTTP:      &http.Client{Timeout: 40 * time.Second},
		UserAgent: adminUserAgent,
	}
}

func (c *AdminClient) Snapshot(ctx context.Context) (AdminSnapshot, error) {
	providers, err := c.Providers(ctx)
	if err != nil {
		return AdminSnapshot{}, err
	}
	keys, err := c.GatewayKeys(ctx)
	if err != nil {
		return AdminSnapshot{}, err
	}
	return AdminSnapshot{
		Providers:   providers,
		GatewayKeys: keys,
		RefreshedAt: time.Now().Format("2006-01-02 15:04:05"),
	}, nil
}

func (c *AdminClient) Ping(ctx context.Context) error {
	_, err := c.Providers(ctx)
	return err
}

func (c *AdminClient) Providers(ctx context.Context) ([]AdminProvider, error) {
	raw, err := c.do(ctx, http.MethodGet, "providers", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Providers []struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Preset string `json:"preset"`
			Host   string `json:"host"`
			Ready  bool   `json:"ready"`
			Off    bool   `json:"off"`
			Key    struct {
				Set    bool   `json:"set"`
				Masked string `json:"masked"`
			} `json:"key"`
			KeyList []AdminProviderKey `json:"keyList"`
			Models  []json.RawMessage  `json:"models"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode magpie providers: %w", err)
	}
	out := make([]AdminProvider, 0, len(response.Providers))
	for _, p := range response.Providers {
		out = append(out, AdminProvider{
			ID:        p.ID,
			Name:      p.Name,
			Preset:    p.Preset,
			Host:      p.Host,
			KeySet:    p.Key.Set,
			KeyMasked: p.Key.Masked,
			Ready:     p.Ready,
			Off:       p.Off,
			Models:    len(p.Models),
			Keys:      p.KeyList,
		})
	}
	return out, nil
}

func (c *AdminClient) GatewayKeys(ctx context.Context) ([]AdminGatewayKey, error) {
	raw, err := c.do(ctx, http.MethodGet, "gateway-keys", nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		Keys []AdminGatewayKey `json:"keys"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode magpie gateway keys: %w", err)
	}
	return response.Keys, nil
}

func (c *AdminClient) AddGatewayKey(ctx context.Context, name string, readOnly bool) (AdminKeyResult, error) {
	body := map[string]any{
		"name":     strings.TrimSpace(name),
		"readOnly": readOnly,
	}
	return c.gatewayKeyAction(ctx, "add-key", body)
}

func (c *AdminClient) RotateGatewayKey(ctx context.Context, id string) (AdminKeyResult, error) {
	return c.gatewayKeyAction(ctx, "rotate-key", map[string]any{"key": strings.TrimSpace(id)})
}

func (c *AdminClient) SetGatewayKeyEnabled(ctx context.Context, id string, enabled bool) (AdminKeyResult, error) {
	action := "off-key"
	if enabled {
		action = "on-key"
	}
	return c.gatewayKeyAction(ctx, action, map[string]any{"key": strings.TrimSpace(id)})
}

func (c *AdminClient) RemoveGatewayKey(ctx context.Context, id string) (AdminKeyResult, error) {
	return c.gatewayKeyAction(ctx, "remove-key", map[string]any{"key": strings.TrimSpace(id)})
}

func (c *AdminClient) gatewayKeyAction(ctx context.Context, action string, body any) (AdminKeyResult, error) {
	raw, err := c.do(ctx, http.MethodPost, "gateway-keys/"+action, body)
	if err != nil {
		return AdminKeyResult{}, err
	}
	var response struct {
		Keys   []AdminGatewayKey `json:"keys"`
		Secret string            `json:"secret"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return AdminKeyResult{}, fmt.Errorf("decode magpie gateway key change: %w", err)
	}
	return AdminKeyResult{Keys: response.Keys, Secret: response.Secret}, nil
}

func (c *AdminClient) ReplaceProviderKey(ctx context.Context, providerID, keyRef, providerKey string) error {
	providerID = strings.TrimSpace(providerID)
	keyRef = strings.TrimSpace(keyRef)
	providerKey = strings.TrimSpace(providerKey)
	if providerID == "" || keyRef == "" {
		return errors.New("找不到要更新的 Provider 密钥")
	}
	if providerKey == "" {
		return errors.New("Provider Key 不能为空")
	}
	body := map[string]any{
		"id":  providerID,
		"ref": keyRef,
		"key": providerKey,
	}
	_, err := c.do(ctx, http.MethodPost, "keys/replace", body)
	if err != nil {
		return sanitizeSecretError(err, providerKey)
	}
	return nil
}

func (c *AdminClient) TestProvider(ctx context.Context, providerID string) ([]ProviderTestResult, error) {
	raw, err := c.do(ctx, http.MethodPost, "provider/test", map[string]any{
		"id": strings.TrimSpace(providerID),
	})
	if err != nil {
		return nil, err
	}
	var response struct {
		Results []ProviderTestResult `json:"results"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, fmt.Errorf("decode magpie provider test: %w", err)
	}
	return response.Results, nil
}

func (c *AdminClient) do(ctx context.Context, method, path string, body any) ([]byte, error) {
	if strings.TrimSpace(c.BaseURL) == "" {
		return nil, errors.New("Magpie 网关地址为空")
	}
	if strings.TrimSpace(c.Secret) == "" {
		return nil, errors.New("尚未初始化 Magpie 管理密钥")
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, errors.New("Magpie 网关地址无效")
	}

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}

	endpoint := c.BaseURL + adminPath + strings.TrimLeft(path, "/")
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, fmt.Errorf("创建 Magpie 管理请求失败: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+c.Secret)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}

	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 40 * time.Second}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("连接 Magpie 管理接口失败: %w", c.sanitizeError(err))
	}
	defer response.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(response.Body, 12<<20))
	if err != nil {
		return nil, fmt.Errorf("读取 Magpie 管理响应失败: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		message := strings.TrimSpace(string(raw))
		var apiError struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(raw, &apiError) == nil && strings.TrimSpace(apiError.Error) != "" {
			message = strings.TrimSpace(apiError.Error)
		}
		if message == "" {
			message = response.Status
		}
		return nil, fmt.Errorf("Magpie 管理请求失败 (%s): %s", response.Status, c.sanitizeText(message))
	}
	return raw, nil
}

func (c *AdminClient) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(c.sanitizeText(err.Error()))
}

func (c *AdminClient) sanitizeText(text string) string {
	text = Redact(text)
	if secret := strings.TrimSpace(c.Secret); secret != "" {
		text = strings.ReplaceAll(text, secret, "[管理密钥已隐藏]")
	}
	return text
}

func sanitizeSecretError(err error, secret string) error {
	if err == nil {
		return nil
	}
	text := Redact(err.Error())
	if secret = strings.TrimSpace(secret); secret != "" {
		text = strings.ReplaceAll(text, secret, "[密钥已隐藏]")
	}
	return errors.New(text)
}
