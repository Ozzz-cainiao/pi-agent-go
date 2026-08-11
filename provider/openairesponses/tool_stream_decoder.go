package openairesponses

import (
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type toolStreamSlot struct {
	call         agent.ToolCall
	partialJSON  string
	contentIndex int
	ended        bool
}

func (decoder *textStreamDecoder) startToolCall(outputIndex int, item responseItem) error {
	if err := decoder.start(); err != nil {
		return err
	}
	if slot, exists := decoder.tools[outputIndex]; exists {
		mergeToolIdentity(&slot.call, item)
		return nil
	}
	call := agent.ToolCall{ID: item.CallID, Name: item.Name, Arguments: map[string]any{}}
	if call.ID == "" {
		call.ID = item.ID
	}
	if arguments, err := decodeJSONObject(item.Arguments, "decode initial tool arguments"); err == nil {
		call.Arguments = arguments
	}
	slot := &toolStreamSlot{
		call: call, partialJSON: item.Arguments, contentIndex: len(decoder.partial.Content),
	}
	decoder.tools[outputIndex] = slot
	decoder.toolOrder = append(decoder.toolOrder, outputIndex)
	decoder.partial.Content = append(decoder.partial.Content, call)
	return decoder.send(agent.AssistantToolCallStartEvent{
		ContentIndex: slot.contentIndex,
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func (decoder *textStreamDecoder) appendToolArguments(outputIndex int, delta string) error {
	slot, exists := decoder.tools[outputIndex]
	if !exists {
		return &ProtocolError{Operation: "tool argument delta has no output item"}
	}
	slot.partialJSON += delta
	if arguments, err := decodeJSONObject(slot.partialJSON, "decode partial tool arguments"); err == nil {
		slot.call.Arguments = arguments
		decoder.partial.Content[slot.contentIndex] = slot.call
	}
	return decoder.send(agent.AssistantToolCallDeltaEvent{
		ContentIndex: slot.contentIndex,
		Delta:        delta,
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func (decoder *textStreamDecoder) finishToolArguments(outputIndex int, argumentsJSON string) error {
	slot, exists := decoder.tools[outputIndex]
	if !exists {
		return &ProtocolError{Operation: "completed tool arguments have no output item"}
	}
	if err := decoder.appendMissingToolArguments(outputIndex, argumentsJSON); err != nil {
		return err
	}
	arguments, err := decodeJSONObject(slot.partialJSON, "decode completed tool arguments")
	if err != nil {
		return err
	}
	slot.call.Arguments = arguments
	decoder.partial.Content[slot.contentIndex] = slot.call
	return nil
}

func (decoder *textStreamDecoder) appendMissingToolArguments(outputIndex int, completed string) error {
	slot := decoder.tools[outputIndex]
	if !strings.HasPrefix(completed, slot.partialJSON) {
		slot.partialJSON = completed
		return nil
	}
	delta := strings.TrimPrefix(completed, slot.partialJSON)
	if delta == "" {
		return nil
	}
	return decoder.appendToolArguments(outputIndex, delta)
}

func (decoder *textStreamDecoder) endToolCall(outputIndex int, item responseItem) error {
	if _, exists := decoder.tools[outputIndex]; !exists {
		if err := decoder.startToolCall(outputIndex, item); err != nil {
			return err
		}
	}
	slot := decoder.tools[outputIndex]
	if slot.ended {
		return nil
	}
	mergeToolIdentity(&slot.call, item)
	if item.Arguments != "" {
		slot.partialJSON = item.Arguments
	}
	arguments, err := decodeJSONObject(slot.partialJSON, "decode final tool arguments")
	if err != nil {
		return err
	}
	slot.call.Arguments = arguments
	slot.ended = true
	decoder.partial.Content[slot.contentIndex] = slot.call
	return decoder.send(agent.AssistantToolCallEndEvent{
		ContentIndex: slot.contentIndex,
		ToolCall:     cloneToolCall(slot.call),
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func mergeToolIdentity(call *agent.ToolCall, item responseItem) {
	if item.CallID != "" {
		call.ID = item.CallID
	} else if call.ID == "" {
		call.ID = item.ID
	}
	if item.Name != "" {
		call.Name = item.Name
	}
}

func cloneToolCall(call agent.ToolCall) agent.ToolCall {
	call.Arguments = cloneJSONObject(call.Arguments)
	return call
}
