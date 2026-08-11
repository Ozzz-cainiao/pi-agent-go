package agent

import (
	"context"
	"testing"
)

func TestNewAgent_usesDefaultState(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "" || state.Model != UnknownModelID || state.ThinkingLevel != ThinkingLevelOff {
		t.Fatalf("default identity state = %#v", state)
	}
	if len(state.Tools) != 0 || len(state.Messages) != 0 || len(state.PendingToolCalls) != 0 {
		t.Fatalf("default collections = %#v", state)
	}
	if state.IsStreaming || state.StreamingMessage != nil || state.ErrorMessage != "" {
		t.Fatalf("default runtime state = %#v", state)
	}
}

func TestNewAgent_copiesCustomInitialState(t *testing.T) {
	arguments := map[string]any{"filters": map[string]any{"scope": "docs"}}
	messages := []AgentMessage{AssistantMessage{
		Content: []AssistantContent{ToolCall{ID: "call-1", Name: "search", Arguments: arguments}},
	}}
	tools := []Tool{stateNamedTool{name: "search"}}
	initial := AgentInitialState{
		SystemPrompt:  "系统提示词",
		Model:         ModelID("demo-model"),
		ThinkingLevel: ThinkingLevelHigh,
		Tools:         tools,
		Messages:      messages,
	}

	agent, err := NewAgent(AgentOptions{InitialState: &initial})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	filters, ok := arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", arguments["filters"])
	}
	filters["scope"] = "changed"
	tools[0] = stateNamedTool{name: "changed"}
	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "系统提示词" || state.Model != "demo-model" || state.ThinkingLevel != ThinkingLevelHigh {
		t.Fatalf("custom identity state = %#v", state)
	}
	if state.Tools[0].Definition().Name != "search" || len(state.PendingToolCalls) != 0 {
		t.Fatalf("custom collections = %#v", state)
	}
	if got := toolArgumentScope(t, state.Messages); got != "docs" {
		t.Fatalf("copied argument scope = %q, want docs", got)
	}
	if state.IsStreaming || state.StreamingMessage != nil || state.ErrorMessage != "" {
		t.Fatalf("custom runtime state = %#v", state)
	}
}

func TestAgent_stateMutators(t *testing.T) {
	agent, err := NewAgent(AgentOptions{})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	tools := []Tool{stateNamedTool{name: "search"}}
	messages := []AgentMessage{UserMessage{Content: []UserContent{TextContent{Text: "你好"}}}}
	operations := []func() error{
		func() error { return agent.SetSystemPrompt("新提示词") },
		func() error { return agent.SetModel("stable-model") },
		func() error { return agent.SetThinkingLevel(ThinkingLevelMedium) },
		func() error { return agent.SetTools(tools) },
		func() error { return agent.ReplaceMessages(messages) },
		func() error { return agent.AppendMessage(AssistantMessage{StopReason: StopReasonStop}) },
	}
	for index, operation := range operations {
		if err := operation(); err != nil {
			t.Fatalf("operation[%d] returned error: %v", index, err)
		}
	}
	tools[0] = stateNamedTool{name: "changed"}
	messages[0] = CustomMessage{Kind: "changed"}

	state, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if state.SystemPrompt != "新提示词" || state.Model != "stable-model" || state.ThinkingLevel != ThinkingLevelMedium {
		t.Fatalf("mutated identity state = %#v", state)
	}
	if state.Tools[0].Definition().Name != "search" || len(state.Messages) != 2 {
		t.Fatalf("mutated collections = %#v", state)
	}
	if err := agent.ClearMessages(); err != nil {
		t.Fatalf("ClearMessages() returned error: %v", err)
	}
	cleared, err := agent.State()
	if err != nil || len(cleared.Messages) != 0 {
		t.Fatalf("cleared state/error = %#v/%v", cleared, err)
	}
}

func TestAgent_StateReturnsDefensiveSnapshot(t *testing.T) {
	message := AssistantMessage{Content: []AssistantContent{
		ToolCall{ID: "call-1", Name: "search", Arguments: map[string]any{"filters": map[string]any{"scope": "docs"}}},
	}}
	agent, err := NewAgent(AgentOptions{InitialState: &AgentInitialState{
		Tools:    []Tool{stateNamedTool{name: "search"}},
		Messages: []AgentMessage{message},
	}})
	if err != nil {
		t.Fatalf("NewAgent() returned error: %v", err)
	}
	t.Cleanup(func() { closeAgent(t, agent) })

	first, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	first.Tools[0] = stateNamedTool{name: "changed"}
	first.PendingToolCalls["external"] = struct{}{}
	mutateToolArgumentScope(t, first.Messages, "changed")

	second, err := agent.State()
	if err != nil {
		t.Fatalf("State() returned error: %v", err)
	}
	if second.Tools[0].Definition().Name != "search" || len(second.PendingToolCalls) != 0 {
		t.Fatalf("snapshot mutation leaked into collections: %#v", second)
	}
	if got := toolArgumentScope(t, second.Messages); got != "docs" {
		t.Fatalf("snapshot mutation leaked into arguments: %q", got)
	}
}

type stateNamedTool struct{ name string }

func (tool stateNamedTool) Definition() ToolDefinition { return ToolDefinition{Name: tool.name} }

func (stateNamedTool) Execute(context.Context, ToolCall, ToolUpdateFunc) (ToolResult, error) {
	return ToolResult{}, nil
}

func closeAgent(t *testing.T, agent *Agent) {
	t.Helper()
	if err := agent.Close(); err != nil {
		t.Errorf("Close() returned error: %v", err)
	}
}

func toolArgumentScope(t *testing.T, messages []AgentMessage) string {
	t.Helper()
	call := stateToolCall(t, messages)
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	scope, ok := filters["scope"].(string)
	if !ok {
		t.Fatalf("scope type = %T, want string", filters["scope"])
	}
	return scope
}

func mutateToolArgumentScope(t *testing.T, messages []AgentMessage, scope string) {
	t.Helper()
	call := stateToolCall(t, messages)
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	filters["scope"] = scope
}

func stateToolCall(t *testing.T, messages []AgentMessage) ToolCall {
	t.Helper()
	assistant, ok := messages[0].(AssistantMessage)
	if !ok {
		t.Fatalf("message type = %T, want AssistantMessage", messages[0])
	}
	call, ok := assistant.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("content type = %T, want ToolCall", assistant.Content[0])
	}
	return call
}
