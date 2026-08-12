package agent

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestTextContent_satisfiesContent(t *testing.T) {
	// 准备
	want := TextContent{
		Text:          "你好",
		TextSignature: "signature-1",
	}

	// 执行
	var content Content = want
	got, ok := content.(TextContent)

	// 验证
	if !ok {
		t.Fatalf("Content type = %T, want TextContent", content)
	}
	if got != want {
		t.Fatalf("TextContent = %#v, want %#v", got, want)
	}
}

func TestThinkingContent_satisfiesContent(t *testing.T) {
	// 准备
	want := ThinkingContent{
		Thinking:          "正在分析问题",
		ThinkingSignature: "thinking-signature-1",
		Redacted:          true,
	}

	// 执行
	var content Content = want

	var got ThinkingContent
	switch value := content.(type) {
	case ThinkingContent:
		got = value
	default:
		t.Fatalf("Content type = %T, want ThinkingContent", content)
	}

	// 验证
	if got != want {
		t.Fatalf("ThinkingContent = %#v, want %#v", got, want)
	}
}

func TestImageContent_satisfiesContent(t *testing.T) {
	// 准备
	want := ImageContent{
		Data:     "iVBORw0KGgo=",
		MIMEType: "image/png",
	}

	// 执行
	var content Content = want
	got, ok := content.(ImageContent)

	// 验证
	if !ok {
		t.Fatalf("Content type = %T, want ImageContent", content)
	}
	if got != want {
		t.Fatalf("ImageContent = %#v, want %#v", got, want)
	}
}

func TestToolCall_satisfiesContent(t *testing.T) {
	// 准备
	want := ToolCall{
		ID:   "call-1",
		Name: "qa_search",
		Arguments: map[string]any{
			"query": "什么是 Agent Loop？",
			"limit": 5,
		},
		ThoughtSignature: "thought-signature-1",
		Namespace:        "qa",
	}

	// 执行
	var content Content = want
	got, ok := content.(ToolCall)

	// 验证
	if !ok {
		t.Fatalf("Content type = %T, want ToolCall", content)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ToolCall = %#v, want %#v", got, want)
	}
}

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
		Messages:     []AgentMessage{originalMessage},
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

func TestParseStopReason_accepts_known_reasons(t *testing.T) {
	// Given
	tests := []struct {
		raw  string
		want StopReason
	}{
		{raw: "pending", want: StopReasonPending},
		{raw: "stop", want: StopReasonStop},
		{raw: "length", want: StopReasonLength},
		{raw: "toolUse", want: StopReasonToolUse},
		{raw: "error", want: StopReasonError},
		{raw: "aborted", want: StopReasonAborted},
		{raw: "deferred", want: StopReasonDeferred},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			// When
			got, err := ParseStopReason(tt.raw)
			// Then
			if err != nil {
				t.Fatalf("ParseStopReason(%q) returned error: %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseStopReason(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseStopReason_rejects_unknown_reason(t *testing.T) {
	// Given
	raw := "banana"

	// When
	_, err := ParseStopReason(raw)

	// Then
	if !errors.Is(err, ErrInvalidStopReason) {
		t.Fatalf("ParseStopReason(%q) error = %v, want ErrInvalidStopReason", raw, err)
	}
}

func TestRunAgentLoop_convertsCustomMessagesBeforeStream(t *testing.T) {
	history := CustomMessage{
		Kind:    "notification",
		Payload: "系统维护已完成",
	}
	prompt := CustomMessage{
		Kind:    "voice_transcript",
		Payload: "你好",
	}
	converted := UserMessage{
		Content: []UserContent{
			TextContent{Text: "你好"},
		},
	}
	response := AssistantMessage{
		Content: []AssistantContent{
			TextContent{Text: "你好，需要什么帮助？"},
		},
		StopReason: StopReasonStop,
	}

	var hookInput []AgentMessage
	var streamMessages []AgentMessage
	config := LoopConfig{
		ConvertToLLM: func(
			_ context.Context,
			messages []AgentMessage,
		) ([]Message, error) {
			hookInput = append([]AgentMessage(nil), messages...)

			return []Message{converted}, nil
		},
		Stream: func(
			_ context.Context,
			context AgentContext,
			_ AssistantMessageEventSink,
		) (AssistantMessage, error) {
			streamMessages = append([]AgentMessage(nil), context.Messages...)

			return response, nil
		},
	}

	got, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{Messages: []AgentMessage{history}},
		config,
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(hookInput, []AgentMessage{history, prompt}) {
		t.Fatalf("ConvertToLLM input = %#v, want history and prompt", hookInput)
	}
	if !reflect.DeepEqual(streamMessages, []AgentMessage{converted}) {
		t.Fatalf("StreamFunc messages = %#v, want converted user message", streamMessages)
	}
	if !reflect.DeepEqual(got, []AgentMessage{prompt, response}) {
		t.Fatalf("RunAgentLoop() = %#v, want custom prompt and response", got)
	}
}
