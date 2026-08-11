package agent

// Abort 幂等取消当前 run；idle 时不执行任何操作。
func (agent *Agent) Abort() error {
	_, err := agent.execute(agentStateCommand{operation: agentStateAbort})
	return err
}

// Reset 在 idle 时清空 transcript 和运行态，并保留 Agent 配置。
func (agent *Agent) Reset() error {
	_, err := agent.execute(agentStateCommand{operation: agentStateReset})
	return err
}
