package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestRunAgentLoop_callsModelWithHistoryAndPrompt(t *testing.T) {
	// 准备
	history := UserMessage{
		Content: []UserContent{
			TextContent{Text: "历史问题"},
		},
		Timestamp: 1,
	}
	prompt := UserMessage{
		Content: []UserContent{
			TextContent{Text: "本轮问题"},
		},
		Timestamp: 2,
	}
	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "模型回答"},
		},
		StopReason: StopReasonStop,
		Timestamp:  3,
	}

	initial := AgentContext{
		SystemPrompt: "你是一个助手。",
		Messages:     []Message{history},
		Tools:        []Tool{stubTool{}},
	}

	var receivedContext AgentContext
	streamFn := StreamFunc(func(
		_ context.Context,
		agentContext AgentContext,
		_ AssistantMessageEventSink,
	) (AssistantMessage, error) {
		receivedContext = agentContext

		return response, nil
	})

	// 执行
	got, err := RunAgentLoop(
		context.Background(),
		[]Message{prompt},
		initial,
		streamFn,
	)

	// 验证
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if len(initial.Messages) != 1 {
		t.Fatalf("initial message count = %d, want 1", len(initial.Messages))
	}
	if len(receivedContext.Messages) != 2 {
		t.Fatalf(
			"model context message count = %d, want 2",
			len(receivedContext.Messages),
		)
	}

	want := []Message{prompt, response}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("RunAgentLoop() = %#v, want %#v", got, want)
	}
}
