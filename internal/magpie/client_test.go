package magpie

import (
	"strings"
	"testing"
)

func TestRedact(t *testing.T) {
	input := "key sk-1234567890abcdef and sk-abcdef1234567890"
	output := Redact(input)
	if strings.Contains(output, "1234567890abcdef") || strings.Contains(output, "abcdef1234567890") {
		t.Fatalf("Redact leaked a key: %s", output)
	}
	if !strings.Contains(output, "sk-***") {
		t.Fatalf("Redact did not mark the key: %s", output)
	}
}
