package agent

import "testing"

func TestFindToolByName_returnsMatchingTool(t *testing.T) {
	// 准备
	tools := []Tool{
		nil,
		stubTool{},
	}

	// 执行
	got, ok := findToolByName(tools, "qa_search")

	// 验证
	if !ok {
		t.Fatal("findToolByName() did not find qa_search")
	}
	if got.Definition().Name != "qa_search" {
		t.Fatalf(
			"tool name = %q, want %q",
			got.Definition().Name,
			"qa_search",
		)
	}
}

func TestFindToolByName_returnsFalseForMissingTool(t *testing.T) {
	// 准备
	tools := []Tool{
		stubTool{},
	}

	// 执行
	got, ok := findToolByName(tools, "rag_search")

	// 验证
	if ok {
		t.Fatalf("findToolByName() found unexpected tool: %#v", got)
	}
	if got != nil {
		t.Fatalf("findToolByName() tool = %#v, want nil", got)
	}
}
