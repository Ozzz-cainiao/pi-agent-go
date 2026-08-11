package agent

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
