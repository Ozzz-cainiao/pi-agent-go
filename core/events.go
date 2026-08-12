package agent

import (
	"errors"
	"fmt"
	"reflect"
	"sync"
)

// AgentEventKind 标识 Agent Loop 中发生的生命周期事件。
type AgentEventKind string

// AgentEventKind 的取值覆盖 Agent、turn、message 与 tool 生命周期。
const (
	AgentEventAgentStart          AgentEventKind = "agent_start"
	AgentEventAgentEnd            AgentEventKind = "agent_end"
	AgentEventTurnStart           AgentEventKind = "turn_start"
	AgentEventTurnEnd             AgentEventKind = "turn_end"
	AgentEventMessageStart        AgentEventKind = "message_start"
	AgentEventMessageUpdate       AgentEventKind = "message_update"
	AgentEventMessageEnd          AgentEventKind = "message_end"
	AgentEventToolExecutionStart  AgentEventKind = "tool_execution_start"
	AgentEventToolExecutionUpdate AgentEventKind = "tool_execution_update"
	AgentEventToolExecutionEnd    AgentEventKind = "tool_execution_end"
)

// AgentEvent 是 Agent Loop 对外发送的封闭生命周期事件集合。
type AgentEvent interface {
	isAgentEvent()
	Kind() AgentEventKind
	snapshot() (AgentEvent, error)
}

// AgentEventSink 按产生顺序接收生命周期事件。
//
// 云端 Harness 可在这里把增量转发给 gRPC Gateway，也可以在这里记录指标；Sink 不应
// 修改 Agent 决策。Core 会在调用 Sink 前创建快照，并在并行工具场景下串行化交付。
type AgentEventSink func(AgentEvent) error

// AgentStartEvent 表示一次 Agent Loop 已开始。
type AgentStartEvent struct{}

func (AgentStartEvent) isAgentEvent() {}

// Kind 返回 agent_start。
func (AgentStartEvent) Kind() AgentEventKind { return AgentEventAgentStart }

// AgentEndEvent 表示 Agent Loop 已结束，并携带最终新增消息。
type AgentEndEvent struct{ Messages []AgentMessage }

func (AgentEndEvent) isAgentEvent() {}

// Kind 返回 agent_end。
func (AgentEndEvent) Kind() AgentEventKind { return AgentEventAgentEnd }

// AgentTurnStartEvent 表示新一轮模型调用已开始。
type AgentTurnStartEvent struct{}

func (AgentTurnStartEvent) isAgentEvent() {}

// Kind 返回 turn_start。
func (AgentTurnStartEvent) Kind() AgentEventKind { return AgentEventTurnStart }

// AgentTurnEndEvent 表示当前轮次及其工具结果已经结束。
type AgentTurnEndEvent struct {
	Message     AssistantMessage
	ToolResults []ToolResultMessage
}

func (AgentTurnEndEvent) isAgentEvent() {}

// Kind 返回 turn_end。
func (AgentTurnEndEvent) Kind() AgentEventKind { return AgentEventTurnEnd }

// AgentMessageStartEvent 表示一条输入或输出消息开始处理。
type AgentMessageStartEvent struct{ Message AgentMessage }

func (AgentMessageStartEvent) isAgentEvent() {}

// Kind 返回 message_start。
func (AgentMessageStartEvent) Kind() AgentEventKind { return AgentEventMessageStart }

// AgentMessageUpdateEvent 携带正在流式构建的 Assistant 消息快照。
type AgentMessageUpdateEvent struct {
	Message        AssistantMessage
	AssistantEvent AssistantMessageEvent
}

func (AgentMessageUpdateEvent) isAgentEvent() {}

// Kind 返回 message_update。
func (AgentMessageUpdateEvent) Kind() AgentEventKind { return AgentEventMessageUpdate }

// AgentMessageEndEvent 表示一条消息已经完整写入 transcript。
type AgentMessageEndEvent struct{ Message AgentMessage }

func (AgentMessageEndEvent) isAgentEvent() {}

// Kind 返回 message_end。
func (AgentMessageEndEvent) Kind() AgentEventKind { return AgentEventMessageEnd }

// AgentToolExecutionStartEvent 表示一次工具执行已经开始。
type AgentToolExecutionStartEvent struct {
	ToolCallID string
	ToolName   string
	Arguments  map[string]any
}

func (AgentToolExecutionStartEvent) isAgentEvent() {}

// Kind 返回 tool_execution_start。
func (AgentToolExecutionStartEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionStart
}

// AgentToolExecutionUpdateEvent 携带工具执行期间的部分结果。
type AgentToolExecutionUpdateEvent struct {
	ToolCallID    string
	ToolName      string
	Arguments     map[string]any
	PartialResult ToolResult
}

func (AgentToolExecutionUpdateEvent) isAgentEvent() {}

// Kind 返回 tool_execution_update。
func (AgentToolExecutionUpdateEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionUpdate
}

// AgentToolExecutionEndEvent 表示工具执行已经完成或失败。
type AgentToolExecutionEndEvent struct {
	ToolCallID string
	ToolName   string
	Result     ToolResult
	IsError    bool
}

func (AgentToolExecutionEndEvent) isAgentEvent() {}

// Kind 返回 tool_execution_end。
func (AgentToolExecutionEndEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionEnd
}

// ErrAgentEventSink 表示生命周期事件无法交付或创建安全快照。
var ErrAgentEventSink = errors.New("agent: event sink failed")

// AgentEventSinkError 描述事件快照或消费者处理失败。
type AgentEventSinkError struct {
	Kind  AgentEventKind
	Cause error
}

func (sinkError *AgentEventSinkError) Error() string {
	return fmt.Sprintf("agent: emit %s event: %v", sinkError.Kind, sinkError.Cause)
}

func (sinkError *AgentEventSinkError) Unwrap() []error {
	return []error{ErrAgentEventSink, sinkError.Cause}
}

type agentEventEmitter struct {
	sink  AgentEventSink
	mutex *sync.Mutex
}

func newAgentEventEmitter(sink AgentEventSink) agentEventEmitter {
	return agentEventEmitter{sink: sink, mutex: &sync.Mutex{}}
}

func (emitter agentEventEmitter) emit(event AgentEvent) error {
	if emitter.sink == nil {
		return nil
	}
	// 多个 Tool worker 可能同时产生 update/end；互斥锁保证 Sink 永远不会被并发调用。
	emitter.mutex.Lock()
	defer emitter.mutex.Unlock()
	snapshot, err := event.snapshot()
	if err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	if err := emitter.sink(snapshot); err != nil {
		return &AgentEventSinkError{Kind: event.Kind(), Cause: err}
	}
	return nil
}

func (event AgentStartEvent) snapshot() (AgentEvent, error)     { return event, nil }
func (event AgentTurnStartEvent) snapshot() (AgentEvent, error) { return event, nil }

func (event AgentEndEvent) snapshot() (AgentEvent, error) {
	messages, err := cloneAgentMessages(event.Messages)
	if err != nil {
		return nil, err
	}
	event.Messages = messages
	return event, nil
}

func (event AgentTurnEndEvent) snapshot() (AgentEvent, error) {
	cloner := newSnapshotCloner()
	message, err := cloner.cloneAssistantMessage(event.Message)
	if err != nil {
		return nil, err
	}
	results := make([]ToolResultMessage, len(event.ToolResults))
	for index, result := range event.ToolResults {
		results[index], err = cloner.cloneToolResultMessage(result)
		if err != nil {
			return nil, err
		}
	}
	event.Message = message
	event.ToolResults = results
	return event, nil
}

func (event AgentMessageStartEvent) snapshot() (AgentEvent, error) {
	message, err := newSnapshotCloner().cloneAgentMessage(event.Message)
	if err != nil {
		return nil, err
	}
	event.Message = message
	return event, nil
}

func (event AgentMessageUpdateEvent) snapshot() (AgentEvent, error) {
	message, err := cloneAssistantMessage(event.Message)
	if err != nil {
		return nil, err
	}
	assistantEvent, err := event.AssistantEvent.snapshot()
	if err != nil {
		return nil, err
	}
	event.Message = message
	event.AssistantEvent = assistantEvent
	return event, nil
}

func (event AgentMessageEndEvent) snapshot() (AgentEvent, error) {
	message, err := newSnapshotCloner().cloneAgentMessage(event.Message)
	if err != nil {
		return nil, err
	}
	event.Message = message
	return event, nil
}

func (event AgentToolExecutionStartEvent) snapshot() (AgentEvent, error) {
	arguments, err := cloneArguments(event.Arguments)
	if err != nil {
		return nil, err
	}
	event.Arguments = arguments
	return event, nil
}

func (event AgentToolExecutionUpdateEvent) snapshot() (AgentEvent, error) {
	cloner := newSnapshotCloner()
	arguments, err := cloneArguments(event.Arguments)
	if err != nil {
		return nil, err
	}
	result, err := cloner.cloneToolResult(event.PartialResult)
	if err != nil {
		return nil, err
	}
	event.Arguments = arguments
	event.PartialResult = result
	return event, nil
}

func (event AgentToolExecutionEndEvent) snapshot() (AgentEvent, error) {
	result, err := newSnapshotCloner().cloneToolResult(event.Result)
	if err != nil {
		return nil, err
	}
	event.Result = result
	return event, nil
}

func cloneAgentMessages(messages []AgentMessage) ([]AgentMessage, error) {
	cloner := newSnapshotCloner()
	cloned := make([]AgentMessage, len(messages))
	for index, message := range messages {
		var err error
		cloned[index], err = cloner.cloneAgentMessage(message)
		if err != nil {
			return nil, err
		}
	}
	return cloned, nil
}

func cloneArguments(arguments map[string]any) (map[string]any, error) {
	if arguments == nil {
		return map[string]any{}, nil
	}
	value, err := newSnapshotCloner().cloneValue(
		reflect.ValueOf(arguments),
		"ToolCall.Arguments",
	)
	if err != nil {
		return nil, err
	}
	cloned, ok := value.Interface().(map[string]any)
	if !ok {
		return nil, newSnapshotCloneError("ToolCall.Arguments", value.Type())
	}
	return cloned, nil
}
