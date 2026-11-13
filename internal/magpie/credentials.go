package magpie

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var ErrCredentialsNotFound = errors.New("CodexLink Magpie credentials not found")

const credentialVersion = 1

var renameCredentialFile = os.Rename

type AdminCredentials struct {
	KeyID   string `json:"keyId"`
	KeyName string `json:"keyName"`
	Secret  string `json:"secret"`
}

type CredentialStore struct {
	Path string
}

type credentialFile struct {
	Version int    `json:"version"`
	KeyID   string `json:"keyId"`
	KeyName string `json:"keyName"`
	Secret  string `json:"secret"`
}

func DefaultCredentialStore() (*CredentialStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("定位 Windows 应用数据目录失败: %w", err)
	}
	return &CredentialStore{
		Path: filepath.Join(dir, "CodexLink", "credentials.dat"),
	}, nil
}

func NewCredentialStore(path string) *CredentialStore {
	return &CredentialStore{Path: path}
}

func (s *CredentialStore) Load() (AdminCredentials, error) {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return AdminCredentials{}, ErrCredentialsNotFound
	}
	info, err := os.Stat(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return AdminCredentials{}, ErrCredentialsNotFound
	}
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("读取 Magpie 管理凭据失败: %w", err)
	}
	if info.Size() <= 0 || info.Size() > 1<<20 {
		return AdminCredentials{}, errors.New("Magpie 管理凭据文件大小异常")
	}

	encrypted, err := os.ReadFile(s.Path)
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("读取 Magpie 管理凭据失败: %w", err)
	}
	raw, err := unprotectBytes(encrypted)
	if err != nil {
		return AdminCredentials{}, fmt.Errorf("解密 Magpie 管理凭据失败: %w", err)
	}
	var saved credentialFile
	if err := json.Unmarshal(raw, &saved); err != nil {
		return AdminCredentials{}, fmt.Errorf("解析 Magpie 管理凭据失败: %w", err)
	}
	if saved.Version != credentialVersion || strings.TrimSpace(saved.Secret) == "" {
		return AdminCredentials{}, errors.New("Magpie 管理凭据格式无效")
	}
	return AdminCredentials{
		KeyID:   strings.TrimSpace(saved.KeyID),
		KeyName: strings.TrimSpace(saved.KeyName),
		Secret:  strings.TrimSpace(saved.Secret),
	}, nil
}

func (s *CredentialStore) Save(credentials AdminCredentials) error {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return errors.New("未配置 Magpie 管理凭据路径")
	}
	credentials.Secret = strings.TrimSpace(credentials.Secret)
	if credentials.Secret == "" {
		return errors.New("Magpie 管理密钥为空")
	}
	raw, err := json.Marshal(credentialFile{
		Version: credentialVersion,
		KeyID:   strings.TrimSpace(credentials.KeyID),
		KeyName: strings.TrimSpace(credentials.KeyName),
		Secret:  credentials.Secret,
	})
	if err != nil {
		return fmt.Errorf("编码 Magpie 管理凭据失败: %w", err)
	}
	encrypted, err := protectBytes(raw)
	if err != nil {
		return fmt.Errorf("使用 Windows 数据保护加密失败: %w", err)
	}

	dir := filepath.Dir(s.Path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建 Magpie 凭据目录失败: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".credentials-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时凭据文件失败: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("设置凭据文件权限失败: %w", err)
	}
	if _, err := temp.Write(encrypted); err != nil {
		temp.Close()
		return fmt.Errorf("写入凭据文件失败: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("同步凭据文件失败: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("关闭凭据文件失败: %w", err)
	}
	if err := renameCredentialFile(tempPath, s.Path); err != nil {
		if writeErr := writeCredentialFile(s.Path, encrypted); writeErr != nil {
			return fmt.Errorf("保存凭据文件失败: %w", writeErr)
		}
	}
	return nil
}

func writeCredentialFile(path string, encrypted []byte) error {
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
	if _, err := file.Write(encrypted); err != nil {
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

func (s *CredentialStore) Delete() error {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return nil
	}
	err := os.Remove(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
