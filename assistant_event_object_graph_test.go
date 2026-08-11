package agent

import (
	"errors"
	"reflect"
	"testing"
)

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
