package codex

import (
	"encoding/json"
	"strings"
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

func TestReadLoopAcceptsResponseLargerThanThirtyTwoMegabytes(t *testing.T) {
	const payloadSize = 33 * 1024 * 1024
	payload := strings.Repeat("x", payloadSize)
	message := `{"id":1,"result":{"data":"` + payload + `"}}` + "\n"

	client := New(Options{})
	resultChannel := make(chan callResult, 1)
	client.pending[1] = resultChannel

	client.readLoop(strings.NewReader(message))

	result := <-resultChannel
	if result.err != nil {
		t.Fatalf("readLoop returned an error for a large response: %v", result.err)
	}
	if len(result.result) <= payloadSize {
		t.Fatalf("result length = %d, want more than %d", len(result.result), payloadSize)
	}
}

func TestInitializeCapabilitiesEnableExperimentalQueueAPI(t *testing.T) {
	params := InitializeParams{
		ClientInfo: ClientInfo{Name: "codexlink", Version: "test"},
		Capabilities: InitializeCapabilities{
			ExperimentalAPI: true,
		},
	}
	data, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Capabilities struct {
			ExperimentalAPI bool `json:"experimentalApi"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if !decoded.Capabilities.ExperimentalAPI {
		t.Fatalf("experimentalApi was not advertised: %s", data)
	}
}

func TestClientMessageIDIsRandomAndUsable(t *testing.T) {
	first := newClientMessageID()
	second := newClientMessageID()
	if len(first) != 32 || len(second) != 32 {
		t.Fatalf("unexpected client message ID length: %q %q", first, second)
	}
	if first == second {
		t.Fatal("client message IDs should be unique")
	}
}
