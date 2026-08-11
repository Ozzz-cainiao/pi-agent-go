package agent

// UsageCost 表示一次模型响应产生的费用。
type UsageCost struct {
	Input      float64
	Output     float64
	CacheRead  float64
	CacheWrite float64
	Total      float64
}

// Usage 表示一次模型响应的 token 使用量和费用。
type Usage struct {
	InputTokens        int64
	OutputTokens       int64
	CacheReadTokens    int64
	CacheWriteTokens   int64
	CacheWrite1HTokens *int64
	ReasoningTokens    *int64
	TotalTokens        int64
	Cost               UsageCost
}
