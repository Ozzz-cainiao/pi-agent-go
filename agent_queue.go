package agent

import (
	"context"
	"errors"
	"fmt"
)

// QueueMode 控制每次 drain 消费的消息数量。
type QueueMode string

const (
	// QueueModeAll 在一次 drain 中取出全部排队消息。
	QueueModeAll QueueMode = "all"
	// QueueModeOneAtATime 在一次 drain 中只取出最早的一条消息。
	QueueModeOneAtATime QueueMode = "one-at-a-time"
)

// ErrInvalidQueueMode 表示队列消费模式无法识别。
var ErrInvalidQueueMode = errors.New("agent: invalid queue mode")

type agentQueueKind uint8

const (
	agentSteeringQueue agentQueueKind = iota
	agentFollowUpQueue
)

type pendingMessageQueue struct {
	mode     QueueMode
	messages []AgentMessage
}

type agentQueues struct {
	steering pendingMessageQueue
	followUp pendingMessageQueue
}

func newAgentQueues(steeringMode, followUpMode QueueMode) (agentQueues, error) {
	steering, err := normalizeQueueMode(steeringMode)
	if err != nil {
		return agentQueues{}, fmt.Errorf("steering mode: %w", err)
	}
	followUp, err := normalizeQueueMode(followUpMode)
	if err != nil {
		return agentQueues{}, fmt.Errorf("follow-up mode: %w", err)
	}
	return agentQueues{
		steering: pendingMessageQueue{mode: steering},
		followUp: pendingMessageQueue{mode: followUp},
	}, nil
}

func normalizeQueueMode(mode QueueMode) (QueueMode, error) {
	if mode == "" {
		return QueueModeOneAtATime, nil
	}
	if mode != QueueModeAll && mode != QueueModeOneAtATime {
		return "", fmt.Errorf("%q: %w", mode, ErrInvalidQueueMode)
	}
	return mode, nil
}

// Steer 排队一条在当前工具批次完成后注入的消息。
func (agent *Agent) Steer(message AgentMessage) error {
	return agent.enqueueMessage(agentStateEnqueueSteering, message)
}

// FollowUp 排队一条仅在 Agent 原本将停止时注入的消息。
func (agent *Agent) FollowUp(message AgentMessage) error {
	return agent.enqueueMessage(agentStateEnqueueFollowUp, message)
}

// SetSteeringMode 设置 steering queue 后续采用的 drain 模式。
func (agent *Agent) SetSteeringMode(mode QueueMode) error {
	return agent.setQueueMode(agentStateSetSteeringMode, mode)
}

// SetFollowUpMode 设置 follow-up queue 后续采用的 drain 模式。
func (agent *Agent) SetFollowUpMode(mode QueueMode) error {
	return agent.setQueueMode(agentStateSetFollowUpMode, mode)
}

// HasQueuedMessages 报告任一消息队列是否仍有内容。
func (agent *Agent) HasQueuedMessages() (bool, error) {
	reply, err := agent.execute(agentStateCommand{operation: agentStateHasQueuedMessages})
	return reply.hasQueued, err
}

func (agent *Agent) enqueueMessage(operation agentStateOperation, message AgentMessage) error {
	cloned, err := cloneAgentMessages([]AgentMessage{message})
	if err != nil {
		return fmt.Errorf("clone queued message: %w", err)
	}
	_, err = agent.execute(agentStateCommand{operation: operation, queueMessage: cloned[0]})
	return err
}

func (agent *Agent) setQueueMode(operation agentStateOperation, mode QueueMode) error {
	normalized, err := normalizeQueueMode(mode)
	if err != nil {
		return err
	}
	_, err = agent.execute(agentStateCommand{operation: operation, queueMode: normalized})
	return err
}

func (agent *Agent) drainQueue(kind agentQueueKind) ([]AgentMessage, error) {
	operation := agentStateDrainSteering
	if kind == agentFollowUpQueue {
		operation = agentStateDrainFollowUp
	}
	reply, err := agent.execute(agentStateCommand{operation: operation})
	return reply.queueMessages, err
}

func (agent *Agent) loopConfigForRun(skipInitialSteering bool) LoopConfig {
	config := agent.loopConfig
	baseSteering := config.GetSteeringMessages
	baseFollowUp := config.GetFollowUpMessages
	config.GetSteeringMessages = func(ctx context.Context) ([]AgentMessage, error) {
		if skipInitialSteering {
			skipInitialSteering = false
			return nil, nil
		}
		return agent.collectQueuedMessages(ctx, agentSteeringQueue, baseSteering)
	}
	config.GetFollowUpMessages = func(ctx context.Context) ([]AgentMessage, error) {
		return agent.collectQueuedMessages(ctx, agentFollowUpQueue, baseFollowUp)
	}
	return config
}

func (agent *Agent) collectQueuedMessages(
	ctx context.Context,
	kind agentQueueKind,
	base GetQueuedMessagesFunc,
) ([]AgentMessage, error) {
	var external []AgentMessage
	var err error
	if base != nil {
		external, err = base(ctx)
		if err != nil {
			return nil, err
		}
	}
	queued, err := agent.drainQueue(kind)
	if err != nil {
		return nil, err
	}
	return append(queued, external...), nil
}
