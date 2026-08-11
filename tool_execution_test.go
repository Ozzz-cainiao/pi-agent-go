package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestExecuteToolCall_returnsSuccessfulResultMessage(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-1",
		Name: "qa_search",
		Arguments: map[string]any{
			"query": "什么是 Agent Loop？",
		},
	}

	// 执行
	got := executeToolCall(
		context.Background(),
		[]Tool{stubTool{}},
		call,
		100,
		func(ToolResult) {},
		JSONSchemaArgumentValidator{},
	)

	// 验证
	want := ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "qa_search",
		Content: []ToolResultContent{
			TextContent{Text: "搜索结果"},
		},
		IsError:   false,
		Timestamp: 100,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("executeToolCall() = %#v, want %#v", got, want)
	}
}

func TestExecuteToolCall_returnsErrorForMissingTool(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-2",
		Name: "rag_search",
	}

	// 执行
	got := executeToolCall(
		context.Background(),
		[]Tool{stubTool{}},
		call,
		200,
		func(ToolResult) {},
		JSONSchemaArgumentValidator{},
	)

	// 验证
	if !got.IsError {
		t.Fatal("executeToolCall() IsError = false, want true")
	}
	if got.ToolCallID != "call-2" {
		t.Fatalf("ToolCallID = %q, want %q", got.ToolCallID, "call-2")
	}

	wantContent := []ToolResultContent{
		TextContent{Text: "Tool rag_search not found"},
	}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", got.Content, wantContent)
	}
}

var errToolBackend = errors.New("tool backend unavailable")

type failingTool struct{}

func (failingTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name: "failing_tool",
	}
}

func (failingTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	return ToolResult{}, errToolBackend
}

func TestExecuteToolCall_convertsToolErrorToResultMessage(t *testing.T) {
	// 准备
	call := ToolCall{
		ID:   "call-3",
		Name: "failing_tool",
	}

	// 执行
	got := executeToolCall(
		context.Background(),
		[]Tool{failingTool{}},
		call,
		300,
		func(ToolResult) {},
		JSONSchemaArgumentValidator{},
	)

	// 验证
	if !got.IsError {
		t.Fatal("executeToolCall() IsError = false, want true")
	}

	wantContent := []ToolResultContent{
		TextContent{Text: errToolBackend.Error()},
	}
	if !reflect.DeepEqual(got.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", got.Content, wantContent)
	}
}
