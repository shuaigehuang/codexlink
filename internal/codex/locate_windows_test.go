//go:build windows

package codex

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveExecutableFindsCodexInstallWithoutPath(t *testing.T) {
	root := t.TempDir()
	localAppData := filepath.Join(root, "local")
	olderDir := filepath.Join(localAppData, "OpenAI", "Codex", "bin", "older")
	newerDir := filepath.Join(localAppData, "OpenAI", "Codex", "bin", "newer")
	for _, dir := range []string{olderDir, newerDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "codex.exe"), []byte("test"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now()
	if err := os.Chtimes(filepath.Join(olderDir, "codex.exe"), now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(filepath.Join(newerDir, "codex.exe"), now, now); err != nil {
		t.Fatal(err)
	}

	t.Setenv("LOCALAPPDATA", localAppData)
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	t.Setenv("ProgramFiles", filepath.Join(root, "programs"))
	t.Setenv("CODEX_CLI_PATH", "")
	t.Setenv("PATH", "")

	got, err := New(Options{}).resolveExecutable()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(newerDir, "codex.exe")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("resolved %q, want newest install %q", got, want)
	}
}
