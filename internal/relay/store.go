package relay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

type sessionRecord struct {
	TokenHash  string `json:"tokenHash"`
	SessionID  string `json:"sessionId,omitempty"`
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName,omitempty"`
	CreatedAt  string `json:"createdAt,omitempty"`
	ExpiresAt  string `json:"expiresAt"`
}

type sessionStore struct {
	mu       sync.Mutex
	path     string
	sessions map[string]sessionRecord
}

var renameSessionFile = os.Rename

func newSessionStore(dataDir string) (*sessionStore, error) {
	store := &sessionStore{
		path:     filepath.Join(dataDir, "sessions.json"),
		sessions: make(map[string]sessionRecord),
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return nil, err
	}
	var saved []sessionRecord
	if err := json.Unmarshal(raw, &saved); err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	migrated := false
	for _, record := range saved {
		expires, err := time.Parse(time.RFC3339, record.ExpiresAt)
		if err != nil || !expires.After(now) || record.TokenHash == "" || record.DeviceID == "" {
			continue
		}
		if record.SessionID == "" {
			record.SessionID = sessionIDForToken(record.TokenHash)
			migrated = true
		}
		if record.CreatedAt == "" {
			record.CreatedAt = expires.Add(-defaultSessionTTL).UTC().Format(time.RFC3339)
			migrated = true
		}
		store.sessions[record.TokenHash] = record
	}
	if migrated {
		if err := store.saveLocked(); err != nil {
			return nil, err
		}
	}
	return store, nil
}

func (s *sessionStore) Add(record sessionRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if record.SessionID == "" {
		record.SessionID = randomID(12)
	}
	if record.CreatedAt == "" {
		record.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	s.sessions[record.TokenHash] = record
	return s.saveLocked()
}

func (s *sessionStore) Lookup(tokenHash string) (sessionRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, ok := s.sessions[tokenHash]
	if !ok {
		return sessionRecord{}, false
	}
	expires, err := time.Parse(time.RFC3339, record.ExpiresAt)
	if err != nil || !expires.After(time.Now().UTC()) {
		delete(s.sessions, tokenHash)
		_ = s.saveLocked()
		return sessionRecord{}, false
	}
	return record, true
}

func (s *sessionStore) CountDevice(deviceID string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	count := 0
	now := time.Now().UTC()
	for _, record := range s.sessions {
		expires, err := time.Parse(time.RFC3339, record.ExpiresAt)
		if record.DeviceID == deviceID && err == nil && expires.After(now) {
			count++
		}
	}
	return count
}

func (s *sessionStore) ListDevice(deviceID string) []sessionRecord {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	records := make([]sessionRecord, 0)
	changed := false
	for key, record := range s.sessions {
		expires, err := time.Parse(time.RFC3339, record.ExpiresAt)
		if err != nil || !expires.After(now) {
			delete(s.sessions, key)
			changed = true
			continue
		}
		if record.DeviceID != deviceID {
			continue
		}
		if record.SessionID == "" {
			record.SessionID = sessionIDForToken(record.TokenHash)
			s.sessions[key] = record
			changed = true
		}
		records = append(records, record)
	}
	if changed {
		_ = s.saveLocked()
	}
	sort.Slice(records, func(i, j int) bool {
		left := sessionSortTime(records[i])
		right := sessionSortTime(records[j])
		if !left.Equal(right) {
			return left.After(right)
		}
		return records[i].SessionID < records[j].SessionID
	})
	return records
}

func (s *sessionStore) Revoke(deviceID, sessionID string, all bool) ([]sessionRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	revoked := make([]sessionRecord, 0)
	for key, record := range s.sessions {
		if record.DeviceID != deviceID {
			continue
		}
		if !all && record.SessionID != sessionID {
			continue
		}
		delete(s.sessions, key)
		revoked = append(revoked, record)
	}
	if len(revoked) == 0 {
		return revoked, nil
	}
	if err := s.saveLocked(); err != nil {
		for _, record := range revoked {
			s.sessions[record.TokenHash] = record
		}
		return nil, err
	}
	return revoked, nil
}

func sessionIDForToken(tokenHash string) string {
	sum := sha256.Sum256([]byte("codexlink-session:" + tokenHash))
	return hex.EncodeToString(sum[:12])
}

func sessionSortTime(record sessionRecord) time.Time {
	if record.CreatedAt != "" {
		if created, err := time.Parse(time.RFC3339, record.CreatedAt); err == nil {
			return created
		}
	}
	if expires, err := time.Parse(time.RFC3339, record.ExpiresAt); err == nil {
		return expires
	}
	return time.Time{}
}

func (s *sessionStore) saveLocked() error {
	records := make([]sessionRecord, 0, len(s.sessions))
	for _, record := range s.sessions {
		records = append(records, record)
	}
	raw, err := json.Marshal(records)
	if err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(s.path), ".sessions-*.tmp")
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
	if err := renameSessionFile(tempPath, s.path); err != nil {
		if writeErr := writeSessionFile(s.path, raw); writeErr != nil {
			return writeErr
		}
	}
	return nil
}

func writeSessionFile(path string, raw []byte) error {
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
