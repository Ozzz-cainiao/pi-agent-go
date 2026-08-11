package openairesponses

import (
	"errors"
	"fmt"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type textLocation struct {
	outputIndex  int
	contentIndex int
}

type textStreamDecoder struct {
	emit      agent.AssistantMessageEventSink
	clock     agent.Clock
	partial   agent.AssistantMessage
	slots     map[textLocation]int
	order     []textLocation
	ended     map[textLocation]bool
	tools     map[int]*toolStreamSlot
	toolOrder []int
	started   bool
}

func newTextStreamDecoder(emit agent.AssistantMessageEventSink, clock agent.Clock) *textStreamDecoder {
	return &textStreamDecoder{
		emit:  emit,
		clock: clock,
		slots: make(map[textLocation]int),
		ended: make(map[textLocation]bool),
		tools: make(map[int]*toolStreamSlot),
		partial: agent.AssistantMessage{
			API:        "openai-responses",
			Provider:   "openai",
			StopReason: agent.StopReasonPending,
			Timestamp:  clock().UnixMilli(),
		},
	}
}

func (event streamEvent) location() textLocation {
	return textLocation{outputIndex: event.OutputIndex, contentIndex: event.ContentIndex}
}

func (decoder *textStreamDecoder) start() error {
	if decoder.started {
		return nil
	}
	decoder.started = true
	return decoder.send(agent.AssistantStartEvent{Partial: cloneResponseMessage(decoder.partial)})
}

func (decoder *textStreamDecoder) startText(location textLocation) error {
	if err := decoder.start(); err != nil {
		return err
	}
	if _, exists := decoder.slots[location]; exists {
		return nil
	}
	index := len(decoder.partial.Content)
	decoder.slots[location] = index
	decoder.order = append(decoder.order, location)
	decoder.partial.Content = append(decoder.partial.Content, agent.TextContent{})
	return decoder.send(agent.AssistantTextStartEvent{
		ContentIndex: index,
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func (decoder *textStreamDecoder) appendText(location textLocation, delta string) error {
	if err := decoder.startText(location); err != nil {
		return err
	}
	index := decoder.slots[location]
	content, ok := decoder.partial.Content[index].(agent.TextContent)
	if !ok {
		return &ProtocolError{Operation: "text slot contains a non-text value"}
	}
	content.Text += delta
	decoder.partial.Content[index] = content
	return decoder.send(agent.AssistantTextDeltaEvent{
		ContentIndex: index,
		Delta:        delta,
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func (decoder *textStreamDecoder) endText(location textLocation, finalText string) error {
	if err := decoder.startText(location); err != nil {
		return err
	}
	if decoder.ended[location] {
		return nil
	}
	index := decoder.slots[location]
	content, ok := decoder.partial.Content[index].(agent.TextContent)
	if !ok {
		return &ProtocolError{Operation: "text slot contains a non-text value"}
	}
	if finalText != "" {
		content.Text = finalText
	}
	decoder.partial.Content[index] = content
	decoder.ended[location] = true
	return decoder.send(agent.AssistantTextEndEvent{
		ContentIndex: index,
		Content:      content.Text,
		Partial:      cloneResponseMessage(decoder.partial),
	})
}

func (decoder *textStreamDecoder) finish(response apiResponse) (agent.AssistantMessage, error) {
	decoder.mergeResponse(response)
	if err := decoder.start(); err != nil {
		return agent.AssistantMessage{}, err
	}
	if err := decoder.backfillResponseText(response); err != nil {
		return agent.AssistantMessage{}, err
	}
	for _, location := range decoder.order {
		if err := decoder.endText(location, ""); err != nil {
			return agent.AssistantMessage{}, err
		}
	}
	for _, outputIndex := range decoder.toolOrder {
		if err := decoder.endToolCall(outputIndex, responseItem{}); err != nil {
			return agent.AssistantMessage{}, err
		}
	}
	decoder.partial.StopReason = stopReasonFrom(response)
	if hasToolCall(decoder.partial) && decoder.partial.StopReason == agent.StopReasonStop {
		decoder.partial.StopReason = agent.StopReasonToolUse
	}
	message := cloneResponseMessage(decoder.partial)
	if err := decoder.send(agent.AssistantDoneEvent{Reason: message.StopReason, Message: message}); err != nil {
		return agent.AssistantMessage{}, err
	}
	return message, nil
}

func (decoder *textStreamDecoder) mergeResponse(response apiResponse) {
	if response.ID != "" {
		decoder.partial.ResponseID = response.ID
	}
	if response.Model != "" {
		decoder.partial.Model = response.Model
		decoder.partial.ResponseModel = response.Model
	}
	decoder.partial.Usage = usageFrom(response.Usage)
}

func (decoder *textStreamDecoder) backfillResponseText(response apiResponse) error {
	if len(decoder.partial.Content) != 0 {
		return nil
	}
	for outputIndex, item := range response.Output {
		switch item.Type {
		case "message":
			location := textLocation{outputIndex: outputIndex}
			if err := decoder.endText(location, textFromItem(item)); err != nil {
				return err
			}
		case "function_call":
			if err := decoder.endToolCall(outputIndex, item); err != nil {
				return err
			}
		}
	}
	return nil
}

func (decoder *textStreamDecoder) fail(response apiResponse, code, message string) error {
	failedError := &ResponseFailedError{
		ResponseID: response.ID, Status: response.Status, Code: code, Message: message,
	}
	decoder.mergeResponse(response)
	if err := decoder.start(); err != nil {
		return errors.Join(failedError, err)
	}
	decoder.partial.StopReason = agent.StopReasonError
	decoder.partial.RawStopReason = response.Status
	decoder.partial.ErrorMessage = message
	emitError := decoder.send(agent.AssistantErrorEvent{
		Reason: agent.StopReasonError,
		Error:  cloneResponseMessage(decoder.partial),
	})
	return errors.Join(failedError, emitError)
}

func (decoder *textStreamDecoder) send(event agent.AssistantMessageEvent) error {
	if decoder.emit == nil {
		return nil
	}
	if err := decoder.emit(event); err != nil {
		return fmt.Errorf("emit assistant event: %w", err)
	}
	return nil
}

func textFromItem(item responseItem) string {
	for _, content := range item.Content {
		if content.Type == "output_text" {
			return content.Text
		}
	}
	return ""
}

func stopReasonFrom(response apiResponse) agent.StopReason {
	if response.Status == "incomplete" && response.IncompleteDetails.Reason == "max_output_tokens" {
		return agent.StopReasonLength
	}
	return agent.StopReasonStop
}

func hasToolCall(message agent.AssistantMessage) bool {
	for _, content := range message.Content {
		if _, ok := content.(agent.ToolCall); ok {
			return true
		}
	}
	return false
}
