package magpie

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

const AdminKeyName = "CodexLink Desktop Admin"

type AdminBootstrap struct {
	Credentials AdminCredentials
	Rotated     bool
}

func EnsureAdmin(ctx context.Context, consolePath, baseURL string, store *CredentialStore) (AdminBootstrap, error) {
	if store == nil {
		return AdminBootstrap{}, errors.New("未配置管理凭据存储")
	}
	if credentials, err := store.Load(); err == nil {
		client := NewAdminClient(baseURL, credentials.Secret)
		if err := client.Ping(ctx); err == nil {
			return AdminBootstrap{Credentials: credentials}, nil
		}
	}

	client := New(consolePath)
	listOutput, err := client.Run(ctx, "gateway-key", "list")
	if err != nil {
		return AdminBootstrap{}, err
	}
	id, found := findGatewayKeyID(listOutput, AdminKeyName)

	var secret string
	if found {
		secret, err = client.RunSecret(ctx, "gateway-key", "rotate", id)
	} else {
		secret, err = client.RunSecret(ctx, "gateway-key", "add", AdminKeyName)
	}
	if err != nil {
		return AdminBootstrap{}, err
	}
	secret = strings.TrimSpace(secret)
	if !strings.HasPrefix(secret, "sk-magpie-key-") || len(secret) < 40 || strings.ContainsAny(secret, " \t\r\n") {
		return AdminBootstrap{}, errors.New("Magpie 返回的管理密钥格式无效")
	}

	if !found {
		listOutput, err = client.Run(ctx, "gateway-key", "list")
		if err != nil {
			return AdminBootstrap{}, err
		}
		id, found = findGatewayKeyID(listOutput, AdminKeyName)
		if !found {
			return AdminBootstrap{}, errors.New("已创建管理密钥，但未能从 Magpie 密钥列表读取其 ID")
		}
	}

	if _, err := client.Run(ctx, "gateway-key", "admin", id, "local"); err != nil {
		return AdminBootstrap{}, fmt.Errorf("授予管理权限失败: %w", err)
	}

	credentials := AdminCredentials{
		KeyID:   id,
		KeyName: AdminKeyName,
		Secret:  secret,
	}
	admin := NewAdminClient(baseURL, secret)
	if err := admin.Ping(ctx); err != nil {
		return AdminBootstrap{}, fmt.Errorf("管理接口验证失败: %w", err)
	}
	if err := store.Save(credentials); err != nil {
		return AdminBootstrap{}, err
	}
	return AdminBootstrap{Credentials: credentials, Rotated: found}, nil
}

func findGatewayKeyID(output, name string) (string, bool) {
	pattern := regexp.MustCompile(`^\s*([0-9a-f]{16,64})\s+` + regexp.QuoteMeta(strings.TrimSpace(name)) + `(?:\s|$)`)
	for _, line := range strings.Split(output, "\n") {
		if match := pattern.FindStringSubmatch(strings.TrimSpace(line)); len(match) == 2 {
			return match[1], true
		}
	}
	return "", false
}
