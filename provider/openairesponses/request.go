package openairesponses

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type requestBody struct {
	Model        string             `json:"model"`
	Instructions string             `json:"instructions,omitempty"`
	Input        []requestInputItem `json:"input"`
	Tools        []requestTool      `json:"tools,omitempty"`
	Stream       bool               `json:"stream"`
}

type requestInputItem struct {
	Type      string           `json:"type,omitempty"`
	Role      string           `json:"role,omitempty"`
	Content   []requestContent `json:"content,omitempty"`
	CallID    string           `json:"call_id,omitempty"`
	Name      string           `json:"name,omitempty"`
	Arguments string           `json:"arguments,omitempty"`
	Output    string           `json:"output,omitempty"`
}

type requestContent struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	ImageURL string `json:"image_url,omitempty"`
}

type requestTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
}

func newRequestBody(model string, context agent.AgentContext) (requestBody, error) {
	input, err := convertMessages(context.Messages)
	if err != nil {
		return requestBody{}, err
	}

	return requestBody{
		Model:        model,
		Instructions: context.SystemPrompt,
		Input:        input,
		Tools:        convertTools(context.Tools),
		Stream:       true,
	}, nil
}

func (provider *Provider) newHTTPRequest(
	ctx context.Context,
	body requestBody,
) (*http.Request, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("encode request: %w", err)
	}

	request, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		provider.baseURL+"/responses",
		bytes.NewReader(encoded),
	)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+provider.apiKey)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "text/event-stream")

	return request, nil
}

func convertMessages(messages []agent.AgentMessage) ([]requestInputItem, error) {
	items := make([]requestInputItem, 0, len(messages))
	for _, message := range messages {
		switch value := message.(type) {
		case agent.UserMessage:
			content, err := convertUserContent(value.Content)
			if err != nil {
				return nil, err
			}
			items = append(items, requestInputItem{Role: "user", Content: content})
		case agent.AssistantMessage:
			converted, err := convertAssistantMessage(value)
			if err != nil {
				return nil, err
			}
			items = append(items, converted...)
		case agent.ToolResultMessage:
			items = append(items, requestInputItem{
				Type:   "function_call_output",
				CallID: value.ToolCallID,
				Output: toolResultText(value.Content),
			})
		default:
			return nil, fmt.Errorf("unsupported message %T: %w", message, ErrProtocol)
		}
	}
	return items, nil
}

func convertUserContent(contents []agent.UserContent) ([]requestContent, error) {
	converted := make([]requestContent, 0, len(contents))
	for _, content := range contents {
		switch value := content.(type) {
		case agent.TextContent:
			converted = append(converted, requestContent{Type: "input_text", Text: value.Text})
		case agent.ImageContent:
			converted = append(converted, requestContent{
				Type:     "input_image",
				ImageURL: "data:" + value.MIMEType + ";base64," + value.Data,
			})
		default:
			return nil, fmt.Errorf("unsupported user content %T: %w", content, ErrProtocol)
		}
	}
	return converted, nil
}

func convertAssistantMessage(message agent.AssistantMessage) ([]requestInputItem, error) {
	items := make([]requestInputItem, 0, len(message.Content))
	for _, content := range message.Content {
		switch value := content.(type) {
		case agent.TextContent:
			items = append(items, requestInputItem{
				Role:    "assistant",
				Content: []requestContent{{Type: "output_text", Text: value.Text}},
			})
		case agent.ToolCall:
			arguments, err := json.Marshal(value.Arguments)
			if err != nil {
				return nil, &ProtocolError{Operation: "encode tool arguments", Cause: err}
			}
			items = append(items, requestInputItem{
				Type:      "function_call",
				CallID:    value.ID,
				Name:      value.Name,
				Arguments: string(arguments),
			})
		}
	}
	return items, nil
}

func convertTools(tools []agent.Tool) []requestTool {
	converted := make([]requestTool, 0, len(tools))
	for _, tool := range tools {
		definition := tool.Definition()
		parameters := definition.Parameters
		if len(parameters) == 0 {
			parameters = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		converted = append(converted, requestTool{
			Type:        "function",
			Name:        definition.Name,
			Description: definition.Description,
			Parameters:  parameters,
		})
	}
	return converted
}

func toolResultText(contents []agent.ToolResultContent) string {
	parts := make([]string, 0, len(contents))
	for _, content := range contents {
		if text, ok := content.(agent.TextContent); ok {
			parts = append(parts, text.Text)
		}
	}
	return strings.Join(parts, "\n")
}
