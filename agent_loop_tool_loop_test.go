package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestRunAgentLoop_executesToolCallAndContinuesModel(t *testing.T) {
	// 准备
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "查询 Agent Loop"},
		},
		Timestamp: 1,
	}

	toolRequest := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "Agent Loop",
				},
			},
		},
		StopReason: StopReasonToolUse,
		Timestamp:  2,
	}

	finalResponse := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "Agent Loop 负责协调模型和工具。"},
		},
		StopReason: StopReasonStop,
		Timestamp:  4,
	}

	modelCalls := 0
	var secondContext AgentContext

	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		modelCalls++

		switch modelCalls {
		case 1:
			return toolRequest, nil
		case 2:
			secondContext = agentContext

			return finalResponse, nil
		default:
			return AssistantMessage{}, errors.New("unexpected Model call")
		}
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{
			Tools: []Tool{stubTool{}},
		},
		LoopConfig{Stream: streamFn},
		nil,
	)
	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCalls != 2 {
		t.Fatalf("Model call count = %d, want 2", modelCalls)
	}
	if len(secondContext.Messages) != 3 {
		t.Fatalf(
			"second Model context message count = %d, want 3",
			len(secondContext.Messages),
		)
	}
	if len(got) != 4 {
		t.Fatalf("new message count = %d, want 4", len(got))
	}

	toolResult, ok := got[2].(ToolResultMessage)
	if !ok {
		t.Fatalf("message[2] type = %T, want ToolResultMessage", got[2])
	}
	if toolResult.ToolCallID != "call-1" {
		t.Fatalf(
			"ToolCallID = %q, want %q",
			toolResult.ToolCallID,
			"call-1",
		)
	}
	if toolResult.IsError {
		t.Fatal("tool result IsError = true, want false")
	}
	if toolResult.Timestamp <= 0 {
		t.Fatalf("tool result Timestamp = %d, want positive value", toolResult.Timestamp)
	}
	if !reflect.DeepEqual(got[3], finalResponse) {
		t.Fatalf("final message = %#v, want %#v", got[3], finalResponse)
	}
}
