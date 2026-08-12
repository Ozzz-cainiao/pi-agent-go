package openairesponses

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

type streamEvent struct {
	Type         string          `json:"type"`
	Response     apiResponse     `json:"response"`
	Delta        string          `json:"delta"`
	Text         string          `json:"text"`
	Arguments    string          `json:"arguments"`
	Code         string          `json:"code"`
	Message      string          `json:"message"`
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
	Error             responseError  `json:"error"`
	IncompleteDetails struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
}

type responseItem struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	CallID    string            `json:"call_id"`
	Name      string            `json:"name"`
	Arguments string            `json:"arguments"`
	Content   []responseContent `json:"content"`
}

type responseError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type responseUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
	InputDetails struct {
		CachedTokens     int64 `json:"cached_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails struct {
		ReasoningTokens *int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

func decodeEventStream(
	reader io.Reader,
	emit agent.AssistantMessageEventSink,
	clock agent.Clock,
) (agent.AssistantMessage, error) {
	// Responses API 的每个 SSE data 行都是一个带 type 的 JSON 事件。decoder.consume
	// 按事件类型累积同一条 AssistantMessage，直到 completed/incomplete/failed。
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

func cloneResponseMessage(message agent.AssistantMessage) agent.AssistantMessage {
	message.Content = slices.Clone(message.Content)
	for index, content := range message.Content {
		if call, ok := content.(agent.ToolCall); ok {
			call.Arguments = cloneJSONObject(call.Arguments)
			message.Content[index] = call
		}
	}
	message.Usage.CacheWrite1HTokens = cloneInt64(message.Usage.CacheWrite1HTokens)
	message.Usage.ReasoningTokens = cloneInt64(message.Usage.ReasoningTokens)
	return message
}

func cloneInt64(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
