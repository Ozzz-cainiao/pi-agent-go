package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

// ErrAgentClosed 表示 Agent owner 已停止接收状态操作。
var ErrAgentClosed = errors.New("agent: closed")

type agentStateOperation uint8

const (
	agentStateSnapshot agentStateOperation = iota
	agentStateSetSystemPrompt
	agentStateSetModel
	agentStateSetThinkingLevel
	agentStateSetTools
	agentStateReplaceMessages
	agentStateAppendMessage
	agentStateClearMessages
	agentStateSubscribe
	agentStateUnsubscribe
	agentStateBeginRun
	agentStateProcessEvent
	agentStateFinishRun
	agentStateCurrentIdle
	agentStateAbort
	agentStateReset
	agentStateEnqueueSteering
	agentStateEnqueueFollowUp
	agentStateDrainSteering
	agentStateDrainFollowUp
	agentStateSetSteeringMode
	agentStateSetFollowUpMode
	agentStateHasQueuedMessages
	agentStateBeginClose
	agentStateClose
)

type agentStateCommand struct {
	operation    agentStateOperation
	systemPrompt string
	model        ModelID
	thinking     ThinkingLevel
	tools        []Tool
	messages     []AgentMessage
	subscriber   AgentSubscriber
	subscriberID uint64
	event        AgentEvent
	runDone      chan struct{}
	cancel       context.CancelFunc
	queueMessage AgentMessage
	queueMode    QueueMode
	reply        chan agentStateReply
}

type agentStateReply struct {
	state         AgentState
	subscribers   []agentSubscriberEntry
	subscriberID  uint64
	idle          <-chan struct{}
	queueMessages []AgentMessage
	hasQueued     bool
	err           error
}

type agentOwnedState struct {
	state            AgentState
	subscribers      []agentSubscriberEntry
	nextSubscriberID uint64
	active           *agentActiveRun
	steeringQueue    pendingMessageQueue
	followUpQueue    pendingMessageQueue
	closing          bool
}

// Agent 通过单一 owner goroutine 串行管理高层状态。
//
// 低层 RunAgentLoop 是一次调用内的纯运行器；高层 Agent 则长期持有 transcript、工具、
// steering/follow-up 队列和 subscriber。所有状态读写都变成 agentStateCommand，由一个
// goroutine 串行处理，从结构上避免多个 Gateway 请求同时修改同一 AgentState。
type Agent struct {
	commands   chan agentStateCommand
	done       chan struct{}
	closeOnce  sync.Once
	closeError error
	closing    atomic.Bool
	loopConfig LoopConfig
}

// NewAgent 创建持有独立状态副本的高层 Agent。
func NewAgent(options AgentOptions) (*Agent, error) {
	state, err := initialAgentState(options.InitialState)
	if err != nil {
		return nil, err
	}
	queues, err := newAgentQueues(options.SteeringMode, options.FollowUpMode)
	if err != nil {
		return nil, err
	}
	agent := &Agent{
		commands:   make(chan agentStateCommand),
		done:       make(chan struct{}),
		loopConfig: options.LoopConfig,
	}
	go agent.ownState(state, queues)
	return agent, nil
}

// State 返回不会反向修改内部状态的深层快照。
func (agent *Agent) State() (AgentState, error) {
	reply, err := agent.execute(agentStateCommand{operation: agentStateSnapshot})
	return reply.state, err
}

// SetSystemPrompt 替换后续请求使用的 system prompt。
func (agent *Agent) SetSystemPrompt(prompt string) error {
	_, err := agent.execute(agentStateCommand{operation: agentStateSetSystemPrompt, systemPrompt: prompt})
	return err
}

// SetModel 替换后续请求使用的模型标识。
func (agent *Agent) SetModel(model ModelID) error {
	_, err := agent.execute(agentStateCommand{operation: agentStateSetModel, model: model})
	return err
}

// SetThinkingLevel 替换后续请求使用的推理强度。
func (agent *Agent) SetThinkingLevel(level ThinkingLevel) error {
	_, err := agent.execute(agentStateCommand{operation: agentStateSetThinkingLevel, thinking: level})
	return err
}

// SetTools 替换工具集合，并复制输入 slice。
func (agent *Agent) SetTools(tools []Tool) error {
	copyOfTools := append([]Tool(nil), tools...)
	_, err := agent.execute(agentStateCommand{operation: agentStateSetTools, tools: copyOfTools})
	return err
}

// ReplaceMessages 使用输入消息的深层副本替换 transcript。
func (agent *Agent) ReplaceMessages(messages []AgentMessage) error {
	cloned, err := cloneAgentMessages(messages)
	if err != nil {
		return fmt.Errorf("replace agent messages: %w", err)
	}
	_, err = agent.execute(agentStateCommand{operation: agentStateReplaceMessages, messages: cloned})
	return err
}

// AppendMessage 向 transcript 追加一条消息的深层副本。
func (agent *Agent) AppendMessage(message AgentMessage) error {
	cloned, err := cloneAgentMessages([]AgentMessage{message})
	if err != nil {
		return fmt.Errorf("append agent message: %w", err)
	}
	_, err = agent.execute(agentStateCommand{operation: agentStateAppendMessage, messages: cloned})
	return err
}

// ClearMessages 清空 transcript。
func (agent *Agent) ClearMessages() error {
	_, err := agent.execute(agentStateCommand{operation: agentStateClearMessages})
	return err
}

// Close 幂等发起关闭并取消 active run，但不在 subscriber 调用链中等待。
// 调用方可通过 WaitForIdle 等待 owner 和当前 run 完全结束。
func (agent *Agent) Close() error {
	agent.closeOnce.Do(func() {
		agent.closing.Store(true)
		reply, err := agent.execute(agentStateCommand{operation: agentStateBeginClose})
		if err != nil {
			agent.closeError = err
			return
		}
		go agent.completeClose(reply.idle)
	})
	return agent.closeError
}

func (agent *Agent) completeClose(idle <-chan struct{}) {
	if idle != nil {
		<-idle
	}
	if _, err := agent.execute(agentStateCommand{operation: agentStateClose}); err != nil {
		return
	}
}

func (agent *Agent) execute(command agentStateCommand) (agentStateReply, error) {
	if agent.closing.Load() && !allowedWhileClosing(command.operation) {
		return agentStateReply{}, ErrAgentClosed
	}
	// 每条命令自带一次性 reply channel，调用者同步等待 owner 应用状态变更。
	command.reply = make(chan agentStateReply, 1)
	select {
	case <-agent.done:
		return agentStateReply{}, ErrAgentClosed
	case agent.commands <- command:
	}
	reply := <-command.reply
	return reply, reply.err
}

func (agent *Agent) ownState(state AgentState, queues agentQueues) {
	// 这是 AgentState 的唯一写入者。外部方法只能发送命令，不能直接接触 owned state。
	defer close(agent.done)
	owned := agentOwnedState{
		state: state, steeringQueue: queues.steering, followUpQueue: queues.followUp,
	}
	for {
		command := <-agent.commands
		if owned.closing && !allowedWhileClosing(command.operation) {
			command.reply <- agentStateReply{err: ErrAgentClosed}
			continue
		}
		reply, stop := applyAgentStateCommand(&owned, command)
		command.reply <- reply
		if stop {
			return
		}
	}
}

func applyAgentStateCommand(owned *agentOwnedState, command agentStateCommand) (agentStateReply, bool) {
	switch command.operation {
	case agentStateSnapshot:
		snapshot, err := cloneAgentState(owned.state)
		return agentStateReply{state: snapshot, err: err}, false
	case agentStateSetSystemPrompt:
		owned.state.SystemPrompt = command.systemPrompt
	case agentStateSetModel:
		owned.state.Model = command.model
	case agentStateSetThinkingLevel:
		owned.state.ThinkingLevel = command.thinking
	case agentStateSetTools:
		owned.state.Tools = command.tools
	case agentStateReplaceMessages:
		owned.state.Messages = command.messages
	case agentStateAppendMessage:
		owned.state.Messages = append(owned.state.Messages, command.messages[0])
	case agentStateClearMessages:
		owned.state.Messages = []AgentMessage{}
	case agentStateSubscribe,
		agentStateUnsubscribe,
		agentStateBeginRun,
		agentStateProcessEvent,
		agentStateFinishRun,
		agentStateCurrentIdle,
		agentStateAbort,
		agentStateReset:
		return applyAgentRuntimeCommand(owned, command), false
	case agentStateEnqueueSteering,
		agentStateEnqueueFollowUp,
		agentStateDrainSteering,
		agentStateDrainFollowUp,
		agentStateSetSteeringMode,
		agentStateSetFollowUpMode,
		agentStateHasQueuedMessages:
		return applyAgentQueueCommand(owned, command), false
	case agentStateBeginClose:
		owned.closing = true
		if owned.active != nil {
			owned.active.cancel()
			return agentStateReply{idle: owned.active.done}, false
		}
		return agentStateReply{}, false
	case agentStateClose:
		return agentStateReply{}, true
	}
	return agentStateReply{}, false
}

func allowedWhileClosing(operation agentStateOperation) bool {
	return operation == agentStateProcessEvent ||
		operation == agentStateFinishRun ||
		operation == agentStateCurrentIdle ||
		operation == agentStateBeginClose ||
		operation == agentStateClose
}

type agentActiveRun struct {
	done   chan struct{}
	cancel func()
}

func applyAgentRuntimeCommand(
	owned *agentOwnedState,
	command agentStateCommand,
) agentStateReply {
	if command.operation == agentStateSubscribe {
		owned.nextSubscriberID++
		entry := agentSubscriberEntry{
			id: owned.nextSubscriberID, subscriber: command.subscriber,
		}
		owned.subscribers = append(owned.subscribers, entry)
		return agentStateReply{subscriberID: entry.id}
	}
	if command.operation == agentStateUnsubscribe {
		owned.subscribers = removeSubscriber(owned.subscribers, command.subscriberID)
		return agentStateReply{}
	}
	return applyAgentRunCommand(owned, command)
}

func applyAgentRunCommand(owned *agentOwnedState, command agentStateCommand) agentStateReply {
	if command.operation == agentStateBeginRun {
		return beginAgentRun(owned, command)
	}
	if command.operation == agentStateProcessEvent {
		return processAgentEvent(owned, command.event)
	}
	if command.operation == agentStateFinishRun {
		finishAgentRun(owned)
		return agentStateReply{}
	}
	if command.operation == agentStateCurrentIdle && owned.active != nil {
		return agentStateReply{idle: owned.active.done}
	}
	if command.operation == agentStateAbort && owned.active != nil {
		owned.active.cancel()
		return agentStateReply{}
	}
	if command.operation == agentStateReset {
		if owned.active != nil {
			return agentStateReply{err: ErrAgentBusy}
		}
		resetAgentRuntimeState(&owned.state)
		owned.steeringQueue.messages = nil
		owned.followUpQueue.messages = nil
		return agentStateReply{}
	}
	if command.operation == agentStateCurrentIdle || command.operation == agentStateAbort {
		return agentStateReply{}
	}
	return agentStateReply{err: fmt.Errorf("unsupported runtime state operation: %d", command.operation)}
}

func beginAgentRun(owned *agentOwnedState, command agentStateCommand) agentStateReply {
	if owned.active != nil {
		return agentStateReply{err: ErrAgentBusy}
	}
	snapshot, err := cloneAgentState(owned.state)
	if err != nil {
		return agentStateReply{err: fmt.Errorf("snapshot agent run state: %w", err)}
	}
	owned.active = &agentActiveRun{done: command.runDone, cancel: command.cancel}
	owned.state.IsStreaming = true
	owned.state.StreamingMessage = nil
	owned.state.PendingToolCalls = map[string]struct{}{}
	owned.state.ErrorMessage = ""
	return agentStateReply{state: snapshot}
}

func processAgentEvent(owned *agentOwnedState, event AgentEvent) agentStateReply {
	if err := reduceAgentEvent(&owned.state, event); err != nil {
		return agentStateReply{err: err}
	}
	return agentStateReply{
		subscribers: append([]agentSubscriberEntry(nil), owned.subscribers...),
	}
}

func finishAgentRun(owned *agentOwnedState) {
	owned.state.IsStreaming = false
	owned.state.StreamingMessage = nil
	owned.state.PendingToolCalls = map[string]struct{}{}
	if owned.active != nil {
		close(owned.active.done)
		owned.active = nil
	}
}

func resetAgentRuntimeState(state *AgentState) {
	state.Messages = []AgentMessage{}
	state.IsStreaming = false
	state.StreamingMessage = nil
	state.PendingToolCalls = map[string]struct{}{}
	state.ErrorMessage = ""
}

func removeSubscriber(
	subscribers []agentSubscriberEntry,
	subscriberID uint64,
) []agentSubscriberEntry {
	for index, entry := range subscribers {
		if entry.id == subscriberID {
			return append(subscribers[:index], subscribers[index+1:]...)
		}
	}
	return subscribers
}

func reduceAgentEvent(state *AgentState, event AgentEvent) error {
	snapshot, err := event.snapshot()
	if err != nil {
		return fmt.Errorf("snapshot agent state event: %w", err)
	}
	switch value := snapshot.(type) {
	case AgentStartEvent:
		state.ErrorMessage = ""
	case AgentMessageStartEvent:
		state.StreamingMessage = value.Message
	case AgentMessageUpdateEvent:
		state.StreamingMessage = value.Message
	case AgentMessageEndEvent:
		state.StreamingMessage = nil
		state.Messages = append(state.Messages, value.Message)
	case AgentToolExecutionStartEvent:
		state.PendingToolCalls[value.ToolCallID] = struct{}{}
	case AgentToolExecutionEndEvent:
		delete(state.PendingToolCalls, value.ToolCallID)
	case AgentTurnEndEvent:
		if value.Message.ErrorMessage != "" {
			state.ErrorMessage = value.Message.ErrorMessage
		}
	case AgentEndEvent:
		state.StreamingMessage = nil
	case AgentTurnStartEvent, AgentToolExecutionUpdateEvent:
	}
	return nil
}

type agentRunEventDispatcher struct {
	agent    *Agent
	ctx      context.Context
	failures *[]error
	terminal *AgentEndEvent
}

func (dispatcher *agentRunEventDispatcher) handle(event AgentEvent) error {
	if terminal, ok := event.(AgentEndEvent); ok {
		dispatcher.terminal = &terminal
		return nil
	}
	return dispatcher.publish(event)
}

func (dispatcher *agentRunEventDispatcher) complete(runError error, model ModelID) error {
	if runError != nil {
		failure := dispatcher.agent.newFailureMessage(runError, model)
		return dispatcher.publishAll([]AgentEvent{
			AgentMessageStartEvent{Message: failure},
			AgentMessageEndEvent{Message: failure},
			AgentTurnEndEvent{Message: failure},
			AgentEndEvent{Messages: []AgentMessage{failure}},
		})
	}
	if dispatcher.terminal == nil {
		return dispatcher.publish(AgentEndEvent{})
	}
	return dispatcher.publish(*dispatcher.terminal)
}

func (dispatcher *agentRunEventDispatcher) publishAll(events []AgentEvent) error {
	failures := make([]error, 0)
	for _, event := range events {
		if err := dispatcher.publish(event); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (dispatcher *agentRunEventDispatcher) publish(event AgentEvent) error {
	failures, err := dispatcher.agent.dispatchEvent(dispatcher.ctx, event)
	*dispatcher.failures = append(*dispatcher.failures, failures...)
	return err
}

func (agent *Agent) newFailureMessage(runError error, model ModelID) AssistantMessage {
	reason := StopReasonError
	if errors.Is(runError, context.Canceled) || errors.Is(runError, context.DeadlineExceeded) {
		reason = StopReasonAborted
	}
	return AssistantMessage{
		Content:      []AssistantContent{TextContent{Text: ""}},
		Model:        string(model),
		StopReason:   reason,
		ErrorMessage: runError.Error(),
		Timestamp:    resolvedClock(agent.loopConfig.Clock)().UnixMilli(),
	}
}
