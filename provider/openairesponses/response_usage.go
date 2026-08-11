package openairesponses

import agent "github.com/Ozzz-cainiao/pi-agent-go"

func usageFrom(usage responseUsage) agent.Usage {
	inputTokens := usage.InputTokens - usage.InputDetails.CachedTokens - usage.InputDetails.CacheWriteTokens
	if inputTokens < 0 {
		inputTokens = 0
	}
	return agent.Usage{
		InputTokens:      inputTokens,
		OutputTokens:     usage.OutputTokens,
		CacheReadTokens:  usage.InputDetails.CachedTokens,
		CacheWriteTokens: usage.InputDetails.CacheWriteTokens,
		ReasoningTokens:  cloneInt64(usage.OutputDetails.ReasoningTokens),
		TotalTokens:      usage.TotalTokens,
	}
}
