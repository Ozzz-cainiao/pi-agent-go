package agent

// findToolByName 根据模型返回的工具名称查找可用工具。
func findToolByName(tools []Tool, name string) (Tool, bool) {
	for _, tool := range tools {
		if tool == nil {
			continue
		}

		if tool.Definition().Name == name {
			return tool, true
		}
	}

	return nil, false
}
