package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"travel-agent/internal/database"
	"travel-agent/internal/llm"
	"travel-agent/internal/tools"
)

type fakeClient struct {
	responses []llm.Response
	requests  [][]llm.Message
	toolSpecs [][]llm.ToolDefinition
}

func (f *fakeClient) Chat(_ context.Context, messages []llm.Message, toolSpecs []llm.ToolDefinition) (llm.Response, error) {
	f.requests = append(f.requests, append([]llm.Message(nil), messages...))
	f.toolSpecs = append(f.toolSpecs, toolSpecs)
	response := f.responses[0]
	f.responses = f.responses[1:]
	return response, nil
}

func TestAgentExecutesMultipleToolCallsAndReturnsFinalAnswer(t *testing.T) {
	db, err := database.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	client := &fakeClient{responses: []llm.Response{
		{ToolCalls: []llm.ToolCall{
			{ID: "call-weather", Type: "function", Function: llm.FunctionCall{
				Name: "get_weather", Arguments: `{"city":"Казань","date":"2026-11-07"}`,
			}},
			{ID: "call-transport", Type: "function", Function: llm.FunctionCall{
				Name: "search_transport", Arguments: `{"from":"Москва","to":"Казань","departure_date":"2026-11-07","passengers":2}`,
			}},
		}},
		{Content: "Нашел транспорт и прогноз для поездки."},
	}}
	assistant := New(client, []tools.Tool{
		tools.NewWeather(),
		tools.NewTransportSearch(db),
	})

	answer, err := assistant.Run(context.Background(), "Поездка из Москвы в Казань")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if answer != "Нашел транспорт и прогноз для поездки." {
		t.Fatalf("answer = %q", answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("model called %d times, want 2", len(client.requests))
	}

	lastMessages := client.requests[1]
	toolResults := 0
	for _, message := range lastMessages {
		if message.Role == "tool" && message.ToolCallID != "" && json.Valid([]byte(message.Content)) {
			toolResults++
		}
	}
	if toolResults != 2 {
		t.Fatalf("tool results = %d, want 2", toolResults)
	}
	if len(client.toolSpecs[0]) != 2 {
		t.Fatalf("tool definitions = %d, want 2", len(client.toolSpecs[0]))
	}
}

func TestAgentPreservesConversationHistory(t *testing.T) {
	client := &fakeClient{responses: []llm.Response{
		{Content: "Первый ответ."},
		{Content: "Уточненный ответ."},
	}}
	assistant := New(client, nil)

	if _, err := assistant.Run(context.Background(), "Найди отель"); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if _, err := assistant.Run(context.Background(), "А можно дешевле?"); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	secondRequest := client.requests[1]
	if len(secondRequest) != 4 {
		t.Fatalf("second request has %d messages, want system + two user/assistant pairs", len(secondRequest))
	}
	if secondRequest[3].Role != "user" || secondRequest[3].Content != "А можно дешевле?" {
		t.Fatalf("second turn missing from history: %+v", secondRequest[3])
	}
}

func TestAgentRequestsRussianResponses(t *testing.T) {
	client := &fakeClient{responses: []llm.Response{{Content: "Ответ."}}}
	assistant := New(client, nil)

	if _, err := assistant.Run(context.Background(), "Привет"); err != nil {
		t.Fatal(err)
	}

	firstMessage := client.requests[0][0]
	if firstMessage.Role != "system" || !strings.Contains(firstMessage.Content, "только на русском языке") {
		t.Fatalf("system prompt does not require Russian: %q", firstMessage.Content)
	}
}

func TestAgentRequestsTerminalFriendlyFormatting(t *testing.T) {
	client := &fakeClient{responses: []llm.Response{{Content: "Уточните даты поездки."}}}
	assistant := New(client, nil)

	if _, err := assistant.Run(context.Background(), "Хочу поехать на выходные"); err != nil {
		t.Fatal(err)
	}

	prompt := client.requests[0][0].Content
	for _, requirement := range []string{"Не используй Markdown-таблицы", "Если даты не указаны", "source=mock"} {
		if !strings.Contains(prompt, requirement) {
			t.Fatalf("system prompt does not contain %q", requirement)
		}
	}
}

func TestSystemPromptIncludesCurrentDateAndFlexibleDateRule(t *testing.T) {
	prompt := buildSystemPrompt(time.Date(2026, time.October, 18, 12, 0, 0, 0, time.UTC))
	for _, requirement := range []string{"Сегодня: 2026-10-18", "ближайшие будущие выходные", "Не спрашивай даты повторно"} {
		if !strings.Contains(prompt, requirement) {
			t.Fatalf("system prompt does not contain %q", requirement)
		}
	}
}

func TestAgentStopsAtIterationLimit(t *testing.T) {
	toolCall := llm.Response{ToolCalls: []llm.ToolCall{{
		ID: "call-weather",
		Function: llm.FunctionCall{
			Name: "get_weather", Arguments: `{"city":"Казань","date":"2026-11-07"}`,
		},
	}}}
	responses := make([]llm.Response, maxIterations)
	for i := range responses {
		responses[i] = toolCall
	}
	client := &fakeClient{responses: responses}
	assistant := New(client, []tools.Tool{tools.NewWeather()})

	_, err := assistant.Run(context.Background(), "Погода?")
	if err == nil || !strings.Contains(err.Error(), "maximum") {
		t.Fatalf("Run() error = %v, want iteration-limit error", err)
	}
	if len(client.requests) != maxIterations {
		t.Fatalf("model called %d times, want %d", len(client.requests), maxIterations)
	}
}
