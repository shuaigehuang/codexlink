package main

import (
	"errors"
	"testing"

	"github.com/shuaigehuang/codexlink/internal/codex"
)

func TestIsActiveWriterError(t *testing.T) {
	if !isActiveWriterError(errors.New("codex rpc error -32600: thread abc already has an active writer")) {
		t.Fatal("expected active writer error to be detected")
	}
	if isActiveWriterError(errors.New("thread not found")) {
		t.Fatal("unrelated error must not be treated as an active writer")
	}
}

func TestIsQueueUnsupportedError(t *testing.T) {
	if !isQueueUnsupportedError(errors.New("codex rpc error -32601: method not found")) {
		t.Fatal("expected unsupported queue method to be detected")
	}
	if !isQueueUnsupportedError(errors.New("user message queue is unavailable")) {
		t.Fatal("expected unavailable queue to be detected")
	}
	if isQueueUnsupportedError(errors.New("codex rpc error -32600: thread not found")) {
		t.Fatal("unrelated queue error must not trigger the legacy fallback")
	}
}

func TestToThreadItemViewUsesTurnTimestampFallback(t *testing.T) {
	completedAt := int64(1794468319)
	view, ok := toThreadItemView(codex.ThreadItem{
		ID:      "item-1",
		Type:    "userMessage",
		Content: []codex.UserInput{{Type: "text", Text: "测试消息"}},
	}, turnTimestamp(codex.Turn{CompletedAt: &completedAt}))
	if !ok {
		t.Fatal("toThreadItemView() ok = false, want true")
	}
	if view.Text != "测试消息" {
		t.Fatalf("Text = %q, want 测试消息", view.Text)
	}
	if view.Timestamp != completedAt*1000 {
		t.Fatalf("Timestamp = %d, want %d", view.Timestamp, completedAt*1000)
	}
}
