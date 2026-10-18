package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type Config struct {
	BaseURL       string
	APIKey        string
	Model         string
	TimeoutString string
}

func ConfigFromEnv() Config {
	return Config{
		BaseURL:       os.Getenv("LLM_BASE_URL"),
		APIKey:        os.Getenv("LLM_API_KEY"),
		Model:         os.Getenv("LLM_MODEL"),
		TimeoutString: os.Getenv("LLM_TIMEOUT"),
	}
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string         `json:"type"`
	Function FunctionSchema `json:"function"`
}

type FunctionSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

type Response struct {
	Content   string
	ToolCalls []ToolCall
}

type Client interface {
	Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (Response, error)
}

type HTTPClient struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

func NewClient(config Config) (*HTTPClient, error) {
	if strings.TrimSpace(config.BaseURL) == "" {
		return nil, fmt.Errorf("LLM_BASE_URL is required")
	}
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("LLM_API_KEY is required")
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("LLM_MODEL is required")
	}

	baseURL, err := url.Parse(strings.TrimSpace(config.BaseURL))
	if err != nil || baseURL.Scheme == "" || baseURL.Host == "" {
		return nil, fmt.Errorf("invalid LLM_BASE_URL %q", config.BaseURL)
	}
	if strings.EqualFold(baseURL.Hostname(), "your-openai-compatible-endpoint") {
		return nil, fmt.Errorf("LLM_BASE_URL still uses the .env.example placeholder; set it to the base URL provided by your OpenAI-compatible API")
	}
	baseURL.Path = strings.TrimRight(baseURL.Path, "/") + "/chat/completions"
	timeout := 180 * time.Second
	if strings.TrimSpace(config.TimeoutString) != "" {
		timeout, err = time.ParseDuration(strings.TrimSpace(config.TimeoutString))
		if err != nil || timeout <= 0 {
			return nil, fmt.Errorf("invalid LLM_TIMEOUT %q: use a positive duration such as 180s", config.TimeoutString)
		}
	}

	return &HTTPClient{
		endpoint: baseURL.String(),
		apiKey:   config.APIKey,
		model:    config.Model,
		client:   &http.Client{Timeout: timeout},
	}, nil
}

type chatRequest struct {
	Model    string           `json:"model"`
	Messages []Message        `json:"messages"`
	Tools    []ToolDefinition `json:"tools,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message Message `json:"message"`
	} `json:"choices"`
}

func (c *HTTPClient) Chat(ctx context.Context, messages []Message, tools []ToolDefinition) (Response, error) {
	payload, err := json.Marshal(chatRequest{
		Model:    c.model,
		Messages: messages,
		Tools:    tools,
	})
	if err != nil {
		return Response{}, fmt.Errorf("encode chat request: %w", err)
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return Response{}, fmt.Errorf("create chat request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+c.apiKey)

	httpResponse, err := c.client.Do(request)
	if err != nil {
		return Response{}, fmt.Errorf("send chat request: %w", err)
	}
	defer httpResponse.Body.Close()

	body, err := io.ReadAll(io.LimitReader(httpResponse.Body, 4<<20))
	if err != nil {
		return Response{}, fmt.Errorf("read chat response: %w", err)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return Response{}, fmt.Errorf("chat API returned HTTP %d: %s", httpResponse.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded chatResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return Response{}, fmt.Errorf("decode chat response: %w", err)
	}
	if len(decoded.Choices) == 0 {
		return Response{}, fmt.Errorf("chat API response contains no choices")
	}

	message := decoded.Choices[0].Message
	return Response{Content: message.Content, ToolCalls: message.ToolCalls}, nil
}
