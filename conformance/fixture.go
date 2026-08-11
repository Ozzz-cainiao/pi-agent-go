package conformance

import (
	"errors"
	"fmt"
)

// SchemaVersion 是当前 conformance fixture 的 JSON schema 版本。
const SchemaVersion = 1

var (
	// ErrInvalidFixture 表示 fixture 不能被安全执行。
	ErrInvalidFixture = errors.New("conformance: invalid fixture")
	// ErrUnknownVersion 表示 runner 不支持 fixture 的 schema version。
	ErrUnknownVersion = errors.New("conformance: unknown schema version")
	// ErrInvalidEvent 表示 expected.events 包含未知 Agent 事件。
	ErrInvalidEvent = errors.New("conformance: invalid event")
	// ErrFixtureMismatch 表示 Core 实际结果与 golden 不一致。
	ErrFixtureMismatch = errors.New("conformance: fixture mismatch")
)

// Fixture 是可跨语言读取的 Agent 行为兼容用例。
type Fixture struct {
	Input        FixtureInput     `json:"input"`
	Expected     FixtureExpected  `json:"expected"`
	Name         string           `json:"name"`
	UpstreamCase string           `json:"upstream_case"`
	Scenario     string           `json:"scenario"`
	Script       []ResponseRecord `json:"script"`
	Version      int              `json:"version"`
}

// FixtureInput 记录场景输入、历史、队列和工具结果。
type FixtureInput struct {
	Prompt      string            `json:"prompt,omitempty"`
	History     []MessageRecord   `json:"history,omitempty"`
	Queued      []string          `json:"queued,omitempty"`
	ToolResults map[string]string `json:"tool_results,omitempty"`
}

// FixtureExpected 记录 golden 事件、消息和最终停止原因。
type FixtureExpected struct {
	Events     []string        `json:"events"`
	Messages   []MessageRecord `json:"messages"`
	StopReason string          `json:"stop_reason"`
}

// ResponseRecord 描述 scripted Model 的一次响应。
type ResponseRecord struct {
	Text          string           `json:"text,omitempty"`
	StopReason    string           `json:"stop_reason,omitempty"`
	ToolCalls     []ToolCallRecord `json:"tool_calls,omitempty"`
	WaitForCancel bool             `json:"wait_for_cancel,omitempty"`
}

// MessageRecord 是 fixture 中与语言无关的 transcript 消息。
type MessageRecord struct {
	Role       string           `json:"role"`
	Text       string           `json:"text,omitempty"`
	ToolName   string           `json:"tool_name,omitempty"`
	ToolCallID string           `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCallRecord `json:"tool_calls,omitempty"`
	IsError    bool             `json:"is_error,omitempty"`
}

// ToolCallRecord 是 fixture 中与语言无关的工具调用。
type ToolCallRecord struct {
	Arguments map[string]any `json:"arguments"`
	ID        string         `json:"id"`
	Name      string         `json:"name"`
}

// Result 是 runner 从 Core 行为归一化得到的结果。
type Result struct {
	Events     []string
	Messages   []MessageRecord
	StopReason string
}

// FixtureError 精确标识非法 fixture 的字段和原因。
type FixtureError struct {
	Path  string
	Field string
	Cause error
}

// Error 返回 fixture 校验错误说明。
func (fixtureError *FixtureError) Error() string {
	return fmt.Sprintf("conformance fixture %q field %s: %v", fixtureError.Path, fixtureError.Field, fixtureError.Cause)
}

// Unwrap 同时暴露通用 fixture 错误和具体原因。
func (fixtureError *FixtureError) Unwrap() []error {
	return []error{ErrInvalidFixture, fixtureError.Cause}
}

// MismatchError 描述一个稳定、可读的 golden 差异。
type MismatchError struct {
	Fixture string
	Field   string
	Want    any
	Got     any
}

// Error 返回 golden 差异说明。
func (mismatch *MismatchError) Error() string {
	return fmt.Sprintf(
		"conformance fixture %q mismatch at %s: want %s, got %s: %v",
		mismatch.Fixture, mismatch.Field, formatValue(mismatch.Want), formatValue(mismatch.Got), ErrFixtureMismatch,
	)
}

// Unwrap 暴露 golden mismatch 哨兵错误。
func (mismatch *MismatchError) Unwrap() error {
	return ErrFixtureMismatch
}
