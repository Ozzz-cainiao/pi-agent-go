package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
	"github.com/Ozzz-cainiao/pi-agent-go/agenttest"
)

type eventRecorder struct {
	mu     sync.Mutex
	events []string
}

func (recorder *eventRecorder) sink(event agent.AgentEvent) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.events = append(recorder.events, string(event.Kind()))
	return nil
}

func (recorder *eventRecorder) subscriber(_ context.Context, event agent.AgentEvent) error {
	return recorder.sink(event)
}

func (recorder *eventRecorder) snapshot() []string {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	return append([]string(nil), recorder.events...)
}

func streamFromFixture(fixture Fixture) (*agenttest.ScriptedStream, error) {
	steps := make([]agenttest.StreamStep, len(fixture.Script))
	for index, record := range fixture.Script {
		response, err := responseFromRecord(record)
		if err != nil {
			return nil, fmt.Errorf("script[%d]: %w", index, err)
		}
		steps[index] = agenttest.StreamStep{Response: response, WaitForCancel: record.WaitForCancel}
	}
	return agenttest.NewScriptedStream(steps...), nil
}

func responseFromRecord(record ResponseRecord) (agent.AssistantMessage, error) {
	response := agent.AssistantMessage{}
	if record.StopReason != "" {
		stopReason, err := agent.ParseStopReason(record.StopReason)
		if err != nil {
			return agent.AssistantMessage{}, err
		}
		response.StopReason = stopReason
	}
	if record.Text != "" {
		response.Content = append(response.Content, agent.TextContent{Text: record.Text})
	}
	for _, call := range record.ToolCalls {
		response.Content = append(response.Content, agent.ToolCall{
			ID: call.ID, Name: call.Name, Arguments: cloneArguments(call.Arguments),
		})
	}
	return response, nil
}

func messagesFromRecords(records []MessageRecord) ([]agent.AgentMessage, error) {
	messages := make([]agent.AgentMessage, len(records))
	for index, record := range records {
		message, err := messageFromRecord(record)
		if err != nil {
			return nil, fmt.Errorf("message[%d]: %w", index, err)
		}
		messages[index] = message
	}
	return messages, nil
}

func messageFromRecord(record MessageRecord) (agent.AgentMessage, error) {
	switch record.Role {
	case "user":
		return agent.UserMessage{Content: []agent.UserContent{agent.TextContent{Text: record.Text}}}, nil
	case "assistant":
		content := make([]agent.AssistantContent, 0, len(record.ToolCalls)+1)
		if record.Text != "" {
			content = append(content, agent.TextContent{Text: record.Text})
		}
		for _, call := range record.ToolCalls {
			content = append(content, agent.ToolCall{ID: call.ID, Name: call.Name, Arguments: cloneArguments(call.Arguments)})
		}
		reason := agent.StopReasonStop
		if len(record.ToolCalls) > 0 {
			reason = agent.StopReasonToolUse
		}
		return agent.AssistantMessage{Content: content, StopReason: reason}, nil
	case "tool":
		return agent.ToolResultMessage{
			ToolCallID: record.ToolCallID, ToolName: record.ToolName,
			Content: []agent.ToolResultContent{agent.TextContent{Text: record.Text}}, IsError: record.IsError,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported role %q: %w", record.Role, ErrInvalidFixture)
	}
}

func toolsFromFixture(fixture Fixture, mode agent.ToolExecutionMode) []agent.Tool {
	seen := make(map[string]struct{})
	tools := make([]agent.Tool, 0)
	for _, response := range fixture.Script {
		for _, call := range response.ToolCalls {
			if _, exists := seen[call.Name]; exists {
				continue
			}
			seen[call.Name] = struct{}{}
			name := call.Name
			tools = append(tools, agenttest.NewScriptedTool(agent.ToolDefinition{
				Name: name, Parameters: json.RawMessage(`{"type":"object"}`), ExecutionMode: mode,
			}, func(context.Context, agent.ToolCall, agent.ToolUpdateFunc) (agent.ToolResult, error) {
				return textToolResult(fixture.Input.ToolResults[name]), nil
			}))
		}
	}
	return tools
}

func textToolResult(text string) agent.ToolResult {
	return agent.ToolResult{Content: []agent.ToolResultContent{agent.TextContent{Text: text}}}
}

func normalizeResult(events []string, messages []agent.AgentMessage) Result {
	records := make([]MessageRecord, 0, len(messages))
	stopReason := ""
	for _, message := range messages {
		record, reason, ok := normalizeMessage(message)
		if !ok {
			continue
		}
		records = append(records, record)
		if reason != "" {
			stopReason = reason
		}
	}
	return Result{Events: events, Messages: records, StopReason: stopReason}
}

func normalizeMessage(message agent.AgentMessage) (MessageRecord, string, bool) {
	switch typed := message.(type) {
	case agent.UserMessage:
		return MessageRecord{Role: "user", Text: userText(typed)}, "", true
	case agent.AssistantMessage:
		return normalizeAssistant(typed), string(typed.StopReason), true
	case agent.ToolResultMessage:
		return MessageRecord{
			Role: "tool", Text: toolText(typed), ToolName: typed.ToolName,
			ToolCallID: typed.ToolCallID, IsError: typed.IsError,
		}, "", true
	default:
		return MessageRecord{}, "", false
	}
}

func normalizeAssistant(message agent.AssistantMessage) MessageRecord {
	record := MessageRecord{Role: "assistant"}
	var text strings.Builder
	for _, content := range message.Content {
		switch typed := content.(type) {
		case agent.TextContent:
			text.WriteString(typed.Text)
		case agent.ToolCall:
			record.ToolCalls = append(record.ToolCalls, ToolCallRecord{
				ID: typed.ID, Name: typed.Name, Arguments: cloneArguments(typed.Arguments),
			})
		}
	}
	record.Text = text.String()
	return record
}

func userText(message agent.UserMessage) string {
	var text strings.Builder
	for _, content := range message.Content {
		if typed, ok := content.(agent.TextContent); ok {
			text.WriteString(typed.Text)
		}
	}
	return text.String()
}

func toolText(message agent.ToolResultMessage) string {
	var text strings.Builder
	for _, content := range message.Content {
		if typed, ok := content.(agent.TextContent); ok {
			text.WriteString(typed.Text)
		}
	}
	return text.String()
}

func cloneArguments(arguments map[string]any) map[string]any {
	if arguments == nil {
		return nil
	}
	cloned := make(map[string]any, len(arguments))
	for key, value := range arguments {
		cloned[key] = value
	}
	return cloned
}
