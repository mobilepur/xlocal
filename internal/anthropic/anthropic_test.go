package anthropic

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func testClient(url string) *Client {
	c := New("sk-test", "claude-sonnet-5")
	c.BaseURL = url
	c.retryDelay = time.Millisecond
	return c
}

func TestCompleteSendsProperRequest(t *testing.T) {
	var gotHeader http.Header
	var gotBody map[string]interface{}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header
		if r.URL.Path != "/v1/messages" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Error(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "  Hallo Welt \n"}},
		})
	}))
	defer server.Close()

	got, err := testClient(server.URL).Complete(context.Background(), "translate this")
	if err != nil {
		t.Fatal(err)
	}
	if got != "Hallo Welt" {
		t.Errorf("Complete = %q, want trimmed %q", got, "Hallo Welt")
	}

	if gotHeader.Get("x-api-key") != "sk-test" {
		t.Error("x-api-key header missing")
	}
	if gotHeader.Get("anthropic-version") == "" {
		t.Error("anthropic-version header missing")
	}
	if gotBody["model"] != "claude-sonnet-5" {
		t.Errorf("model = %v", gotBody["model"])
	}
	messages := gotBody["messages"].([]interface{})
	first := messages[0].(map[string]interface{})
	if first["role"] != "user" || first["content"] != "translate this" {
		t.Errorf("message = %v", first)
	}
}

func TestCompleteRetriesOnServerErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": "ok"}},
		})
	}))
	defer server.Close()

	got, err := testClient(server.URL).Complete(context.Background(), "p")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ok" {
		t.Errorf("Complete = %q", got)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestCompleteDoesNotRetryClientErrors(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad prompt"}}`))
	}))
	defer server.Close()

	_, err := testClient(server.URL).Complete(context.Background(), "p")
	if err == nil {
		t.Fatal("expected error")
	}
	if calls.Load() != 1 {
		t.Errorf("client errors must not be retried, calls = %d", calls.Load())
	}
}

func TestCompleteGivesUpAfterMaxRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(529) // anthropic "overloaded"
	}))
	defer server.Close()

	if _, err := testClient(server.URL).Complete(context.Background(), "p"); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestCompleteEmptyContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"content": []interface{}{}})
	}))
	defer server.Close()

	if _, err := testClient(server.URL).Complete(context.Background(), "p"); err == nil {
		t.Fatal("expected error for empty content")
	}
}

func TestResolveModelPicksLatestMatchingAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/models" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{
				{"id": "claude-haiku-4-5", "created_at": "2025-10-01T00:00:00Z"},
				{"id": "claude-sonnet-4-5", "created_at": "2025-09-29T00:00:00Z"},
				{"id": "claude-sonnet-5", "created_at": "2026-03-01T00:00:00Z"},
			},
		})
	}))
	defer server.Close()

	got, err := testClient(server.URL).ResolveModel(context.Background(), "sonnet")
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude-sonnet-5" {
		t.Errorf("ResolveModel = %q, want claude-sonnet-5", got)
	}
}

func TestResolveModelPassesThroughFullIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("full model IDs must not trigger an API call")
	}))
	defer server.Close()

	got, err := testClient(server.URL).ResolveModel(context.Background(), "claude-opus-4-8")
	if err != nil {
		t.Fatal(err)
	}
	if got != "claude-opus-4-8" {
		t.Errorf("ResolveModel = %q", got)
	}
}

func TestResolveModelUnknownAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"id": "claude-sonnet-5", "created_at": "2026-03-01T00:00:00Z"}},
		})
	}))
	defer server.Close()

	if _, err := testClient(server.URL).ResolveModel(context.Background(), "gpt"); err == nil {
		t.Fatal("expected error for unknown alias")
	}
}
