package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"
)

func TestRunAgentLoop_preservesAccumulatedDeltaSnapshots(t *testing.T) {
	var textSnapshots []string
	var thinkingSnapshots []string
	var toolArgumentSnapshots []map[string]any
	sink := AssistantMessageEventSink(func(event AssistantMessageEvent) error {
		switch value := event.(type) {
		case AssistantTextDeltaEvent:
			content := contentAt[TextContent](t, value.Partial, value.ContentIndex)
			textSnapshots = append(textSnapshots, content.Text)
		case AssistantThinkingDeltaEvent:
			content := contentAt[ThinkingContent](t, value.Partial, value.ContentIndex)
			thinkingSnapshots = append(thinkingSnapshots, content.Thinking)
		case AssistantToolCallDeltaEvent:
			content := contentAt[ToolCall](t, value.Partial, value.ContentIndex)
			toolArgumentSnapshots = append(toolArgumentSnapshots, content.Arguments)
		}

		return nil
	})

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: accumulatingAssistantStream()},
		sink,
	)
	if err != nil {
		t.Fatalf("RunAgentLoop() returned error: %v", err)
	}
	if !reflect.DeepEqual(textSnapshots, []string{"你", "你好"}) {
		t.Fatalf("text snapshots = %#v, want incremental text", textSnapshots)
	}
	if !reflect.DeepEqual(thinkingSnapshots, []string{"思", "思考"}) {
		t.Fatalf("thinking snapshots = %#v, want incremental thinking", thinkingSnapshots)
	}
	wantToolArguments := []map[string]any{
		{},
		{"query": "pi"},
	}
	if !reflect.DeepEqual(toolArgumentSnapshots, wantToolArguments) {
		t.Fatalf("tool argument snapshots = %#v, want %#v", toolArgumentSnapshots, wantToolArguments)
	}
}

func accumulatingAssistantStream() StreamFunc {
	return func(
		_ context.Context,
		_ AgentContext,
		emit AssistantMessageEventSink,
	) (AssistantMessage, error) {
		partial := AssistantMessage{StopReason: StopReasonPending}
		if err := emit(AssistantStartEvent{Partial: partial}); err != nil {
			return AssistantMessage{}, err
		}
		if err := emitTextDeltas(emit, &partial); err != nil {
			return AssistantMessage{}, err
		}
		if err := emitThinkingDeltas(emit, &partial); err != nil {
			return AssistantMessage{}, err
		}
		if err := emitToolArgumentDeltas(emit, &partial); err != nil {
			return AssistantMessage{}, err
		}

		return AssistantMessage{StopReason: StopReasonStop}, nil
	}
}

func emitTextDeltas(emit AssistantMessageEventSink, partial *AssistantMessage) error {
	partial.Content = append(partial.Content, TextContent{})
	if err := emit(AssistantTextStartEvent{ContentIndex: 0, Partial: *partial}); err != nil {
		return err
	}

	text := TextContent{}
	for _, delta := range []string{"你", "好"} {
		text.Text += delta
		partial.Content[0] = text
		if err := emit(AssistantTextDeltaEvent{ContentIndex: 0, Delta: delta, Partial: *partial}); err != nil {
			return err
		}
	}

	return nil
}

func emitThinkingDeltas(emit AssistantMessageEventSink, partial *AssistantMessage) error {
	partial.Content = append(partial.Content, ThinkingContent{})
	if err := emit(AssistantThinkingStartEvent{ContentIndex: 1, Partial: *partial}); err != nil {
		return err
	}

	thinking := ThinkingContent{}
	for _, delta := range []string{"思", "考"} {
		thinking.Thinking += delta
		partial.Content[1] = thinking
		if err := emit(AssistantThinkingDeltaEvent{ContentIndex: 1, Delta: delta, Partial: *partial}); err != nil {
			return err
		}
	}

	return nil
}

func emitToolArgumentDeltas(emit AssistantMessageEventSink, partial *AssistantMessage) error {
	call := ToolCall{ID: "call-1", Name: "search", Arguments: map[string]any{}}
	partial.Content = append(partial.Content, call)
	if err := emit(AssistantToolCallStartEvent{ContentIndex: 2, Partial: *partial}); err != nil {
		return err
	}

	var rawArguments string
	for _, delta := range []string{`{"query":"`, `pi"}`} {
		rawArguments += delta
		var arguments map[string]any
		if err := json.Unmarshal([]byte(rawArguments), &arguments); err == nil {
			call.Arguments = arguments
		}
		partial.Content[2] = call
		if err := emit(AssistantToolCallDeltaEvent{ContentIndex: 2, Delta: delta, Partial: *partial}); err != nil {
			return err
		}
	}

	return nil
}

func contentAt[T AssistantContent](t *testing.T, partial AssistantMessage, index int) T {
	t.Helper()

	content, ok := partial.Content[index].(T)
	if !ok {
		t.Fatalf("content[%d] type = %T", index, partial.Content[index])
	}

	return content
}
