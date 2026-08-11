package agent

import (
	"context"
	"errors"
	"fmt"
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

// LoopConfig 集中声明 Agent Loop 的依赖与安全限制。
type LoopConfig struct {
	Stream            StreamFunc
	TransformContext  TransformContextFunc
	ConvertToLLM      ConvertToLLMFunc
	PrepareNextTurn   PrepareNextTurnFunc
	ArgumentValidator ArgumentValidator
	Clock             Clock
	MaxTurns          int
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
	stream            StreamFunc
	transformContext  TransformContextFunc
	convertToLLM      ConvertToLLMFunc
	prepareNextTurn   PrepareNextTurnFunc
	argumentValidator ArgumentValidator
	clock             Clock
	maxTurns          int
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
	clock := config.Clock
	if clock == nil {
		clock = Clock(time.Now)
	}
	convertToLLM := config.ConvertToLLM
	if convertToLLM == nil {
		convertToLLM = defaultConvertToLLM
	}
	argumentValidator := config.ArgumentValidator
	if argumentValidator == nil {
		argumentValidator = JSONSchemaArgumentValidator{}
	}

	return loopRuntime{
		stream:            config.Stream,
		transformContext:  config.TransformContext,
		convertToLLM:      convertToLLM,
		prepareNextTurn:   config.PrepareNextTurn,
		argumentValidator: argumentValidator,
		clock:             clock,
		maxTurns:          maxTurns,
	}, nil
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
