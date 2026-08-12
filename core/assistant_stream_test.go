package agent

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestAssistantMessageEventSink_forwardsEveryEventVariant(t *testing.T) {
	partial := AssistantMessage{StopReason: StopReasonPending}
	final := AssistantMessage{StopReason: StopReasonStop}
	toolCall := ToolCall{ID: "call-1", Name: "search"}
	want := []AssistantMessageEvent{
		AssistantStartEvent{Partial: partial},
		AssistantTextStartEvent{ContentIndex: 0, Partial: partial},
		AssistantTextDeltaEvent{ContentIndex: 0, Delta: "你", Partial: partial},
		AssistantTextEndEvent{ContentIndex: 0, Content: "你好", Partial: partial},
		AssistantThinkingStartEvent{ContentIndex: 1, Partial: partial},
		AssistantThinkingDeltaEvent{ContentIndex: 1, Delta: "分析", Partial: partial},
		AssistantThinkingEndEvent{ContentIndex: 1, Content: "分析完成", Partial: partial},
		AssistantToolCallStartEvent{ContentIndex: 2, Partial: partial},
		AssistantToolCallDeltaEvent{ContentIndex: 2, Delta: `{"query":"pi"`, Partial: partial},
		AssistantToolCallEndEvent{ContentIndex: 2, ToolCall: toolCall, Partial: partial},
		AssistantDoneEvent{Reason: StopReasonStop, Message: final},
		AssistantErrorEvent{
			Reason: StopReasonError,
			Error: AssistantMessage{
				StopReason:   StopReasonError,
				ErrorMessage: "provider failed",
			},
		},
	}

	got := make([]AssistantMessageEvent, 0, len(want))
	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		got = append(got, event)

		return nil
	})
	for _, event := range want {
		if err := emit(event); err != nil {
			t.Fatalf("emit(%T) returned error: %v", event, err)
		}
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %#v, want %#v", got, want)
	}
}

func TestRunAgentLoop_stopsImmediatelyWhenAssistantEventSinkFails(t *testing.T) {
	sinkError := errors.New("sink failed")
	emittedAfterFailure := false
	stream := StreamFunc(func(
		_ context.Context,
		_ AgentContext,
		emit AssistantMessageEventSink,
	) (AssistantMessage, error) {
		if err := emit(AssistantStartEvent{
			Partial: AssistantMessage{StopReason: StopReasonPending},
		}); err != nil {
			return AssistantMessage{}, err
		}

		emittedAfterFailure = true

		return AssistantMessage{StopReason: StopReasonStop}, nil
	})
	failingSinkCalls := 0
	sink := AgentEventSink(func(event AgentEvent) error {
		if event.Kind() != AgentEventMessageStart {
			return nil
		}
		failingSinkCalls++
		return sinkError
	})

	_, err := RunAgentLoop(
		context.Background(),
		nil,
		AgentContext{},
		LoopConfig{Stream: stream},
		sink,
	)

	if !errors.Is(err, sinkError) {
		t.Fatalf("RunAgentLoop() error = %v, want sink error", err)
	}
	if !errors.Is(err, ErrAgentEventSink) {
		t.Fatalf("RunAgentLoop() error = %v, want ErrAgentEventSink", err)
	}
	if emittedAfterFailure {
		t.Fatal("StreamFunc continued after AssistantMessageEventSink failure")
	}
	if failingSinkCalls != 1 {
		t.Fatalf("failing sink call count = %d, want 1", failingSinkCalls)
	}
}

func TestRunAgentLoop_preservesAccumulatedDeltaSnapshots(t *testing.T) {
	var textSnapshots []string
	var thinkingSnapshots []string
	var toolArgumentSnapshots []map[string]any
	sink := AgentEventSink(func(event AgentEvent) error {
		update, ok := event.(AgentMessageUpdateEvent)
		if !ok {
			return nil
		}
		switch value := update.AssistantEvent.(type) {
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

func TestAssistantMessageEventSink_clonesCommonMutableArgumentValues(t *testing.T) {
	tags := []string{"agent", "go"}
	labels := map[string]string{"language": "go"}
	payload := []byte{1, 2, 3}
	nested := []any{map[string]string{"scope": "core"}}
	partial := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				Arguments: map[string]any{
					"tags":    tags,
					"labels":  labels,
					"payload": payload,
					"nested":  nested,
				},
			},
		},
	}
	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		delta, ok := event.(AssistantToolCallDeltaEvent)
		if !ok {
			t.Fatalf("event type = %T, want AssistantToolCallDeltaEvent", event)
		}
		call := contentAt[ToolCall](t, delta.Partial, 0)
		argumentAs[[]string](t, call.Arguments, "tags")[0] = "changed"
		argumentAs[map[string]string](t, call.Arguments, "labels")["language"] = "typescript"
		argumentAs[[]byte](t, call.Arguments, "payload")[0] = 9
		nestedValues := argumentAs[[]any](t, call.Arguments, "nested")
		valueAs[map[string]string](t, nestedValues[0])["scope"] = "changed"

		return nil
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: partial})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	if tags[0] != "agent" {
		t.Fatalf("tags[0] = %q, want agent", tags[0])
	}
	if labels["language"] != "go" {
		t.Fatalf("labels[language] = %q, want go", labels["language"])
	}
	if payload[0] != 1 {
		t.Fatalf("payload[0] = %d, want 1", payload[0])
	}
	nestedMap := valueAs[map[string]string](t, nested[0])
	if nestedMap["scope"] != "core" {
		t.Fatalf("nested scope = %q, want core", nestedMap["scope"])
	}
}

func argumentAs[T any](t *testing.T, arguments map[string]any, name string) T {
	t.Helper()

	return valueAs[T](t, arguments[name])
}

func valueAs[T any](t *testing.T, value any) T {
	t.Helper()

	typed, ok := value.(T)
	if !ok {
		t.Fatalf("value type = %T", value)
	}

	return typed
}

func TestAssistantMessageEventSink_clonesPartialSnapshots(t *testing.T) {
	reasoningTokens := int64(7)
	arguments := map[string]any{
		"query": "pi",
		"filters": map[string]any{
			"language": "go",
		},
	}
	partial := AssistantMessage{
		Content: []AssistantContent{
			ToolCall{
				ID:        "call-1",
				Name:      "search",
				Arguments: arguments,
			},
		},
		Usage: Usage{ReasoningTokens: &reasoningTokens},
	}

	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		delta, ok := event.(AssistantToolCallDeltaEvent)
		if !ok {
			t.Fatalf("event type = %T, want AssistantToolCallDeltaEvent", event)
		}
		call, ok := delta.Partial.Content[0].(ToolCall)
		if !ok {
			t.Fatalf("partial content type = %T, want ToolCall", delta.Partial.Content[0])
		}
		call.Arguments["query"] = "changed"
		filters, ok := call.Arguments["filters"].(map[string]any)
		if !ok {
			t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
		}
		filters["language"] = "typescript"
		delta.Partial.Content[0] = TextContent{Text: "changed"}
		*delta.Partial.Usage.ReasoningTokens = 99

		return nil
	})

	err := emit(AssistantToolCallDeltaEvent{
		ContentIndex: 0,
		Delta:        `{"query":"pi"}`,
		Partial:      partial,
	})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	call, ok := partial.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("partial content type = %T, want ToolCall", partial.Content[0])
	}
	if call.Arguments["query"] != "pi" {
		t.Fatalf("partial query = %v, want pi", call.Arguments["query"])
	}
	filters, ok := call.Arguments["filters"].(map[string]any)
	if !ok {
		t.Fatalf("filters type = %T, want map[string]any", call.Arguments["filters"])
	}
	if filters["language"] != "go" {
		t.Fatalf("partial language = %v, want go", filters["language"])
	}
	if reasoningTokens != 7 {
		t.Fatalf("partial reasoning tokens = %d, want 7", reasoningTokens)
	}
}

func TestAssistantMessageEventSink_clonesTerminalMessagesAndToolCalls(t *testing.T) {
	toolCall := ToolCall{
		ID:   "call-1",
		Name: "search",
		Arguments: map[string]any{
			"query": "pi",
		},
	}
	message := AssistantMessage{
		Content: []AssistantContent{toolCall},
	}

	emit := assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		switch value := event.(type) {
		case AssistantToolCallEndEvent:
			value.ToolCall.Arguments["query"] = "changed tool call"
		case AssistantDoneEvent:
			call, ok := value.Message.Content[0].(ToolCall)
			if !ok {
				t.Fatalf("message content type = %T, want ToolCall", value.Message.Content[0])
			}
			call.Arguments["query"] = "changed message"
		}

		return nil
	})

	if err := emit(AssistantToolCallEndEvent{ToolCall: toolCall, Partial: message}); err != nil {
		t.Fatalf("emit tool call end returned error: %v", err)
	}
	if err := emit(AssistantDoneEvent{Reason: StopReasonStop, Message: message}); err != nil {
		t.Fatalf("emit done returned error: %v", err)
	}

	if toolCall.Arguments["query"] != "pi" {
		t.Fatalf("tool call query = %v, want pi", toolCall.Arguments["query"])
	}
	messageCall, ok := message.Content[0].(ToolCall)
	if !ok {
		t.Fatalf("message content type = %T, want ToolCall", message.Content[0])
	}
	if messageCall.Arguments["query"] != "pi" {
		t.Fatalf("message query = %v, want pi", messageCall.Arguments["query"])
	}
}

type mutableArgumentStruct struct {
	Tags   []string
	Labels map[string]string
}

type cyclicArgumentNode struct {
	Name string
	Next *cyclicArgumentNode
	Tags []string
}

type hiddenMutableArgument struct {
	values []string
}

func TestAssistantMessageEventSink_clonesMutableStructFields(t *testing.T) {
	argument := mutableArgumentStruct{
		Tags:   []string{"agent"},
		Labels: map[string]string{"language": "go"},
	}
	emit := toolCallSnapshotSink(t, func(arguments map[string]any) {
		cloned := valueAs[mutableArgumentStruct](t, arguments["config"])
		cloned.Tags[0] = "changed"
		cloned.Labels["language"] = "typescript"
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: assistantMessageWithArguments(map[string]any{
		"config": argument,
	})})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	if argument.Tags[0] != "agent" {
		t.Fatalf("argument Tags[0] = %q, want agent", argument.Tags[0])
	}
	if argument.Labels["language"] != "go" {
		t.Fatalf("argument Labels[language] = %q, want go", argument.Labels["language"])
	}
}

func TestAssistantMessageEventSink_preservesMapCyclesAndSharedTopology(t *testing.T) {
	shared := map[string]any{"value": "original"}
	graph := map[string]any{
		"first":  shared,
		"second": shared,
	}
	graph["self"] = graph
	emit := toolCallSnapshotSink(t, func(arguments map[string]any) {
		clonedSelf := valueAs[map[string]any](t, arguments["self"])
		if reflect.ValueOf(clonedSelf).Pointer() != reflect.ValueOf(arguments).Pointer() {
			t.Fatal("cloned self reference does not point to cloned root map")
		}
		first := valueAs[map[string]any](t, arguments["first"])
		second := valueAs[map[string]any](t, arguments["second"])
		first["value"] = "changed"
		if second["value"] != "changed" {
			t.Fatal("shared map topology was not preserved")
		}
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: assistantMessageWithArguments(graph)})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	if shared["value"] != "original" {
		t.Fatalf("source shared value = %v, want original", shared["value"])
	}
}

func TestAssistantMessageEventSink_preservesPointerCycles(t *testing.T) {
	node := &cyclicArgumentNode{Name: "root", Tags: []string{"agent"}}
	node.Next = node
	emit := toolCallSnapshotSink(t, func(arguments map[string]any) {
		cloned := valueAs[*cyclicArgumentNode](t, arguments["node"])
		if cloned.Next != cloned {
			t.Fatal("cloned pointer cycle does not point to cloned root node")
		}
		cloned.Tags[0] = "changed"
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: assistantMessageWithArguments(map[string]any{
		"node": node,
	})})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	if node.Tags[0] != "agent" {
		t.Fatalf("source node Tags[0] = %q, want agent", node.Tags[0])
	}
}

func TestAssistantMessageEventSink_preservesSliceCycles(t *testing.T) {
	cycle := make([]any, 1)
	cycle[0] = cycle
	emit := toolCallSnapshotSink(t, func(arguments map[string]any) {
		cloned := valueAs[[]any](t, arguments["cycle"])
		clonedSelf := valueAs[[]any](t, cloned[0])
		if reflect.ValueOf(clonedSelf).Pointer() != reflect.ValueOf(cloned).Pointer() {
			t.Fatal("cloned slice cycle does not point to cloned root slice")
		}
		cloned[0] = "changed"
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: assistantMessageWithArguments(map[string]any{
		"cycle": cycle,
	})})
	if err != nil {
		t.Fatalf("emit() returned error: %v", err)
	}
	sourceSelf := valueAs[[]any](t, cycle[0])
	if reflect.ValueOf(sourceSelf).Pointer() != reflect.ValueOf(cycle).Pointer() {
		t.Fatal("source slice cycle was modified")
	}
}

func TestAssistantMessageEventSink_rejectsUnexportedMutableFields(t *testing.T) {
	argument := hiddenMutableArgument{values: []string{"private"}}
	sinkCalled := false
	emit := assistantMessageEventSinkOrDiscard(func(AssistantMessageEvent) error {
		sinkCalled = true

		return nil
	})

	err := emit(AssistantToolCallDeltaEvent{Partial: assistantMessageWithArguments(map[string]any{
		"hidden": argument,
	})})

	if !errors.Is(err, ErrUnsupportedSnapshotValue) {
		t.Fatalf("emit() error = %v, want ErrUnsupportedSnapshotValue", err)
	}
	var cloneError *SnapshotCloneError
	if !errors.As(err, &cloneError) {
		t.Fatalf("emit() error type = %T, want *SnapshotCloneError", err)
	}
	if cloneError.Path == "" {
		t.Fatal("SnapshotCloneError.Path is empty")
	}
	if sinkCalled {
		t.Fatal("sink called with a snapshot that could not be cloned safely")
	}
	if argument.values[0] != "private" {
		t.Fatalf("source hidden value = %q, want private", argument.values[0])
	}
}

func assistantMessageWithArguments(arguments map[string]any) AssistantMessage {
	return AssistantMessage{
		Content: []AssistantContent{
			ToolCall{Arguments: arguments},
		},
	}
}

func toolCallSnapshotSink(
	t *testing.T,
	mutate func(map[string]any),
) AssistantMessageEventSink {
	t.Helper()

	return assistantMessageEventSinkOrDiscard(func(event AssistantMessageEvent) error {
		delta, ok := event.(AssistantToolCallDeltaEvent)
		if !ok {
			t.Fatalf("event type = %T, want AssistantToolCallDeltaEvent", event)
		}
		call := contentAt[ToolCall](t, delta.Partial, 0)
		mutate(call.Arguments)

		return nil
	})
}
