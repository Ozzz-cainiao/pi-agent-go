package agent

import (
	"context"
	"reflect"
	"testing"
)

func TestRunAgentLoop_appliesTransformBeforeConvertWithoutMutatingInput(t *testing.T) {
	originalMessage := UserMessage{Content: []UserContent{TextContent{Text: "原始"}}}
	initial := AgentContext{Messages: []AgentMessage{originalMessage}}
	var order []string

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		initial,
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				text := userTextAt(t, modelContext.Messages, 0)
				if text.Text != "已转换" {
					t.Fatalf("Model text = %q, want transformed value", text.Text)
				}
				return AssistantMessage{StopReason: StopReasonStop}, nil
			},
			TransformContext: func(_ context.Context, messages []AgentMessage) ([]AgentMessage, error) {
				order = append(order, "transform")
				user := userMessageAt(t, messages, 0)
				user.Content[0] = TextContent{Text: "已转换"}
				messages[0] = user
				return messages, nil
			},
			ConvertToLLM: func(_ context.Context, messages []AgentMessage) ([]Message, error) {
				order = append(order, "convert")
				return []Message{userMessageAt(t, messages, 0)}, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(order, []string{"transform", "convert"}) {
		t.Fatalf("hook order = %#v, want transform then convert", order)
	}
	if !reflect.DeepEqual(initial.Messages, []AgentMessage{originalMessage}) {
		t.Fatal("context hooks modified input context")
	}
}

func TestRunAgentLoop_usesPrepareNextTurnSnapshot(t *testing.T) {
	prompt := UserMessage{Content: []UserContent{TextContent{Text: "原始"}}}
	toolRequest := AssistantMessage{
		Content:    []AssistantContent{ToolCall{ID: "call-1", Name: "qa_search"}},
		StopReason: StopReasonToolUse,
	}
	final := AssistantMessage{StopReason: StopReasonStop}
	modelCall := 0
	prepareCall := 0

	_, err := RunAgentLoop(
		context.Background(),
		[]AgentMessage{prompt},
		AgentContext{SystemPrompt: "first", Tools: []Tool{stubTool{}}},
		LoopConfig{
			Stream: func(_ context.Context, modelContext AgentContext, _ AssistantMessageEventSink) (AssistantMessage, error) {
				modelCall++
				if modelCall == 1 {
					return toolRequest, nil
				}
				if modelContext.SystemPrompt != "second" {
					t.Fatalf("second SystemPrompt = %q, want second", modelContext.SystemPrompt)
				}
				text := userTextAt(t, modelContext.Messages, 0)
				if text.Text != "原始" {
					t.Fatalf("second prompt text = %q, snapshot mutation leaked", text.Text)
				}
				return final, nil
			},
			PrepareNextTurn: func(_ context.Context, turn PrepareNextTurnContext) (NextTurnUpdate, error) {
				prepareCall++
				if prepareCall > 1 {
					return NextTurnUpdate{}, nil
				}
				user := userMessageAt(t, turn.Context.Messages, 0)
				user.Content[0] = TextContent{Text: "篡改快照"}
				turn.Context.Messages[0] = user
				next := AgentContext{
					SystemPrompt: "second",
					Messages:     append([]AgentMessage(nil), turn.NewMessages...),
					Tools:        turn.Context.Tools,
				}
				next.Messages[0] = prompt
				return NextTurnUpdate{Context: &next}, nil
			},
		},
		nil,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if modelCall != 2 || prepareCall != 2 {
		t.Fatalf("calls = model:%d prepare:%d, want 2/2", modelCall, prepareCall)
	}
	text, ok := prompt.Content[0].(TextContent)
	if !ok {
		t.Fatalf("input content type = %T, want TextContent", prompt.Content[0])
	}
	if text.Text != "原始" {
		t.Fatalf("input prompt = %q, want unchanged", text.Text)
	}
}

func userMessageAt(t *testing.T, messages []AgentMessage, index int) UserMessage {
	t.Helper()
	message, ok := messages[index].(UserMessage)
	if !ok {
		t.Fatalf("message[%d] type = %T, want UserMessage", index, messages[index])
	}
	return message
}

func userTextAt(t *testing.T, messages []AgentMessage, index int) TextContent {
	t.Helper()
	message := userMessageAt(t, messages, index)
	content, ok := message.Content[0].(TextContent)
	if !ok {
		t.Fatalf("message[%d] content type = %T, want TextContent", index, message.Content[0])
	}
	return content
}
