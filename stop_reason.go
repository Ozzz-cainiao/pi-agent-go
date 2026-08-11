package agent

import (
	"errors"
	"fmt"
)

// StopReason 表示模型响应停止的原因。
type StopReason string

const (
	// StopReasonPending 表示响应仍在流式生成。
	StopReasonPending StopReason = "pending"

	// StopReasonStop 表示模型正常完成响应。
	StopReasonStop StopReason = "stop"

	// StopReasonLength 表示模型达到长度限制。
	StopReasonLength StopReason = "length"

	// StopReasonToolUse 表示模型请求执行工具。
	StopReasonToolUse StopReason = "toolUse"

	// StopReasonError 表示模型执行失败。
	StopReasonError StopReason = "error"

	// StopReasonAborted 表示执行被取消。
	StopReasonAborted StopReason = "aborted"

	// StopReasonDeferred 表示响应需要稍后获取。
	StopReasonDeferred StopReason = "deferred"
)

// ErrInvalidStopReason 表示不受支持的 StopReason。
var ErrInvalidStopReason = errors.New("agent: invalid stop reason")

// ParseStopReason 将外部字符串解析为合法的 StopReason。
func ParseStopReason(raw string) (StopReason, error) {
	reason := StopReason(raw)

	switch reason {
	case StopReasonPending,
		StopReasonStop,
		StopReasonLength,
		StopReasonToolUse,
		StopReasonError,
		StopReasonAborted,
		StopReasonDeferred:
		return reason, nil
	}

	return "", fmt.Errorf("parse stop reason %q: %w", raw, ErrInvalidStopReason)
}
