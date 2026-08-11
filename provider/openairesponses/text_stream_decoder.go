package openairesponses

import (
	"fmt"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type textLocation struct {
	outputIndex  int
	contentIndex int
}

type textStreamDecoder struct {
	emit    agent.AssistantMessageEventSink
	clock   agent.Clock
	partial agent.AssistantMessage
	slots   map[textLocation]int
	order   []textLocation
	ended   map[textLocation]bool
	started bool
}

func newTextStreamDecoder(emit agent.AssistantMessageEventSink, clock agent.Clock) *textStreamDecoder {
	return &textStreamDecoder{
		emit:  emit,
		clock: clock,
		slots: make(map[textLocation]int),
		ended: make(map[textLocation]bool),
		partial: agent.AssistantMessage{
			API:        "openai-responses",
			Provider:   "openai",
			StopReason: agent.StopReasonPending,
			Timestamp:  clock().UnixMilli(),
		},
	}
}

func (decoder *textStreamDecoder) consume(event streamEvent) (agent.AssistantMessage, bool, error) {
	switch event.Type {
	case "response.created":
		decoder.mergeResponse(event.Response)
		return agent.AssistantMessage{}, false, decoder.start()
	case "response.output_text.start":
		return agent.AssistantMessage{}, false, decoder.startText(event.location())
	case "response.output_item.added":
		if event.Item.Type != "message" {
			return agent.AssistantMessage{}, false, nil
		}
		return agent.AssistantMessage{}, false, decoder.startText(event.location())
	case "response.content_part.added":
		if event.Part.Type != "output_text" {
			return agent.AssistantMessage{}, false, nil
		}
		return agent.AssistantMessage{}, false, decoder.startText(event.location())
	case "response.output_text.delta":
		return agent.AssistantMessage{}, false, decoder.appendText(event.location(), event.Delta)
	case "response.output_text.done", "response.content_part.done":
		return agent.AssistantMessage{}, false, decoder.endText(event.location(), event.Text)
	case "response.output_item.done":
		if event.Item.Type != "message" {
			return agent.AssistantMessage{}, false, nil
		}
		return agent.AssistantMessage{}, false, decoder.endText(event.location(), textFromItem(event.Item))
	case "response.completed", "response.incomplete":
		message, err := decoder.finish(event.Response)
		return message, true, err
	default:
		return agent.AssistantMessage{}, false, nil
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
	return decoder.send(agent.AssistantStartEvent{Partial: cloneTextMessage(decoder.partial)})
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
		Partial:      cloneTextMessage(decoder.partial),
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
		Partial:      cloneTextMessage(decoder.partial),
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
		Partial:      cloneTextMessage(decoder.partial),
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
	decoder.partial.StopReason = stopReasonFrom(response)
	message := cloneTextMessage(decoder.partial)
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
	decoder.partial.Usage = agent.Usage{
		InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens,
		TotalTokens: response.Usage.TotalTokens,
	}
}

func (decoder *textStreamDecoder) backfillResponseText(response apiResponse) error {
	if len(decoder.partial.Content) != 0 {
		return nil
	}
	for outputIndex, item := range response.Output {
		text := textFromItem(item)
		if text == "" {
			continue
		}
		location := textLocation{outputIndex: outputIndex}
		if err := decoder.startText(location); err != nil {
			return err
		}
		if err := decoder.endText(location, text); err != nil {
			return err
		}
	}
	return nil
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
