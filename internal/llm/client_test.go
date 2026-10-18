package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestClientSendsToolsAndParsesToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("request path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var request struct {
			Model    string           `json:"model"`
			Messages []Message        `json:"messages"`
			Tools    []ToolDefinition `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if request.Model != "qwen-test" || len(request.Tools) != 1 {
			t.Errorf("request model/tools = %q/%d", request.Model, len(request.Tools))
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"tool_calls":[{"id":"call-1","type":"function","function":{"name":"get_weather","arguments":"{\"city\":\"Казань\",\"date\":\"2026-11-07\"}"}}]}}]}`)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "qwen-test"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	response, err := client.Chat(context.Background(), []Message{{Role: "user", Content: "Погода?"}}, []ToolDefinition{{
		Type:     "function",
		Function: FunctionSchema{Name: "get_weather", Parameters: json.RawMessage(`{"type":"object"}`)},
	}})
	if err != nil {
		t.Fatalf("Chat() error = %v", err)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0].Function.Name != "get_weather" {
		t.Fatalf("response = %+v", response)
	}
}

func TestClientReportsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, APIKey: "key", Model: "model"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.Chat(context.Background(), []Message{{Role: "user", Content: "hello"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("Chat() error = %v, want HTTP status error", err)
	}
}

func TestNewClientRejectsExampleBaseURL(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL: "https://your-openai-compatible-endpoint/v1",
		APIKey:  "test-key",
		Model:   "qwen-test",
	})
	if err == nil || !strings.Contains(err.Error(), ".env.example placeholder") {
		t.Fatalf("NewClient() error = %v, want placeholder configuration error", err)
	}
}

func TestNewClientUsesConfiguredTimeout(t *testing.T) {
	client, err := NewClient(Config{
		BaseURL:       "https://example.com/v1",
		APIKey:        "key",
		Model:         "model",
		TimeoutString: "2m",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client.client.Timeout != 2*time.Minute {
		t.Fatalf("timeout = %s, want 2m", client.client.Timeout)
	}
}

func TestNewClientRejectsInvalidTimeout(t *testing.T) {
	_, err := NewClient(Config{
		BaseURL:       "https://example.com/v1",
		APIKey:        "key",
		Model:         "model",
		TimeoutString: "later",
	})
	if err == nil || !strings.Contains(err.Error(), "LLM_TIMEOUT") {
		t.Fatalf("NewClient() error = %v, want LLM_TIMEOUT error", err)
	}
}
