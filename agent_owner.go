package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
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
}

// Agent 通过单一 owner goroutine 串行管理高层状态。
type Agent struct {
	commands   chan agentStateCommand
	done       chan struct{}
	closeOnce  sync.Once
	closeError error
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

// Close 停止 owner goroutine；重复调用是安全的。
func (agent *Agent) Close() error {
	agent.closeOnce.Do(func() {
		_, agent.closeError = agent.execute(agentStateCommand{operation: agentStateClose})
	})
	return agent.closeError
}

func (agent *Agent) execute(command agentStateCommand) (agentStateReply, error) {
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
	defer close(agent.done)
	owned := agentOwnedState{
		state: state, steeringQueue: queues.steering, followUpQueue: queues.followUp,
	}
	for {
		command := <-agent.commands
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
	case agentStateClose:
		return agentStateReply{}, true
	}
	return agentStateReply{}, false
}
