package agent

import (
	"reflect"
	"testing"
)

func TestUserMessage_satisfiesMessage(t *testing.T) {
	// 准备
	want := UserMessage{
		Content: []UserContent{
			TextContent{Text: "描述这张图片"},
			ImageContent{
				Data:     "iVBORw0KGgo=",
				MIMEType: "image/png",
			},
		},
		Timestamp: 1_754_841_600_000,
	}

	// 执行
	var message Message = want
	got, ok := message.(UserMessage)

	// 验证
	if !ok {
		t.Fatalf("Message type = %T, want UserMessage", message)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("UserMessage = %#v, want %#v", got, want)
	}
}

func TestAssistantMessage_satisfiesMessage(t *testing.T) {
	// 准备
	want := AssistantMessage{
		Content: []AssistantContent{
			ThinkingContent{
				Thinking: "需要查询知识库",
			},
			ToolCall{
				ID:   "call-1",
				Name: "qa_search",
				Arguments: map[string]any{
					"query": "什么是 Agent Loop？",
				},
			},
		},
		API:      "responses",
		Provider: "openai",
		Model:    "example-model",
		Usage: Usage{
			InputTokens:  100,
			OutputTokens: 20,
			TotalTokens:  120,
		},
		StopReason: StopReasonToolUse,
		Timestamp:  1_754_841_600_000,
	}

	// 执行
	var message Message = want
	got, ok := message.(AssistantMessage)

	// 验证
	if !ok {
		t.Fatalf("Message type = %T, want AssistantMessage", message)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("AssistantMessage = %#v, want %#v", got, want)
	}
}

func TestToolResultMessage_satisfiesMessage(t *testing.T) {
	// 准备
	usage := &Usage{
		InputTokens: 5,
		TotalTokens: 5,
	}

	want := ToolResultMessage{
		ToolCallID: "call-1",
		ToolName:   "qa_search",
		Content: []ToolResultContent{
			TextContent{
				Text: "Agent Loop 负责协调模型调用和工具执行。",
			},
		},
		Details: map[string]any{
			"documentCount": 2,
		},
		Usage:          usage,
		AddedToolNames: []string{"document_search"},
		IsError:        false,
		Timestamp:      1_754_841_601_000,
	}

	// 执行
	var message Message = want
	got, ok := message.(ToolResultMessage)

	// 验证
	if !ok {
		t.Fatalf("Message type = %T, want ToolResultMessage", message)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToolResultMessage = %#v, want %#v", got, want)
	}
}
