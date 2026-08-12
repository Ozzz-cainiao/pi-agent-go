// Package agent 是 pi-agent-go 面向调用方的稳定公共入口。
//
// 这里通过类型别名和薄包装导出 core 子包的能力，不复制业务逻辑。这样外部项目可以一直
// 使用简短的根包导入路径，而学习实现时可以进入 core 子包逐层阅读。
//
// 一般业务代码应导入 github.com/Ozzz-cainiao/pi-agent-go；只有研究内部实现或参与 Core
// 开发时，才需要直接阅读 github.com/Ozzz-cainiao/pi-agent-go/core。
package agent

import (
	"context"

	core "github.com/Ozzz-cainiao/pi-agent-go/core"
)

const (
	// 以下常量直接复用 core 中的定义，根包与 core 因此不会产生两套不兼容的枚举值。
	DefaultMaxTurns = core.DefaultMaxTurns

	StopReasonPending  = core.StopReasonPending
	StopReasonStop     = core.StopReasonStop
	StopReasonLength   = core.StopReasonLength
	StopReasonToolUse  = core.StopReasonToolUse
	StopReasonError    = core.StopReasonError
	StopReasonAborted  = core.StopReasonAborted
	StopReasonDeferred = core.StopReasonDeferred

	AgentEventAgentStart          = core.AgentEventAgentStart
	AgentEventAgentEnd            = core.AgentEventAgentEnd
	AgentEventTurnStart           = core.AgentEventTurnStart
	AgentEventTurnEnd             = core.AgentEventTurnEnd
	AgentEventMessageStart        = core.AgentEventMessageStart
	AgentEventMessageUpdate       = core.AgentEventMessageUpdate
	AgentEventMessageEnd          = core.AgentEventMessageEnd
	AgentEventToolExecutionStart  = core.AgentEventToolExecutionStart
	AgentEventToolExecutionUpdate = core.AgentEventToolExecutionUpdate
	AgentEventToolExecutionEnd    = core.AgentEventToolExecutionEnd
	ToolExecutionModeSequential   = core.ToolExecutionModeSequential
	ToolExecutionModeParallel     = core.ToolExecutionModeParallel
	UnknownModelID                = core.UnknownModelID
	ThinkingLevelOff              = core.ThinkingLevelOff
	ThinkingLevelMinimal          = core.ThinkingLevelMinimal
	ThinkingLevelLow              = core.ThinkingLevelLow
	ThinkingLevelMedium           = core.ThinkingLevelMedium
	ThinkingLevelHigh             = core.ThinkingLevelHigh
	ThinkingLevelXHigh            = core.ThinkingLevelXHigh
	ThinkingLevelMax              = core.ThinkingLevelMax
	QueueModeAll                  = core.QueueModeAll
	QueueModeOneAtATime           = core.QueueModeOneAtATime
)

var (
	// 以下错误保留 errors.Is/errors.As 语义，调用方无需知道错误实际定义在 core 中。
	ErrInvalidStopReason        = core.ErrInvalidStopReason
	ErrUnsupportedSnapshotValue = core.ErrUnsupportedSnapshotValue
	ErrAgentEventSink           = core.ErrAgentEventSink
	ErrInvalidLoopConfig        = core.ErrInvalidLoopConfig
	ErrNilStreamFunc            = core.ErrNilStreamFunc
	ErrMaxTurnsExceeded         = core.ErrMaxTurnsExceeded
	ErrInvalidContinuation      = core.ErrInvalidContinuation
	ErrEmptyContinuationContext = core.ErrEmptyContinuationContext
	ErrAssistantContinuation    = core.ErrAssistantContinuation
	ErrTurnControlHook          = core.ErrTurnControlHook
	ErrInvalidQueueMode         = core.ErrInvalidQueueMode
	ErrAgentClosed              = core.ErrAgentClosed
	ErrAgentSubscriber          = core.ErrAgentSubscriber
	ErrNilAgentSubscriber       = core.ErrNilAgentSubscriber
	ErrAgentBusy                = core.ErrAgentBusy
)

type (
	// 类型别名保持根包 API 与 core 实现完全相同，不产生运行时转换和额外分配。
	Content                       = core.Content
	TextContent                   = core.TextContent
	ThinkingContent               = core.ThinkingContent
	ImageContent                  = core.ImageContent
	ToolCall                      = core.ToolCall
	UsageCost                     = core.UsageCost
	Usage                         = core.Usage
	StopReason                    = core.StopReason
	AgentMessage                  = core.AgentMessage
	Message                       = core.Message
	CustomMessage                 = core.CustomMessage
	UserContent                   = core.UserContent
	UserMessage                   = core.UserMessage
	AssistantContent              = core.AssistantContent
	AssistantMessage              = core.AssistantMessage
	ToolResultContent             = core.ToolResultContent
	ToolResultMessage             = core.ToolResultMessage
	AgentContext                  = core.AgentContext
	AssistantMessageEvent         = core.AssistantMessageEvent
	AssistantMessageEventSink     = core.AssistantMessageEventSink
	StreamFunc                    = core.StreamFunc
	AssistantStartEvent           = core.AssistantStartEvent
	AssistantTextStartEvent       = core.AssistantTextStartEvent
	AssistantTextDeltaEvent       = core.AssistantTextDeltaEvent
	AssistantTextEndEvent         = core.AssistantTextEndEvent
	AssistantThinkingStartEvent   = core.AssistantThinkingStartEvent
	AssistantThinkingDeltaEvent   = core.AssistantThinkingDeltaEvent
	AssistantThinkingEndEvent     = core.AssistantThinkingEndEvent
	AssistantToolCallStartEvent   = core.AssistantToolCallStartEvent
	AssistantToolCallDeltaEvent   = core.AssistantToolCallDeltaEvent
	AssistantToolCallEndEvent     = core.AssistantToolCallEndEvent
	AssistantDoneEvent            = core.AssistantDoneEvent
	AssistantErrorEvent           = core.AssistantErrorEvent
	SnapshotCloneError            = core.SnapshotCloneError
	AgentEventKind                = core.AgentEventKind
	AgentEvent                    = core.AgentEvent
	AgentEventSink                = core.AgentEventSink
	AgentStartEvent               = core.AgentStartEvent
	AgentEndEvent                 = core.AgentEndEvent
	AgentTurnStartEvent           = core.AgentTurnStartEvent
	AgentTurnEndEvent             = core.AgentTurnEndEvent
	AgentMessageStartEvent        = core.AgentMessageStartEvent
	AgentMessageUpdateEvent       = core.AgentMessageUpdateEvent
	AgentMessageEndEvent          = core.AgentMessageEndEvent
	AgentToolExecutionStartEvent  = core.AgentToolExecutionStartEvent
	AgentToolExecutionUpdateEvent = core.AgentToolExecutionUpdateEvent
	AgentToolExecutionEndEvent    = core.AgentToolExecutionEndEvent
	AgentEventSinkError           = core.AgentEventSinkError
	Clock                         = core.Clock
	ConvertToLLMFunc              = core.ConvertToLLMFunc
	LoopConfig                    = core.LoopConfig
	LoopConfigError               = core.LoopConfigError
	MaxTurnsError                 = core.MaxTurnsError
	ContinuationError             = core.ContinuationError
	TransformContextFunc          = core.TransformContextFunc
	ShouldStopAfterTurnContext    = core.ShouldStopAfterTurnContext
	PrepareNextTurnContext        = core.PrepareNextTurnContext
	NextTurnUpdate                = core.NextTurnUpdate
	PrepareNextTurnFunc           = core.PrepareNextTurnFunc
	ShouldStopAfterTurnFunc       = core.ShouldStopAfterTurnFunc
	GetQueuedMessagesFunc         = core.GetQueuedMessagesFunc
	TurnControlHookError          = core.TurnControlHookError
	ToolExecutionMode             = core.ToolExecutionMode
	ToolDefinition                = core.ToolDefinition
	ToolResult                    = core.ToolResult
	ToolUpdateFunc                = core.ToolUpdateFunc
	Tool                          = core.Tool
	ArgumentValidator             = core.ArgumentValidator
	ArgumentValidatorFunc         = core.ArgumentValidatorFunc
	ToolArgumentPreparer          = core.ToolArgumentPreparer
	BeforeToolCallContext         = core.BeforeToolCallContext
	BeforeToolCallResult          = core.BeforeToolCallResult
	BeforeToolCallFunc            = core.BeforeToolCallFunc
	AfterToolCallContext          = core.AfterToolCallContext
	AfterToolCallResult           = core.AfterToolCallResult
	AfterToolCallFunc             = core.AfterToolCallFunc
	JSONSchemaArgumentValidator   = core.JSONSchemaArgumentValidator
	ModelID                       = core.ModelID
	ThinkingLevel                 = core.ThinkingLevel
	AgentState                    = core.AgentState
	AgentInitialState             = core.AgentInitialState
	AgentOptions                  = core.AgentOptions
	Agent                         = core.Agent
	QueueMode                     = core.QueueMode
	AgentSubscriber               = core.AgentSubscriber
	Unsubscribe                   = core.Unsubscribe
	AgentSubscriberError          = core.AgentSubscriberError
)

func ParseStopReason(raw string) (StopReason, error) {
	return core.ParseStopReason(raw)
}

// RunAgentLoop 调用低层循环；返回值只包含本次运行新增的消息，不包含 initial 中的旧历史。
func RunAgentLoop(
	ctx context.Context,
	prompts []AgentMessage,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) ([]AgentMessage, error) {
	return core.RunAgentLoop(ctx, prompts, initial, config, sink)
}

// ContinueAgentLoop 从已有 transcript 继续执行，不重新提交一条新的用户消息。
func ContinueAgentLoop(
	ctx context.Context,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) ([]AgentMessage, error) {
	return core.ContinueAgentLoop(ctx, initial, config, sink)
}

// NewAgent 创建管理 transcript、队列和订阅者的高层有状态 Agent。
func NewAgent(options AgentOptions) (*Agent, error) {
	return core.NewAgent(options)
}
