package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrAgentSubscriber 表示 Agent subscriber 执行失败。
	ErrAgentSubscriber = errors.New("agent: subscriber failed")
	// ErrNilAgentSubscriber 表示 Subscribe 收到了 nil subscriber。
	ErrNilAgentSubscriber = errors.New("agent: nil subscriber")
	// ErrAgentBusy 表示 Agent 已有正在执行的 run。
	ErrAgentBusy = errors.New("agent: busy")
)

// AgentSubscriber 按生命周期顺序处理 AgentEvent。
type AgentSubscriber func(context.Context, AgentEvent) error

// Unsubscribe 移除对应的 subscriber；重复调用是安全的。
type Unsubscribe func()

// AgentSubscriberError 保留失败 subscriber 和事件信息。
type AgentSubscriberError struct {
	SubscriberID uint64
	Event        AgentEventKind
	Cause        error
	Panicked     bool
}

// Error 返回 subscriber 错误说明。
func (subscriberError *AgentSubscriberError) Error() string {
	return fmt.Sprintf(
		"agent: subscriber %d failed on %s: %v",
		subscriberError.SubscriberID,
		subscriberError.Event,
		subscriberError.Cause,
	)
}

// Unwrap 同时暴露通用 subscriber 错误和原始错误。
func (subscriberError *AgentSubscriberError) Unwrap() []error {
	return []error{ErrAgentSubscriber, subscriberError.Cause}
}

type agentSubscriberEntry struct {
	id         uint64
	subscriber AgentSubscriber
}

// Subscribe 注册 subscriber，并返回幂等取消函数。
func (agent *Agent) Subscribe(subscriber AgentSubscriber) (Unsubscribe, error) {
	if subscriber == nil {
		return nil, ErrNilAgentSubscriber
	}
	reply, err := agent.execute(agentStateCommand{
		operation:  agentStateSubscribe,
		subscriber: subscriber,
	})
	if err != nil {
		return nil, err
	}
	var once sync.Once
	unsubscribe := func() {
		once.Do(func() {
			_, executeError := agent.execute(agentStateCommand{
				operation:    agentStateUnsubscribe,
				subscriberID: reply.subscriberID,
			})
			if executeError != nil {
				return
			}
		})
	}
	return unsubscribe, nil
}

func (agent *Agent) dispatchEvent(
	ctx context.Context,
	event AgentEvent,
) ([]error, error) {
	reply, err := agent.execute(agentStateCommand{
		operation: agentStateProcessEvent,
		event:     event,
	})
	if err != nil {
		return nil, err
	}
	failures := make([]error, 0)
	for _, entry := range reply.subscribers {
		snapshot, snapshotError := event.snapshot()
		if snapshotError != nil {
			return failures, fmt.Errorf("snapshot subscriber event: %w", snapshotError)
		}
		if subscriberError := invokeSubscriber(ctx, entry, snapshot); subscriberError != nil {
			failures = append(failures, subscriberError)
		}
	}
	return failures, nil
}

func invokeSubscriber(
	ctx context.Context,
	entry agentSubscriberEntry,
	event AgentEvent,
) (failure error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			cause, ok := recovered.(error)
			if !ok {
				cause = fmt.Errorf("panic: %v", recovered)
			}
			failure = &AgentSubscriberError{
				SubscriberID: entry.id,
				Event:        event.Kind(), Cause: cause, Panicked: true,
			}
		}
	}()
	if err := entry.subscriber(ctx, event); err != nil {
		return &AgentSubscriberError{
			SubscriberID: entry.id,
			Event:        event.Kind(), Cause: err,
		}
	}
	return nil
}
