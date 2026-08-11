package agent

// AgentEventKind 标识 Agent Loop 中发生的生命周期事件。
type AgentEventKind string

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

type AgentStartEvent struct{}

func (AgentStartEvent) isAgentEvent()        {}
func (AgentStartEvent) Kind() AgentEventKind { return AgentEventAgentStart }

type AgentEndEvent struct{ Messages []AgentMessage }

func (AgentEndEvent) isAgentEvent()        {}
func (AgentEndEvent) Kind() AgentEventKind { return AgentEventAgentEnd }

type AgentTurnStartEvent struct{}

func (AgentTurnStartEvent) isAgentEvent()        {}
func (AgentTurnStartEvent) Kind() AgentEventKind { return AgentEventTurnStart }

type AgentTurnEndEvent struct {
	Message     AssistantMessage
	ToolResults []ToolResultMessage
}

func (AgentTurnEndEvent) isAgentEvent()        {}
func (AgentTurnEndEvent) Kind() AgentEventKind { return AgentEventTurnEnd }

type AgentMessageStartEvent struct{ Message AgentMessage }

func (AgentMessageStartEvent) isAgentEvent()        {}
func (AgentMessageStartEvent) Kind() AgentEventKind { return AgentEventMessageStart }

type AgentMessageUpdateEvent struct {
	Message        AssistantMessage
	AssistantEvent AssistantMessageEvent
}

func (AgentMessageUpdateEvent) isAgentEvent()        {}
func (AgentMessageUpdateEvent) Kind() AgentEventKind { return AgentEventMessageUpdate }

type AgentMessageEndEvent struct{ Message AgentMessage }

func (AgentMessageEndEvent) isAgentEvent()        {}
func (AgentMessageEndEvent) Kind() AgentEventKind { return AgentEventMessageEnd }

type AgentToolExecutionStartEvent struct {
	ToolCallID string
	ToolName   string
	Arguments  map[string]any
}

func (AgentToolExecutionStartEvent) isAgentEvent() {}
func (AgentToolExecutionStartEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionStart
}

type AgentToolExecutionUpdateEvent struct {
	ToolCallID    string
	ToolName      string
	Arguments     map[string]any
	PartialResult ToolResult
}

func (AgentToolExecutionUpdateEvent) isAgentEvent() {}
func (AgentToolExecutionUpdateEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionUpdate
}

type AgentToolExecutionEndEvent struct {
	ToolCallID string
	ToolName   string
	Result     ToolResult
	IsError    bool
}

func (AgentToolExecutionEndEvent) isAgentEvent() {}
func (AgentToolExecutionEndEvent) Kind() AgentEventKind {
	return AgentEventToolExecutionEnd
}
