package agent

import (
	"testing"

	"github.com/nigelpepper/attractor/internal/llm"
)

func TestHealOrphanToolPairs(t *testing.T) {
	// A tool_result with no preceding tool_use is an orphan and must be dropped.
	orphanResult := llm.ToolResultMessage("call_x", "result", false)
	user := llm.UserMessage("hello")
	healed := healOrphanToolPairs([]llm.Message{orphanResult, user})
	if len(healed) != 1 || healed[0].Role != llm.RoleUser {
		t.Fatalf("expected orphan tool_result dropped, got %d messages", len(healed))
	}

	// A matched tool_use/tool_result pair survives.
	asst := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentPart{
		llm.ToolCallPart("call_y", "shell", map[string]any{"command": "ls"}),
	}}
	result := llm.ToolResultMessage("call_y", "ok", false)
	healed = healOrphanToolPairs([]llm.Message{asst, result})
	if len(healed) != 2 {
		t.Fatalf("matched pair should survive, got %d", len(healed))
	}
}

func TestEstimateTokens(t *testing.T) {
	msgs := []llm.Message{llm.UserMessage("abcd")} // 4 chars ~ 1 token
	if got := estimateTokens(msgs); got != 1 {
		t.Errorf("estimateTokens = %d, want 1", got)
	}
}

func TestLoopDetector(t *testing.T) {
	d := NewLoopDetector(20, 3)
	args := map[string]any{"command": "ls"}
	var last LoopDetection
	for i := 0; i < 3; i++ {
		last = d.Record("shell", args)
	}
	if !last.IsLooping {
		t.Error("expected loop detection after 3 identical calls")
	}
}
