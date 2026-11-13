package relay

import (
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteJSONCompressedUsesGzipForLargeResponses(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/mobile/rpc", nil)
	request.Header.Set("Accept-Encoding", "gzip")
	recorder := httptest.NewRecorder()
	payload := map[string]string{"data": strings.Repeat("历史消息", 4000)}

	writeJSONCompressed(recorder, request, http.StatusOK, payload)

	response := recorder.Result()
	defer response.Body.Close()
	if encoding := response.Header.Get("Content-Encoding"); encoding != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", encoding)
	}
	reader, err := gzip.NewReader(response.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	defer reader.Close()
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if decoded["data"] != payload["data"] {
		t.Fatal("compressed response did not preserve payload")
	}
}

func TestWriteJSONCompressedKeepsPlainResponseWithoutGzip(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/v1/mobile/rpc", nil)
	recorder := httptest.NewRecorder()

	writeJSONCompressed(recorder, request, http.StatusOK, map[string]bool{"ok": true})

	response := recorder.Result()
	defer response.Body.Close()
	if encoding := response.Header.Get("Content-Encoding"); encoding != "" {
		t.Fatalf("Content-Encoding = %q, want empty", encoding)
	}
}
