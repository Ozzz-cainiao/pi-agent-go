package openairesponses

import agent "github.com/Ozzz-cainiao/pi-agent-go"

func (decoder *textStreamDecoder) consume(event streamEvent) (agent.AssistantMessage, bool, error) {
	switch event.Type {
	case "response.created":
		decoder.mergeResponse(event.Response)
		return agent.AssistantMessage{}, false, decoder.start()
	case "response.output_text.start":
		return agent.AssistantMessage{}, false, decoder.startText(event.location())
	case "response.output_item.added":
		return agent.AssistantMessage{}, false, decoder.startOutputItem(event)
	case "response.content_part.added":
		if event.Part.Type != "output_text" {
			return agent.AssistantMessage{}, false, nil
		}
		return agent.AssistantMessage{}, false, decoder.startText(event.location())
	case "response.output_text.delta":
		return agent.AssistantMessage{}, false, decoder.appendText(event.location(), event.Delta)
	case "response.output_text.done", "response.content_part.done":
		return agent.AssistantMessage{}, false, decoder.endText(event.location(), event.Text)
	case "response.function_call_arguments.delta":
		return agent.AssistantMessage{}, false, decoder.appendToolArguments(event.OutputIndex, event.Delta)
	case "response.function_call_arguments.done":
		return agent.AssistantMessage{}, false, decoder.finishToolArguments(event.OutputIndex, event.Arguments)
	case "response.output_item.done":
		return agent.AssistantMessage{}, false, decoder.endOutputItem(event)
	case "response.completed", "response.incomplete":
		message, err := decoder.finish(event.Response)
		return message, true, err
	case "response.failed":
		return agent.AssistantMessage{}, true,
			decoder.fail(event.Response, event.Response.Error.Code, event.Response.Error.Message)
	case "error":
		return agent.AssistantMessage{}, true, decoder.fail(apiResponse{Status: "failed"}, event.Code, event.Message)
	default:
		return agent.AssistantMessage{}, false, nil
	}
}

func (decoder *textStreamDecoder) startOutputItem(event streamEvent) error {
	switch event.Item.Type {
	case "message":
		return decoder.startText(event.location())
	case "function_call":
		return decoder.startToolCall(event.OutputIndex, event.Item)
	default:
		return nil
	}
}

func (decoder *textStreamDecoder) endOutputItem(event streamEvent) error {
	switch event.Item.Type {
	case "message":
		return decoder.endText(event.location(), textFromItem(event.Item))
	case "function_call":
		return decoder.endToolCall(event.OutputIndex, event.Item)
	default:
		return nil
	}
}
