package agenttest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	agent "github.com/Ozzz-cainiao/pi-agent-go"
)

// ErrScriptExhausted 表示 scripted StreamFunc 已没有下一步响应。
var ErrScriptExhausted = errors.New("agenttest: script exhausted")

// StreamStep 描述一次 Model 调用的响应、事件或阻塞行为。
type StreamStep struct {
	Response      agent.AssistantMessage
	Events        []agent.AssistantMessageEvent
	Error         error
	WaitForCancel bool
}

// ScriptedStream 按顺序返回预设响应，并记录每次 Model 上下文。
type ScriptedStream struct {
	mu      sync.Mutex
	steps   []StreamStep
	calls   []agent.AgentContext
	started chan int
	next    int
}

// NewScriptedStream 创建一个可复用的确定性 StreamFunc。
func NewScriptedStream(steps ...StreamStep) *ScriptedStream {
	return &ScriptedStream{
		steps: slices.Clone(steps), started: make(chan int, len(steps)+1),
	}
}

// Stream 实现 agent.StreamFunc。
func (stream *ScriptedStream) Stream(
	ctx context.Context,
	modelContext agent.AgentContext,
	emit agent.AssistantMessageEventSink,
) (agent.AssistantMessage, error) {
	stream.mu.Lock()
	if stream.next >= len(stream.steps) {
		stream.mu.Unlock()
		return agent.AssistantMessage{}, ErrScriptExhausted
	}
	index := stream.next
	step := stream.steps[index]
	stream.next++
	modelContext.Messages = slices.Clone(modelContext.Messages)
	modelContext.Tools = slices.Clone(modelContext.Tools)
	stream.calls = append(stream.calls, modelContext)
	stream.mu.Unlock()
	stream.started <- index

	if step.WaitForCancel {
		<-ctx.Done()
		return agent.AssistantMessage{}, ctx.Err()
	}
	for _, event := range step.Events {
		if emit == nil {
			continue
		}
		if err := emit(event); err != nil {
			return agent.AssistantMessage{}, fmt.Errorf("agenttest emit event: %w", err)
		}
	}
	if step.Error != nil {
		return agent.AssistantMessage{}, step.Error
	}
	return step.Response, nil
}

// Calls 返回已捕获 Model 上下文的 slice 副本。
func (stream *ScriptedStream) Calls() []agent.AgentContext {
	stream.mu.Lock()
	defer stream.mu.Unlock()
	return slices.Clone(stream.calls)
}

// Started 返回每次 Stream 开始时写入步骤索引的 channel。
func (stream *ScriptedStream) Started() <-chan int {
	return stream.started
}
