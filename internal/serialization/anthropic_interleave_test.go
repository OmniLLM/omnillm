package serialization

import (
	"encoding/json"
	"testing"

	"omnillm/internal/cif"
)

// collectAnthropicSSE runs events through the converter and returns, for each
// emitted Anthropic block index, the tool name it was started with and the
// concatenated partial_json it accumulated.
type anthropicBlock struct {
	Name    string
	ID      string
	Partial string
	Text    string
	Stopped bool
}

func collectAnthropicSSE(t *testing.T, state *AnthropicStreamState, events ...cif.CIFStreamEvent) map[int]*anthropicBlock {
	t.Helper()
	blocks := map[int]*anthropicBlock{}
	get := func(i int) *anthropicBlock {
		if blocks[i] == nil {
			blocks[i] = &anthropicBlock{}
		}
		return blocks[i]
	}
	for _, e := range events {
		out, err := ConvertCIFEventToAnthropicSSE(e, state)
		if err != nil {
			t.Fatalf("convert: %v", err)
		}
		for _, ev := range out {
			idxF, _ := ev["index"].(int)
			switch ev["type"] {
			case "content_block_start":
				cb, _ := ev["content_block"].(map[string]interface{})
				b := get(idxF)
				b.Name, _ = cb["name"].(string)
				b.ID, _ = cb["id"].(string)
			case "content_block_delta":
				d, _ := ev["delta"].(map[string]interface{})
				b := get(idxF)
				if pj, ok := d["partial_json"].(string); ok {
					b.Partial += pj
				}
				if tx, ok := d["text"].(string); ok {
					b.Text += tx
				}
			case "content_block_stop":
				get(idxF).Stopped = true
			}
		}
	}
	return blocks
}

func toolCallBlock(id, name string) cif.CIFToolCallPart {
	return cif.CIFToolCallPart{Type: "tool_call", ToolCallID: id, ToolName: name}
}

// Two concurrent tool calls stream their argument deltas interleaved. Each
// tool's arguments must land in its own block and parse as valid JSON.
// Regression: a single "currently open block" cursor routed every delta to
// whichever block was opened last, so one tool received both argument streams
// concatenated (unparseable) and the other received nothing.
func TestAnthropicSSE_InterleavedToolCallArgumentsStaySeparate(t *testing.T) {
	state := CreateAnthropicStreamState()
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("a", "Read"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("b", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: ""}},
		// Interleaved continuation deltas, no ContentBlock attached.
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"path":`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"/a"}`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"ls"}`}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)

	if len(blocks) != 2 {
		t.Fatalf("expected exactly 2 content blocks, got %d: %+v", len(blocks), blocks)
	}

	want := map[string]map[string]interface{}{
		"Read": {"path": "/a"},
		"Bash": {"command": "ls"},
	}
	seen := map[string]bool{}
	for idx, b := range blocks {
		expected, ok := want[b.Name]
		if !ok {
			t.Fatalf("block %d has unexpected tool name %q", idx, b.Name)
		}
		seen[b.Name] = true

		var got map[string]interface{}
		if err := json.Unmarshal([]byte(b.Partial), &got); err != nil {
			t.Fatalf("block %d (%s) arguments are not valid JSON: %q (%v)", idx, b.Name, b.Partial, err)
		}
		for k, v := range expected {
			if got[k] != v {
				t.Errorf("block %d (%s) arguments[%q] = %v, want %v (full: %q)", idx, b.Name, k, got[k], v, b.Partial)
			}
		}
		if len(got) != len(expected) {
			t.Errorf("block %d (%s) got %d args, want %d: %q", idx, b.Name, len(got), len(expected), b.Partial)
		}
		if !b.Stopped {
			t.Errorf("block %d (%s) was never closed with content_block_stop", idx, b.Name)
		}
	}
	if len(seen) != 2 {
		t.Errorf("expected both Read and Bash blocks, saw %v", seen)
	}
}

// Some providers (Copilot) re-attach the ContentBlock to EVERY delta rather
// than only the first. Re-announcing on each one shredded a single tool call
// into N blocks, each holding an unparseable JSON fragment.
func TestAnthropicSSE_ReannouncedContentBlockDoesNotSplitToolCall(t *testing.T) {
	state := CreateAnthropicStreamState()
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("a", "Read"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"pa`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("b", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"co`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("a", "Read"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `th":"/a"}`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("b", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `mmand":"ls"}`}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)

	if len(blocks) != 2 {
		t.Fatalf("re-announced blocks split the stream into %d blocks, want 2: %+v", len(blocks), blocks)
	}
	for idx, b := range blocks {
		var got map[string]interface{}
		if err := json.Unmarshal([]byte(b.Partial), &got); err != nil {
			t.Errorf("block %d (%s) arguments are not valid JSON: %q", idx, b.Name, b.Partial)
		}
	}
}

// A text block and a tool call interleaving must not cross-contaminate.
func TestAnthropicSSE_TextAndToolCallInterleaveStaySeparate(t *testing.T) {
	state := CreateAnthropicStreamState()
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: cif.CIFTextPart{Type: "text"},
			Delta:        cif.TextDelta{Type: "text_delta", Text: "Let me "}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("a", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			Delta: cif.TextDelta{Type: "text_delta", Text: "check."}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			Delta: cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `"ls"}`}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d: %+v", len(blocks), blocks)
	}
	var text, tool *anthropicBlock
	for _, b := range blocks {
		if b.Name == "Bash" {
			tool = b
		} else {
			text = b
		}
	}
	if text == nil || tool == nil {
		t.Fatalf("missing text or tool block: %+v", blocks)
	}
	if text.Text != "Let me check." {
		t.Errorf("text block = %q, want %q", text.Text, "Let me check.")
	}
	if text.Partial != "" {
		t.Errorf("text block leaked tool arguments: %q", text.Partial)
	}
	if tool.Partial != `{"command":"ls"}` {
		t.Errorf("tool block arguments = %q, want %q", tool.Partial, `{"command":"ls"}`)
	}
}

// All open blocks must be closed when the stream ends, not just the last one.
func TestAnthropicSSE_StreamEndClosesEveryOpenBlock(t *testing.T) {
	state := CreateAnthropicStreamState()
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("a", "Read"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"path":"/a"}`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("b", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":"ls"}`}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)
	for idx, b := range blocks {
		if !b.Stopped {
			t.Errorf("block %d (%s) not closed at stream end", idx, b.Name)
		}
	}
	if state.ContentBlockOpen {
		t.Error("state still reports an open content block after stream end")
	}
}

// FinalizeAnthropicStream must close every open block on a truncated stream.
func TestFinalizeAnthropicStreamClosesAllOpenBlocks(t *testing.T) {
	state := CreateAnthropicStreamState()
	collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("a", "Read"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"pa`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("b", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"co`}},
	)

	events := FinalizeAnthropicStream(state, cif.StopReasonEndTurn)
	stops := 0
	for _, e := range events {
		if e["type"] == "content_block_stop" {
			stops++
		}
	}
	if stops != 2 {
		t.Errorf("finalize emitted %d content_block_stop events, want 2 (one per open block)", stops)
	}
	if state.ContentBlockOpen {
		t.Error("blocks still open after finalize")
	}
}

func TestAnthropicSSE_ReusedProviderIndexStartsNewBlock(t *testing.T) {
	state := CreateAnthropicStreamState()
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: toolCallBlock("a", "Bash"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"command":"ls"}`}},
		cif.CIFContentDelta{Type: "content_delta", Index: 1,
			ContentBlock: cif.CIFTextPart{Type: "text"},
			Delta:        cif.TextDelta{Type: "text_delta", Text: "done"}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonEndTurn},
	)

	if len(blocks) != 2 {
		t.Fatalf("expected separate tool and text blocks, got %+v", blocks)
	}
	if blocks[0].Name != "Bash" || blocks[0].Partial != `{"command":"ls"}` || !blocks[0].Stopped {
		t.Errorf("tool block = %+v", blocks[0])
	}
	if blocks[1].Text != "done" || !blocks[1].Stopped {
		t.Errorf("text block = %+v", blocks[1])
	}
}

func TestAnthropicSSE_SuppressedThinkingIndexDoesNotHideToolCall(t *testing.T) {
	state := CreateAnthropicStreamState()
	state.SuppressThinkingBlocks = true
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: -1,
			ContentBlock: cif.CIFThinkingPart{Type: "thinking"},
			Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: "reasoning"}},
		cif.CIFContentDelta{Type: "content_delta", Index: 0,
			ContentBlock: toolCallBlock("slack-1", "mcp__slackBlizzard__slack_search_public_and_private"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"query":"cn_it_production"}`}},
		cif.CIFContentBlockStop{Type: "content_block_stop", Index: 0},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)

	if len(blocks) != 1 {
		t.Fatalf("expected only the tool block after suppressing thinking, got %d: %+v", len(blocks), blocks)
	}
	block := blocks[0]
	if block == nil {
		t.Fatalf("expected tool block at Anthropic index 0, got %+v", blocks)
	}
	if block.Name != "mcp__slackBlizzard__slack_search_public_and_private" {
		t.Fatalf("tool name = %q", block.Name)
	}
	if block.Partial != `{"query":"cn_it_production"}` {
		t.Errorf("tool arguments = %q", block.Partial)
	}
	if !block.Stopped {
		t.Error("tool block was not closed")
	}
}

func TestAnthropicSSE_NegativeToolIndexWithoutThinkingIsNotSuppressed(t *testing.T) {
	state := CreateAnthropicStreamState()
	state.SuppressThinkingBlocks = true
	blocks := collectAnthropicSSE(t, state,
		cif.CIFStreamStart{Type: "stream_start", ID: "msg", Model: "m"},
		cif.CIFContentDelta{Type: "content_delta", Index: -1,
			ContentBlock: toolCallBlock("slack-1", "mcp__slackBlizzard__slack_search_channels"),
			Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: `{"query":"cn_it_production"}`}},
		cif.CIFStreamEnd{Type: "stream_end", StopReason: cif.StopReasonToolUse},
	)

	if len(blocks) != 1 || blocks[0] == nil {
		t.Fatalf("negative provider index was mistaken for suppressed thinking: %+v", blocks)
	}
	if blocks[0].Partial != `{"query":"cn_it_production"}` {
		t.Errorf("tool arguments = %q", blocks[0].Partial)
	}
}
