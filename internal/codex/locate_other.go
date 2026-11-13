//go:build !windows

package codex

import (
	"os"
	"path/filepath"
)

func platformExecutableCandidates() []string {
	candidates := []string{
		"/opt/homebrew/bin/codex",
		"/usr/local/bin/codex",
		"/usr/bin/codex",
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "codex"),
			filepath.Join(home, ".codex", "bin", "codex"),
		)
	}
	return candidates
}
