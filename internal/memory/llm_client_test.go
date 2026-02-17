package memory

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLLMClientExtract(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"facts\":[\"hello\"]}"}}]}`))
	}))
	defer server.Close()

	client, err := NewLLMClient(nil, server.URL, "test-key", "gpt-4.1-nano-2025-04-14", 0)
	if err != nil {
		t.Fatalf("new llm client: %v", err)
	}
	resp, err := client.Extract(context.Background(), ExtractRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(resp.Facts) != 1 || resp.Facts[0] != "hello" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}
