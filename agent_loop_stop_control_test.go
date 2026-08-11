package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type countingTool struct {
	calls int
}

func (tool *countingTool) Definition() ToolDefinition {
	return ToolDefinition{Name: "qa_search"}
}

func (tool *countingTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	tool.calls++

	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: "不应该执行"},
		},
	}, nil
}

func TestRunAgentLoop_terminalStopReasonDoesNotExecuteTool(t *testing.T) {
	reasons := []StopReason{
		StopReasonError,
		StopReasonAborted,
	}

	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			// 准备
			tool := &countingTool{}
			modelCalls := 0

			response := AssistantMessage{
				Content: []AssistantContent{
					ToolCall{
						ID:   "call-1",
						Name: "qa_search",
					},
				},
				StopReason: reason,
			}

			streamFn := StreamFunc(func(
				_ context.Context,
				_ AgentContext,
				_ AssistantMessageEventSink,
			) (AssistantMessage, error) {
				modelCalls++
				if modelCalls > 1 {
					return AssistantMessage{}, errors.New("unexpected Model call")
				}

				return response, nil
			})

			// 执行
			got, err := RunAgentLoop(
				context.Background(),
				nil,
				AgentContext{Tools: []Tool{tool}},
				LoopConfig{Stream: streamFn},
				nil,
			)
			// 验证
			if err != nil {
				t.Fatalf("RunAgentLoop() returned error: %v", err)
			}
			if modelCalls != 1 {
				t.Fatalf("Model call count = %d, want 1", modelCalls)
			}
			if tool.calls != 0 {
				t.Fatalf("Tool call count = %d, want 0", tool.calls)
			}
			if len(got) != 1 {
				t.Fatalf("new message count = %d, want 1", len(got))
			}
		})
	}
}

func TestRunAgentLoop_doesNotExecuteTruncatedToolCall(t *testing.T) {
	// 准备
	tool := &countingTool{}
	modelCalls := 0

	truncatedResponse := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "可能被截断的参数",
				},
			},
		},
		StopReason: StopReasonLength,
	}

	finalResponse := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "工具参数被截断，请重试。"},
		},
		StopReason: StopReasonStop,
	}

	streamFn := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++

		switch modelCalls {
		case 1:
			return truncatedResponse, nil
		case 2:
			return finalResponse, nil
		default:
			return AssistantMessage{}, errors.New("unexpected Model call")
		}
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{Tools: []Tool{tool}},
		LoopConfig{Stream: streamFn},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if tool.calls != 0 {
		t.Fatalf("Tool call count = %d, want 0", tool.calls)
	}
	if modelCalls != 2 {
		t.Fatalf("Model call count = %d, want 2", modelCalls)
	}
	if len(got) != 3 {
		t.Fatalf("new message count = %d, want 3", len(got))
	}

	result, ok := got[1].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[1] type = %T, want ToolResultMessage", got[1])
	}
	if !result.IsError {
		t.Fatal("truncated ToolResultMessage IsError = false, want true")
	}

	wantContent := []ToolResultContent{
		TextContent{
			Text: `Tool call "qa_search" was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.`,
		},
	}
	if !reflect.DeepEqual(result.Content, wantContent) {
		t.Fatalf("content = %#v, want %#v", result.Content, wantContent)
	}
}
