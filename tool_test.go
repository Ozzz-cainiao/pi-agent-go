package agent

import (
	"context"
	"testing"
)

type stubTool struct{}

func (stubTool) Definition() ToolDefinition {
	return ToolDefinition{
		Name:        "qa_search",
		Label:       "QA Search",
		Description: "搜索知识库",
	}
}

func (stubTool) Execute(
	_ context.Context,
	_ ToolCall,
	_ ToolUpdateFunc,
) (ToolResult, error) {
	return ToolResult{
		Content: []ToolResultContent{
			TextContent{Text: "搜索结果"},
		},
	}, nil
}

func TestTool_contract(t *testing.T) {
	// 准备
	var tool Tool = stubTool{}

	// 执行
	definition := tool.Definition()
	result, err := tool.Execute(
		context.Background(),
		ToolCall{
			ID:   "call-1",
			Name: "qa_search",
		},
		nil,
	)

	// 验证
	if err != nil {
		t.Fatalf("Execute() returned error: %v", err)
	}
	if definition.Name != "qa_search" {
		t.Fatalf("Definition().Name = %q, want %q", definition.Name, "qa_search")
	}
	if len(result.Content) != 1 {
		t.Fatalf("result content count = %d, want 1", len(result.Content))
	}
}
