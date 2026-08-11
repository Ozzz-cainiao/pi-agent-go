// Package openairesponses 将 OpenAI Responses API 适配为 agent.StreamFunc。
package openairesponses

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

const defaultBaseURL = "https://api.openai.com/v1"

var (
	// ErrInvalidConfig 表示 Provider 配置不完整或不合法。
	ErrInvalidConfig = errors.New("openai responses: invalid config")
	// ErrAPIResponse 表示远端 API 返回了非成功状态。
	ErrAPIResponse = errors.New("openai responses: api response error")
	// ErrProtocol 表示远端响应不符合 Responses API 流协议。
	ErrProtocol = errors.New("openai responses: protocol error")
	// ErrResponseFailed 表示 Responses 流以失败事件终止。
	ErrResponseFailed = errors.New("openai responses: response failed")
)

// Config 表示 Responses API Provider 的连接配置。
type Config struct {
	APIKey     string
	BaseURL    string
	Model      string
	HTTPClient *http.Client
	Clock      agent.Clock
}

// Provider 调用 OpenAI Responses API。
type Provider struct {
	apiKey     string
	baseURL    string
	model      string
	httpClient *http.Client
	clock      agent.Clock
}

// New 创建一个 Responses API Provider。
func New(config Config) (*Provider, error) {
	if strings.TrimSpace(config.APIKey) == "" {
		return nil, fmt.Errorf("APIKey is required: %w", ErrInvalidConfig)
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, fmt.Errorf("model is required: %w", ErrInvalidConfig)
	}

	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	clock := config.Clock
	if clock == nil {
		clock = time.Now
	}

	return &Provider{
		apiKey:     config.APIKey,
		baseURL:    baseURL,
		model:      config.Model,
		httpClient: httpClient,
		clock:      clock,
	}, nil
}

// Stream 调用 Responses API，并返回最终的 AssistantMessage。
func (provider *Provider) Stream(
	ctx context.Context,
	agentContext agent.AgentContext,
	emit agent.AssistantMessageEventSink,
) (agent.AssistantMessage, error) {
	body, err := newRequestBody(provider.model, agentContext)
	if err != nil {
		return agent.AssistantMessage{}, err
	}

	request, err := provider.newHTTPRequest(ctx, body)
	if err != nil {
		return agent.AssistantMessage{}, err
	}

	response, err := provider.httpClient.Do(request)
	if err != nil {
		return agent.AssistantMessage{}, fmt.Errorf("send request: %w", err)
	}
	defer func() {
		if err := response.Body.Close(); err != nil {
			return
		}
	}()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return agent.AssistantMessage{}, decodeAPIError(response)
	}

	return decodeEventStream(response.Body, emit, provider.clock)
}
