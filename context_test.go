package agent

import (
	"reflect"
	"testing"
)

func TestAgentContext_WithMessages_returnsIndependentContext(t *testing.T) {
	// 准备
	originalMessage := UserMessage{
		Content: []UserContent{
			TextContent{Text: "第一个问题"},
		},
		Timestamp: 1,
	}
	addedMessage := UserMessage{
		Content: []UserContent{
			TextContent{Text: "第二个问题"},
		},
		Timestamp: 2,
	}

	original := AgentContext{
		SystemPrompt: "你是一个助手。",
		Messages:     []Message{originalMessage},
		Tools:        []Tool{stubTool{}},
	}

	// 执行
	got := original.WithMessages(addedMessage)

	// 验证：原始上下文没有被修改
	if len(original.Messages) != 1 {
		t.Fatalf("original message count = %d, want 1", len(original.Messages))
	}

	// 验证：新上下文包含原消息和新增消息
	if len(got.Messages) != 2 {
		t.Fatalf("new message count = %d, want 2", len(got.Messages))
	}

	// 验证：两个上下文不共享 slice 的底层数组
	got.Messages[0] = addedMessage
	if !reflect.DeepEqual(original.Messages[0], originalMessage) {
		t.Fatal("modifying new context changed original context")
	}

	// 验证：两个上下文不共享 Tools slice 的底层数组
	got.Tools[0] = nil
	if original.Tools[0] == nil {
		t.Fatal("modifying new context tools changed original context")
	}
}
