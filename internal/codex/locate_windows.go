//go:build windows

package codex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func platformExecutableCandidates() []string {
	var candidates []string
	if localAppData := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); localAppData != "" {
		matches, _ := filepath.Glob(filepath.Join(localAppData, "OpenAI", "Codex", "bin", "*", "codex.exe"))
		sort.Slice(matches, func(i, j int) bool {
			left, leftErr := os.Stat(matches[i])
			right, rightErr := os.Stat(matches[j])
			if leftErr == nil && rightErr == nil && !left.ModTime().Equal(right.ModTime()) {
				return left.ModTime().After(right.ModTime())
			}
			return matches[i] > matches[j]
		})
		candidates = append(candidates, matches...)
		candidates = append(candidates,
			filepath.Join(localAppData, "Programs", "Codex", "codex.exe"),
			filepath.Join(localAppData, "Codex", "codex.exe"),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".codex", ".sandbox-bin", "codex.exe"),
			filepath.Join(home, ".codex", "plugins", ".plugin-appserver", "codex.exe"),
			filepath.Join(home, ".codex", "bin", "codex.exe"),
		)
	}
	if programFiles := strings.TrimSpace(os.Getenv("ProgramFiles")); programFiles != "" {
		candidates = append(candidates, filepath.Join(programFiles, "Codex", "codex.exe"))
	}
	return uniqueCandidates(candidates)
}

func uniqueCandidates(candidates []string) []string {
	seen := make(map[string]bool, len(candidates))
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(candidate))
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, candidate)
	}
	return result
}
