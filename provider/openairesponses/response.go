package openairesponses

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type streamEvent struct {
	Type     string       `json:"type"`
	Response apiResponse  `json:"response"`
	Delta    string       `json:"delta"`
	Item     responseItem `json:"item"`
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
) (agent.AssistantMessage, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}

		var event streamEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			return agent.AssistantMessage{}, fmt.Errorf("decode event: %w: %w", err, ErrProtocol)
		}
		if event.Type == "response.completed" {
			return messageFromResponse(event.Response), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return agent.AssistantMessage{}, fmt.Errorf("read event stream: %w", err)
	}
	return agent.AssistantMessage{}, fmt.Errorf("response.completed event is missing: %w", ErrProtocol)
}

func messageFromResponse(response apiResponse) agent.AssistantMessage {
	return agent.AssistantMessage{
		API:           "openai-responses",
		Provider:      "openai",
		Model:         response.Model,
		ResponseModel: response.Model,
		ResponseID:    response.ID,
		Usage: agent.Usage{
			InputTokens:  response.Usage.InputTokens,
			OutputTokens: response.Usage.OutputTokens,
			TotalTokens:  response.Usage.TotalTokens,
		},
		StopReason: stopReasonFrom(response),
		Timestamp:  time.Now().UnixMilli(),
	}
}

func stopReasonFrom(response apiResponse) agent.StopReason {
	if response.Status == "incomplete" && response.IncompleteDetails.Reason == "max_output_tokens" {
		return agent.StopReasonLength
	}
	return agent.StopReasonStop
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
