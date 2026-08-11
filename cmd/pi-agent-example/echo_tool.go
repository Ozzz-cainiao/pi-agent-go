package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

var errInvalidEchoArguments = errors.New("pi-agent-example: invalid echo arguments")

type echoTool struct{}

func (echoTool) Definition() agent.ToolDefinition {
	return agent.ToolDefinition{
		Name:        "echo",
		Label:       "Echo",
		Description: "返回输入的 message，用于演示工具调用闭环",
		Parameters: json.RawMessage(
			`{"type":"object","additionalProperties":false,"required":["message"],"properties":{"message":{"type":"string"}}}`,
		),
	}
}

func (echoTool) Execute(
	_ context.Context,
	call agent.ToolCall,
	_ agent.ToolUpdateFunc,
) (agent.ToolResult, error) {
	message, ok := call.Arguments["message"].(string)
	if !ok {
		return agent.ToolResult{}, fmt.Errorf("echo message must be a string: %w", errInvalidEchoArguments)
	}
	return agent.ToolResult{
		Content: []agent.ToolResultContent{agent.TextContent{Text: message}},
	}, nil
}
