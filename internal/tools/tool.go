package tools

import (
	"context"
	"encoding/json"

	"travel-agent/internal/llm"
)

type Tool interface {
	Name() string
	Description() string
	InputSchema() json.RawMessage
	Execute(ctx context.Context, arguments json.RawMessage) (any, error)
}

func Definitions(available []Tool) []llm.ToolDefinition {
	definitions := make([]llm.ToolDefinition, 0, len(available))
	for _, tool := range available {
		definitions = append(definitions, llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionSchema{
				Name:        tool.Name(),
				Description: tool.Description(),
				Parameters:  tool.InputSchema(),
			},
		})
	}
	return definitions
}
