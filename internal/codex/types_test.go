package codex

import (
	"encoding/json"
	"testing"
)

func TestThreadItemsListResponseAcceptsLegacyStringContent(t *testing.T) {
	payload := `{
		"data": [{
			"item": {
				"type": "userMessage",
				"id": "item-1",
				"content": [
					"旧格式文本",
					{"type": "text", "text": "新格式文本"}
				]
			}
		}]
	}`

	var response ThreadItemsListResponse
	if err := json.Unmarshal([]byte(payload), &response); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if len(response.Data) != 1 || len(response.Data[0].Item.Content) != 2 {
		t.Fatalf("unexpected response: %#v", response)
	}
	content := response.Data[0].Item.Content
	if content[0].Type != "text" || content[0].Text != "旧格式文本" {
		t.Fatalf("legacy content = %#v", content[0])
	}
	if content[1].Type != "text" || content[1].Text != "新格式文本" {
		t.Fatalf("object content = %#v", content[1])
	}
}
