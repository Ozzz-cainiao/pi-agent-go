package openairesponses

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// APIError 描述 Responses HTTP API 返回的非成功响应。
type APIError struct {
	Cause      error
	Message    string
	Code       string
	StatusCode int
}

// Error 返回 HTTP API 错误说明。
func (apiError *APIError) Error() string {
	if apiError.Message == "" {
		return fmt.Sprintf("openai responses: status %d: %v", apiError.StatusCode, ErrAPIResponse)
	}
	return fmt.Sprintf("openai responses: status %d: %s", apiError.StatusCode, apiError.Message)
}

// Unwrap 同时暴露 API 错误哨兵和解码失败原因。
func (apiError *APIError) Unwrap() []error {
	if apiError.Cause == nil {
		return []error{ErrAPIResponse}
	}
	return []error{ErrAPIResponse, apiError.Cause}
}

// ResponseFailedError 描述 SSE 中的 response.failed 或 error 事件。
type ResponseFailedError struct {
	ResponseID string
	Status     string
	Code       string
	Message    string
}

// Error 返回流失败说明。
func (failedError *ResponseFailedError) Error() string {
	return fmt.Sprintf("openai responses: %s: %s: %v", failedError.Code, failedError.Message, ErrResponseFailed)
}

// Unwrap 暴露流失败和 API 哨兵错误。
func (failedError *ResponseFailedError) Unwrap() []error {
	return []error{ErrResponseFailed, ErrAPIResponse}
}

func decodeAPIError(response *http.Response) error {
	var payload struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return &APIError{StatusCode: response.StatusCode, Cause: err}
	}
	return &APIError{
		StatusCode: response.StatusCode,
		Code:       payload.Error.Code,
		Message:    payload.Error.Message,
	}
}
