package agent

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// DefaultMaxTurns 是未显式配置时允许的最大 Model 调用轮数。
const DefaultMaxTurns = 32

var (
	// ErrInvalidLoopConfig 表示 Agent Loop 配置不合法。
	ErrInvalidLoopConfig = errors.New("agent: invalid loop config")
	// ErrNilStreamFunc 表示 Agent Loop 未注入 StreamFunc。
	ErrNilStreamFunc = errors.New("agent: nil stream func")
	// ErrMaxTurnsExceeded 表示 Agent Loop 已达到最大轮数。
	ErrMaxTurnsExceeded = errors.New("agent: max turns exceeded")
)

// Clock 返回 Agent Loop 使用的当前时间。
type Clock func() time.Time

// ConvertToLLMFunc 将 Agent 消息转换为 Model 可接收的核心消息。
type ConvertToLLMFunc func(context.Context, []AgentMessage) ([]Message, error)

// LoopConfig 集中声明 Agent Loop 的依赖、扩展点与安全限制。
//
// Stream 是唯一必填项，它把 Core 接到具体 Model Provider。其余函数字段都是可选 Hook；
// 零值配置会采用默认消息转换、JSON Schema 校验、并行工具执行和 DefaultMaxTurns。
type LoopConfig struct {
	Stream              StreamFunc
	TransformContext    TransformContextFunc
	ConvertToLLM        ConvertToLLMFunc
	PrepareNextTurn     PrepareNextTurnFunc
	ArgumentValidator   ArgumentValidator
	BeforeToolCall      BeforeToolCallFunc
	AfterToolCall       AfterToolCallFunc
	ShouldStopAfterTurn ShouldStopAfterTurnFunc
	GetSteeringMessages GetQueuedMessagesFunc
	GetFollowUpMessages GetQueuedMessagesFunc
	ToolExecution       ToolExecutionMode
	Clock               Clock
	MaxTurns            int
}

// LoopConfigError 描述不合法的 Agent Loop 配置字段。
type LoopConfigError struct {
	Field string
	Cause error
}

// Error 返回配置错误说明。
func (loopError *LoopConfigError) Error() string {
	return fmt.Sprintf("agent: invalid loop config field %s: %v", loopError.Field, loopError.Cause)
}

// Unwrap 同时暴露通用配置错误和字段错误。
func (loopError *LoopConfigError) Unwrap() []error {
	return []error{ErrInvalidLoopConfig, loopError.Cause}
}

// MaxTurnsError 描述触发安全停止时允许的最大轮数。
type MaxTurnsError struct {
	MaxTurns int
}

// Error 返回最大轮数错误说明。
func (turnError *MaxTurnsError) Error() string {
	return fmt.Sprintf("agent: maximum turn count %d reached", turnError.MaxTurns)
}

// Unwrap 暴露最大轮数哨兵错误。
func (turnError *MaxTurnsError) Unwrap() error {
	return ErrMaxTurnsExceeded
}

type loopRuntime struct {
	stream              StreamFunc
	transformContext    TransformContextFunc
	convertToLLM        ConvertToLLMFunc
	prepareNextTurn     PrepareNextTurnFunc
	argumentValidator   ArgumentValidator
	beforeToolCall      BeforeToolCallFunc
	afterToolCall       AfterToolCallFunc
	shouldStopAfterTurn ShouldStopAfterTurnFunc
	getSteeringMessages GetQueuedMessagesFunc
	getFollowUpMessages GetQueuedMessagesFunc
	toolExecution       ToolExecutionMode
	clock               Clock
	maxTurns            int
}

func (config LoopConfig) runtime() (loopRuntime, error) {
	if config.Stream == nil {
		return loopRuntime{}, &LoopConfigError{
			Field: "Stream",
			Cause: ErrNilStreamFunc,
		}
	}
	if config.MaxTurns < 0 {
		return loopRuntime{}, &LoopConfigError{
			Field: "MaxTurns",
			Cause: fmt.Errorf("must not be negative: %d", config.MaxTurns),
		}
	}

	maxTurns := config.MaxTurns
	if maxTurns == 0 {
		maxTurns = DefaultMaxTurns
	}
	clock := resolvedClock(config.Clock)
	convertToLLM := config.ConvertToLLM
	if convertToLLM == nil {
		convertToLLM = defaultConvertToLLM
	}
	argumentValidator := config.ArgumentValidator
	if argumentValidator == nil {
		argumentValidator = JSONSchemaArgumentValidator{}
	}
	toolExecution := config.ToolExecution
	if toolExecution == "" {
		toolExecution = ToolExecutionModeParallel
	}
	if toolExecution != ToolExecutionModeSequential && toolExecution != ToolExecutionModeParallel {
		return loopRuntime{}, &LoopConfigError{
			Field: "ToolExecution",
			Cause: fmt.Errorf("unknown mode %q", toolExecution),
		}
	}

	return loopRuntime{
		stream:              config.Stream,
		transformContext:    config.TransformContext,
		convertToLLM:        convertToLLM,
		prepareNextTurn:     config.PrepareNextTurn,
		argumentValidator:   argumentValidator,
		beforeToolCall:      config.BeforeToolCall,
		afterToolCall:       config.AfterToolCall,
		shouldStopAfterTurn: config.ShouldStopAfterTurn,
		getSteeringMessages: config.GetSteeringMessages,
		getFollowUpMessages: config.GetFollowUpMessages,
		toolExecution:       toolExecution,
		clock:               clock,
		maxTurns:            maxTurns,
	}, nil
}

func resolvedClock(clock Clock) Clock {
	if clock == nil {
		return Clock(time.Now)
	}
	return clock
}

func defaultConvertToLLM(
	_ context.Context,
	messages []AgentMessage,
) ([]Message, error) {
	converted := make([]Message, 0, len(messages))
	for _, message := range messages {
		if llmMessage, ok := message.toLLM(); ok {
			converted = append(converted, llmMessage)
		}
	}

	return converted, nil
}

// RunAgentLoop 使用本轮 prompts 启动 Agent Loop，并返回本次新增的全部消息。
//
// 参数职责：
//
//   - ctx：把 Gateway 断连、请求超时等取消信号一直传到 Provider 和 Tool。
//   - prompts：这次刚进入循环的新消息，通常是一条 UserMessage。
//   - initial：调用前已有的 SystemPrompt、历史消息和可用工具快照。
//   - config：Provider Stream、Hook、时钟与轮次上限。
//   - sink：可选的完整生命周期观察者；它不参与业务决策。
//
// 返回切片不重复 initial.Messages，便于上层只把增量追加回 Session。
func RunAgentLoop(
	ctx context.Context,
	prompts []AgentMessage,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) (messages []AgentMessage, runError error) {
	// state.current 是发送给下一轮 Model 的累计上下文；state.messages 只记录本次增量。
	newMessages := slices.Clone(prompts)
	state := loopState{
		current:  initial.WithMessages(prompts...),
		messages: newMessages,
	}
	return runLoopLifecycle(ctx, prompts, config, sink, &state)
}

func runLoopLifecycle(
	ctx context.Context,
	prompts []AgentMessage,
	config LoopConfig,
	sink AgentEventSink,
	state *loopState,
) ([]AgentMessage, error) {
	// 即使中间失败也尝试发送 agent_end，让 Gateway/UI 能闭合一次运行的生命周期。
	events := newAgentEventEmitter(sink)
	if err := events.emit(AgentStartEvent{}); err != nil {
		return state.messages, err
	}

	runError := runAgentTurns(ctx, prompts, config, events, state)
	endError := events.emit(AgentEndEvent{Messages: state.messages})

	return state.messages, errors.Join(runError, endError)
}

// toolCallsFrom 提取 AssistantMessage 中的全部工具调用。
func toolCallsFrom(message AssistantMessage) []ToolCall {
	var calls []ToolCall

	for _, content := range message.Content {
		switch value := content.(type) {
		case ToolCall:
			calls = append(calls, value)
		case *ToolCall:
			if value != nil {
				calls = append(calls, *value)
			}
		}
	}

	return calls
}

// newTruncatedToolResultMessage 为参数可能被截断的工具调用创建错误结果。
func newTruncatedToolResultMessage(
	call ToolCall,
	timestamp int64,
) ToolResultMessage {
	message := fmt.Sprintf(
		`Tool call %q was not executed: the response hit the output token limit, so its arguments may be truncated. Re-issue the tool call with complete arguments.`,
		call.Name,
	)

	return newToolResultMessage(
		call,
		newErrorToolResult(message),
		true,
		timestamp,
	)
}

var (
	// ErrInvalidContinuation 表示无法从当前上下文继续 Agent Loop。
	ErrInvalidContinuation = errors.New("agent: invalid continuation")
	// ErrEmptyContinuationContext 表示继续执行时没有历史消息。
	ErrEmptyContinuationContext = errors.New("agent: empty continuation context")
	// ErrAssistantContinuation 表示不能从 Assistant 消息之后直接继续。
	ErrAssistantContinuation = errors.New("agent: cannot continue after assistant message")
)

// ContinuationError 描述 ContinueAgentLoop 的输入错误。
type ContinuationError struct{ Cause error }

func (continuationError *ContinuationError) Error() string {
	return fmt.Sprintf("agent: cannot continue loop: %v", continuationError.Cause)
}

func (continuationError *ContinuationError) Unwrap() []error {
	return []error{ErrInvalidContinuation, continuationError.Cause}
}

// ContinueAgentLoop 从已有历史继续执行，只返回本次新增消息。
//
// 与 RunAgentLoop 的区别是它没有新 prompts，适合 transcript 已以 ToolResultMessage 等
// 可继续状态结尾的场景；AssistantMessage 结尾会被拒绝，避免无输入地反复请求 Model。
func ContinueAgentLoop(
	ctx context.Context,
	initial AgentContext,
	config LoopConfig,
	sink AgentEventSink,
) ([]AgentMessage, error) {
	if err := validateContinuation(initial); err != nil {
		return nil, err
	}
	current := initial
	current.Messages = slices.Clone(initial.Messages)
	current.Tools = slices.Clone(initial.Tools)
	state := loopState{current: current, messages: []AgentMessage{}}
	return runLoopLifecycle(ctx, nil, config, sink, &state)
}

func validateContinuation(context AgentContext) error {
	if len(context.Messages) == 0 {
		return &ContinuationError{Cause: ErrEmptyContinuationContext}
	}
	last := context.Messages[len(context.Messages)-1]
	switch last.(type) {
	case AssistantMessage, *AssistantMessage:
		return &ContinuationError{Cause: ErrAssistantContinuation}
	default:
		return nil
	}
}
