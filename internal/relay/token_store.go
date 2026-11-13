package relay

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	errTokenNotFound   = errors.New("desktop token not found")
	errTokenDuplicate  = errors.New("desktop token already exists")
	errLastToken       = errors.New("the last desktop token cannot be deleted")
	renameTokenFile    = os.Rename
	minDesktopTokenLen = 32
)

type desktopTokenRecord struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TokenHash  string `json:"tokenHash"`
	TokenHint  string `json:"tokenHint,omitempty"`
	CreatedAt  string `json:"createdAt"`
	LastUsedAt string `json:"lastUsedAt,omitempty"`
	Source     string `json:"source,omitempty"`
}

type desktopTokenStore struct {
	mu     sync.Mutex
	path   string
	tokens map[string]desktopTokenRecord
}

func newDesktopTokenStore(dataDir, initialToken string) (*desktopTokenStore, error) {
	store := &desktopTokenStore{
		path:   filepath.Join(dataDir, "desktop_tokens.json"),
		tokens: make(map[string]desktopTokenRecord),
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		record := desktopTokenRecord{
			ID:        randomID(8),
			Name:      "安装时令牌",
			TokenHash: hashToken(initialToken),
			TokenHint: tokenHint(initialToken),
			CreatedAt: time.Now().UTC().Format(time.RFC3339),
			Source:    "installer",
		}
		store.tokens[record.ID] = record
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var saved []desktopTokenRecord
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, err
	}
	for _, record := range saved {
		if record.ID == "" || record.TokenHash == "" {
			continue
		}
		store.tokens[record.ID] = record
	}
	return store, nil
}

func (s *desktopTokenStore) Authenticate(token string) (desktopTokenRecord, bool) {
	if strings.TrimSpace(token) == "" {
		return desktopTokenRecord{}, false
	}
	want := hashToken(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, record := range s.tokens {
		if subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(want)) != 1 {
			continue
		}
		return record, true
	}
	return desktopTokenRecord{}, false
}

func (s *desktopTokenStore) MarkUsed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.tokens[id]
	if !ok {
		return errTokenNotFound
	}
	record.LastUsedAt = time.Now().UTC().Format(time.RFC3339)
	s.tokens[id] = record
	return s.saveLocked()
}

func (s *desktopTokenStore) List() []desktopTokenRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	records := make([]desktopTokenRecord, 0, len(s.tokens))
	for _, record := range s.tokens {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].CreatedAt != records[j].CreatedAt {
			return records[i].CreatedAt < records[j].CreatedAt
		}
		return records[i].ID < records[j].ID
	})
	return records
}

func (s *desktopTokenStore) Add(name, token string) (desktopTokenRecord, string, error) {
	name = limitText(name, 80)
	if name == "" {
		name = "电脑端令牌"
	}
	if token == "" {
		token = randomToken(32)
	}
	if len(token) < minDesktopTokenLen || len(token) > 256 {
		return desktopTokenRecord{}, "", errors.New("令牌长度需在 32 到 256 个字符之间")
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	tokenDigest := hashToken(token)
	for _, record := range s.tokens {
		if subtle.ConstantTimeCompare([]byte(record.TokenHash), []byte(tokenDigest)) == 1 {
			return desktopTokenRecord{}, "", errTokenDuplicate
		}
	}
	record := desktopTokenRecord{
		ID:        randomID(8),
		Name:      name,
		TokenHash: tokenDigest,
		TokenHint: tokenHint(token),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Source:    "admin",
	}
	s.tokens[record.ID] = record
	if err := s.saveLocked(); err != nil {
		delete(s.tokens, record.ID)
		return desktopTokenRecord{}, "", err
	}
	return record, token, nil
}

func (s *desktopTokenStore) Delete(id string) (desktopTokenRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.tokens[id]
	if !ok {
		return desktopTokenRecord{}, errTokenNotFound
	}
	if len(s.tokens) <= 1 {
		return desktopTokenRecord{}, errLastToken
	}
	delete(s.tokens, id)
	if err := s.saveLocked(); err != nil {
		s.tokens[id] = record
		return desktopTokenRecord{}, err
	}
	return record, nil
}

func (s *desktopTokenStore) Count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tokens)
}

func (s *desktopTokenStore) saveLocked() error {
	records := make([]desktopTokenRecord, 0, len(s.tokens))
	for _, record := range s.tokens {
		records = append(records, record)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CreatedAt < records[j].CreatedAt
	})
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".desktop-tokens-*.tmp")
	if err != nil {
		return err
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
	if err := renameTokenFile(tempPath, s.path); err != nil {
		return writeTokenFile(s.path, raw)
	}
	return nil
}

func writeTokenFile(path string, raw []byte) error {
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

func tokenHint(token string) string {
	if len(token) <= 12 {
		return "已隐藏"
	}
	return token[:6] + "..." + token[len(token)-4:]
}
