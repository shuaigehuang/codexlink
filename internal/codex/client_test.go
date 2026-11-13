package codex

import (
	"encoding/json"
	"testing"
)

func TestParseMessageID(t *testing.T) {
	tests := []struct {
		input string
		want  int64
		ok    bool
	}{
		{input: "42", want: 42, ok: true},
		{input: `"42"`, want: 42, ok: true},
		{input: "", ok: false},
		{input: "null", ok: false},
	}

	for _, test := range tests {
		got, err := parseMessageID(json.RawMessage(test.input))
		if test.ok && err != nil {
			t.Fatalf("parseMessageID(%q) returned error: %v", test.input, err)
		}
		if !test.ok && err == nil {
			t.Fatalf("parseMessageID(%q) expected an error", test.input)
		}
		if got != test.want {
			t.Fatalf("parseMessageID(%q) = %d, want %d", test.input, got, test.want)
		}
	}
}
