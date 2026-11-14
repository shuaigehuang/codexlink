package main

import (
	"errors"
	"fmt"
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

func TestThreadTailItemsFiltersHistoryAndTracksLatest(t *testing.T) {
	first := int64(1000)
	second := int64(2000)
	completedAt := int64(2)
	turns := []codex.Turn{{
		Status:      "completed",
		CompletedAt: &completedAt,
		Items: []codex.ThreadItem{
			{
				ID:            "old-message",
				Type:          "userMessage",
				Content:       []codex.UserInput{{Type: "text", Text: "old"}},
				CompletedAtMs: &first,
			},
			{
				ID:            "new-message",
				Type:          "agentMessage",
				Text:          "new",
				CompletedAtMs: &second,
			},
		},
	}}

	items, latest := threadTailItems(turns, second)
	if latest != second {
		t.Fatalf("latest = %d, want %d", latest, second)
	}
	if len(items) != 1 || items[0].ID != "new-message" {
		t.Fatalf("unexpected tail items: %+v", items)
	}
}

func TestThreadTailItemsIncludesEveryItemOfActiveTurn(t *testing.T) {
	old := int64(1000)
	latest := int64(3000)
	turns := []codex.Turn{
		{
			Status:      "completed",
			CompletedAt: &old,
			Items: []codex.ThreadItem{{
				ID:            "already-seen",
				Type:          "agentMessage",
				Text:          "old",
				StartedAtMs:   &old,
				CompletedAtMs: &old,
			}},
		},
		{
			ID:     "turn-active",
			Status: "inProgress",
			Items: []codex.ThreadItem{
				{
					ID:          "streamed-reply",
					Type:        "agentMessage",
					Text:        "partial",
					StartedAtMs: &old,
				},
			},
		},
	}

	items, newest := threadTailItems(turns, latest)
	if newest != old {
		t.Fatalf("latest = %d, want %d", newest, old)
	}
	if len(items) != 1 || items[0].ID != "streamed-reply" {
		t.Fatalf("active turn items were filtered out: %+v", items)
	}
}

func TestThreadTailItemsKeepsSameTimestampUpdates(t *testing.T) {
	updated := int64(2000)
	turns := []codex.Turn{{
		ID:          "turn-completed",
		Status:      "completed",
		CompletedAt: &updated,
		Items: []codex.ThreadItem{{
			ID:          "final-message",
			Type:        "agentMessage",
			Text:        "final text",
			StartedAtMs: &updated,
		}},
	}}

	items, _ := threadTailItems(turns, updated)
	if len(items) != 1 || items[0].Text != "final text" {
		t.Fatalf("same-timestamp update was filtered out: %+v", items)
	}
}

func TestThreadTailItemsSkipsOldCompletedItemsInActiveTurn(t *testing.T) {
	old := int64(1000)
	streaming := int64(200000)
	turns := []codex.Turn{{
		ID:     "turn-active",
		Status: "inProgress",
		Items: []codex.ThreadItem{
			{
				ID:            "old-reasoning",
				Type:          "reasoning",
				Summary:       []any{"old thought"},
				StartedAtMs:   &old,
				CompletedAtMs: &old,
			},
			{
				ID:          "streaming-reasoning",
				Type:        "reasoning",
				Summary:     []any{"live thought"},
				StartedAtMs: &streaming,
			},
		},
	}}

	items, _ := threadTailItems(turns, 100000)
	if len(items) != 1 || items[0].ID != "streaming-reasoning" {
		t.Fatalf("old completed item was resent: %+v", items)
	}
}

func TestReasoningSummaryTextSupportsStringsAndObjects(t *testing.T) {
	text := reasoningSummaryText([]any{
		"first",
		map[string]any{"type": "summary_text", "text": "second"},
	})
	if text != "first\n\nsecond" {
		t.Fatalf("reasoning text = %q", text)
	}
}

func TestToThreadItemViewIncludesReasoning(t *testing.T) {
	started := int64(1794468319000)
	view, ok := toThreadItemView(codex.ThreadItem{
		ID:          "reasoning-1",
		Type:        "reasoning",
		Summary:     []any{"thinking..."},
		StartedAtMs: &started,
	}, 0)
	if !ok {
		t.Fatal("reasoning item was skipped")
	}
	if view.Type != "reasoning" || view.Text != "thinking..." {
		t.Fatalf("unexpected reasoning view: %+v", view)
	}
	if view.Completed {
		t.Fatal("streaming reasoning must not be marked completed")
	}
}

func TestMergeThreadItemViewsReplacesAndSorts(t *testing.T) {
	base := []ThreadItemView{{
		ID:        "message-1",
		Type:      "agentMessage",
		Text:      "partial",
		Timestamp: 100,
	}}
	extra := []ThreadItemView{
		{
			ID:        "reasoning-1",
			Type:      "reasoning",
			Text:      "thought",
			Timestamp: 50,
		},
		{
			ID:        "message-1",
			Type:      "agentMessage",
			Text:      "final",
			Timestamp: 100,
			Completed: true,
		},
	}

	merged := mergeThreadItemViews(base, extra)
	if len(merged) != 2 {
		t.Fatalf("merged length = %d, want 2", len(merged))
	}
	if merged[0].ID != "reasoning-1" || merged[1].Text != "final" {
		t.Fatalf("unexpected merged items: %+v", merged)
	}
}

func TestLimitInitialTurnItemsCapsReasoningAndCommands(t *testing.T) {
	items := make([]ThreadItemView, 0, 42)
	for index := 0; index < 20; index++ {
		items = append(items,
			ThreadItemView{ID: fmt.Sprintf("r-%d", index), Type: "reasoning", Timestamp: int64(index)},
			ThreadItemView{ID: fmt.Sprintf("c-%d", index), Type: "commandExecution", Timestamp: int64(100 + index)},
		)
	}
	items = append(items,
		ThreadItemView{ID: "message-1", Type: "userMessage", Timestamp: 500},
		ThreadItemView{ID: "message-2", Type: "agentMessage", Timestamp: 501},
	)

	limited := limitInitialTurnItems(items)
	reasoning := 0
	commands := 0
	messages := 0
	for _, item := range limited {
		switch item.Type {
		case "reasoning":
			reasoning++
		case "commandExecution":
			commands++
		default:
			messages++
		}
	}
	if reasoning != 12 || commands != 12 || messages != 2 {
		t.Fatalf("counts = reasoning %d, commands %d, messages %d", reasoning, commands, messages)
	}
	if limited[len(limited)-1].ID != "message-2" {
		t.Fatalf("newest item was not kept: %+v", limited[len(limited)-1])
	}
}
