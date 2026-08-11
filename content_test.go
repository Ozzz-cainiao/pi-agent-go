package agent

import (
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
