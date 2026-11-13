package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const closeBehaviorVersion = 1

type CloseBehaviorSettings struct {
	KeepRunningOnClose bool `json:"keepRunningOnClose"`
}

type closeBehaviorFile struct {
	Version            int  `json:"version"`
	KeepRunningOnClose bool `json:"keepRunningOnClose"`
}

type closeBehaviorStore struct {
	path string
}

func defaultCloseBehaviorStore() (*closeBehaviorStore, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("定位应用数据目录失败: %w", err)
	}
	return &closeBehaviorStore{
		path: filepath.Join(dir, "CodexLink", "settings.json"),
	}, nil
}

func defaultCloseBehaviorSettings() CloseBehaviorSettings {
	return CloseBehaviorSettings{KeepRunningOnClose: true}
}

func (s *closeBehaviorStore) Load() (CloseBehaviorSettings, error) {
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultCloseBehaviorSettings(), nil
	}
	if err != nil {
		return defaultCloseBehaviorSettings(), fmt.Errorf("读取运行选项失败: %w", err)
	}

	var stored closeBehaviorFile
	if err := json.Unmarshal(raw, &stored); err != nil {
		return defaultCloseBehaviorSettings(), fmt.Errorf("解析运行选项失败: %w", err)
	}
	if stored.Version != closeBehaviorVersion {
		return defaultCloseBehaviorSettings(), fmt.Errorf("运行选项版本不受支持: %d", stored.Version)
	}
	return CloseBehaviorSettings{KeepRunningOnClose: stored.KeepRunningOnClose}, nil
}

func (s *closeBehaviorStore) Save(settings CloseBehaviorSettings) error {
	raw, err := json.Marshal(closeBehaviorFile{
		Version:            closeBehaviorVersion,
		KeepRunningOnClose: settings.KeepRunningOnClose,
	})
	if err != nil {
		return fmt.Errorf("编码运行选项失败: %w", err)
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("创建配置目录失败: %w", err)
	}
	temp, err := os.CreateTemp(dir, ".settings-*.tmp")
	if err != nil {
		return fmt.Errorf("创建临时配置失败: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
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
	if err := os.Rename(tempPath, s.path); err != nil {
		if removeErr := os.Remove(s.path); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			return fmt.Errorf("替换运行选项失败: %w", err)
		}
		if renameErr := os.Rename(tempPath, s.path); renameErr != nil {
			return fmt.Errorf("保存运行选项失败: %w", renameErr)
		}
	}
	return nil
}
