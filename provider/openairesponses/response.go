package openairesponses

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type streamEvent struct {
	Type         string          `json:"type"`
	Response     apiResponse     `json:"response"`
	Delta        string          `json:"delta"`
	Text         string          `json:"text"`
	OutputIndex  int             `json:"output_index"`
	ContentIndex int             `json:"content_index"`
	Item         responseItem    `json:"item"`
	Part         responseContent `json:"part"`
}

type apiResponse struct {
	ID                string         `json:"id"`
	Model             string         `json:"model"`
	Status            string         `json:"status"`
	Output            []responseItem `json:"output"`
	Usage             responseUsage  `json:"usage"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responseItem struct {
	Type      string            `json:"type"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Arguments string            `json:"arguments"`
	Content   []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
}

func decodeEventStream(
	reader io.Reader,
	emit agent.AssistantMessageEventSink,
	clock agent.Clock,
) (agent.AssistantMessage, error) {
	decoder := newTextStreamDecoder(emit, clock)
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		data, ok := sseDataFromLine(scanner.Text())
		if !ok {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return agent.AssistantMessage{}, &ProtocolError{Operation: "decode SSE event", Cause: err}
		}
		message, terminal, err := decoder.consume(event)
		if err != nil {
			return agent.AssistantMessage{}, err
		}
		if terminal {
			return message, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.AssistantMessage{}, fmt.Errorf("read event stream: %w", err)
	}
	return agent.AssistantMessage{}, &ProtocolError{Operation: "terminal event is missing"}
}

func sseDataFromLine(line string) (string, bool) {
	if !strings.HasPrefix(line, "data:") {
		return "", false
	}
	data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
	return data, data != "" && data != "[DONE]"
}

func decodeAPIError(response *http.Response) error {
	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return fmt.Errorf("status %d: %w", response.StatusCode, ErrAPIResponse)
	}
	return fmt.Errorf("status %d: %s: %w", response.StatusCode, payload.Error.Message, ErrAPIResponse)
}

func cloneTextMessage(message agent.AssistantMessage) agent.AssistantMessage {
	message.Content = slices.Clone(message.Content)
	return message
}
