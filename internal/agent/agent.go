package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"travel-agent/internal/llm"
	"travel-agent/internal/tools"
)

const maxIterations = 10

const systemPromptTemplate = `Ты AI-агент для планирования путешествий.

Сегодня: %s.

Правила ответа:
- Всегда отвечай только на русском языке. Не вставляй иностранные слова, если пользователь прямо об этом не попросил.
- Ответ отображается в обычном терминале. Не используй Markdown-таблицы, символы ** для жирного текста и HTML.
- Используй короткие абзацы и простые списки с дефисами.
- Не придумывай города, число гостей, бюджет и другие параметры.
- Если даты не указаны, сначала спроси их. Но если пользователь отвечает «без разницы», «любые», «выбери сам» или явно разрешает выбрать даты, самостоятельно возьми ближайшие будущие выходные относительно сегодняшней даты. Для поездки на выходные используй ближайшие пятницу — воскресенье, назови выбранные даты и продолжай поиск. Не спрашивай даты повторно.
- Никогда не предлагай даты в прошлом.
- Не выдумывай результаты инструментов и не называй тестовые данные актуальными.
- Результаты инструментов с source=mock или source=local_database являются демонстрационными. Явно предупреди об этом одной короткой строкой.
- Цены оформляй единообразно в рублях. Итоговые суммы вычисляй только из полученных данных.

Твоя задача — помочь пользователю подобрать поездку. Используй доступные инструменты, когда для ответа нужны данные об отелях, транспорте или погоде. После получения необходимых данных сформируй краткое и понятное предложение.`

type Agent struct {
	client      llm.Client
	tools       []tools.Tool
	toolsByName map[string]tools.Tool
	messages    []llm.Message
}

func New(client llm.Client, availableTools []tools.Tool) *Agent {
	byName := make(map[string]tools.Tool, len(availableTools))
	for _, tool := range availableTools {
		byName[tool.Name()] = tool
	}
	return &Agent{
		client:      client,
		tools:       availableTools,
		toolsByName: byName,
		messages:    []llm.Message{{Role: "system", Content: buildSystemPrompt(time.Now())}},
	}
}

func buildSystemPrompt(now time.Time) string {
	return fmt.Sprintf(systemPromptTemplate, now.Format("2006-01-02"))
}

func (a *Agent) Run(ctx context.Context, userInput string) (string, error) {
	a.messages = append(a.messages, llm.Message{Role: "user", Content: userInput})
	definitions := tools.Definitions(a.tools)

	for iteration := 0; iteration < maxIterations; iteration++ {
		requestStarted := time.Now()
		log.Printf("[agent] запрос к модели #%d...", iteration+1)
		response, err := a.client.Chat(ctx, a.messages, definitions)
		if err != nil {
			log.Printf("[agent] запрос к модели #%d завершился ошибкой через %s", iteration+1, elapsed(requestStarted))
			return "", fmt.Errorf("request model response: %w", err)
		}
		log.Printf("[agent] модель ответила на запрос #%d за %s; tool calls: %d", iteration+1, elapsed(requestStarted), len(response.ToolCalls))
		if len(response.ToolCalls) == 0 {
			if response.Content == "" {
				return "", fmt.Errorf("model returned neither a text response nor tool calls")
			}
			a.messages = append(a.messages, llm.Message{Role: "assistant", Content: response.Content})
			return response.Content, nil
		}

		a.messages = append(a.messages, llm.Message{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})
		for _, call := range response.ToolCalls {
			toolStarted := time.Now()
			log.Printf("[agent] запуск инструмента %s...", call.Function.Name)
			tool, ok := a.toolsByName[call.Function.Name]
			if !ok {
				return "", fmt.Errorf("model requested unknown tool %q", call.Function.Name)
			}
			var arguments json.RawMessage
			if err := json.Unmarshal([]byte(call.Function.Arguments), &arguments); err != nil {
				return "", fmt.Errorf("parse arguments for tool %q: %w", call.Function.Name, err)
			}
			result, err := tool.Execute(ctx, arguments)
			if err != nil {
				return "", fmt.Errorf("execute tool %q: %w", call.Function.Name, err)
			}
			log.Printf("[agent] инструмент %s завершён за %s", call.Function.Name, elapsed(toolStarted))
			resultJSON, err := json.Marshal(result)
			if err != nil {
				return "", fmt.Errorf("encode result from tool %q: %w", call.Function.Name, err)
			}
			a.messages = append(a.messages, llm.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Content:    string(resultJSON),
			})
		}
	}

	return "", fmt.Errorf("agent reached the maximum of %d model iterations", maxIterations)
}

func elapsed(started time.Time) time.Duration {
	return time.Since(started).Round(time.Millisecond)
}
