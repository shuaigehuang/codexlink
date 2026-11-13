package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloseBehaviorStoreDefaultsToKeepRunning(t *testing.T) {
	store := &closeBehaviorStore{path: filepath.Join(t.TempDir(), "settings.json")}

	settings, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if !settings.KeepRunningOnClose {
		t.Fatal("default KeepRunningOnClose = false, want true")
	}
}

func TestCloseBehaviorStoreRoundTrip(t *testing.T) {
	store := &closeBehaviorStore{path: filepath.Join(t.TempDir(), "nested", "settings.json")}
	want := CloseBehaviorSettings{KeepRunningOnClose: false}

	if err := store.Save(want); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
}

func TestCloseBehaviorStoreFallsBackForInvalidFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := &closeBehaviorStore{path: path}

	settings, err := store.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want parse error")
	}
	if !settings.KeepRunningOnClose {
		t.Fatal("KeepRunningOnClose = false, want safe default true")
	}
}
