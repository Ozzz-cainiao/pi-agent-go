package agent

import (
	"context"
	"reflect"
	"testing"
)

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
