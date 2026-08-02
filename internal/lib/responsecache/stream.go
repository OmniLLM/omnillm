package responsecache

import (
	"omnillm/internal/cif"
)

// StreamAccumulator rebuilds a CanonicalResponse from a stream of CIF events so
// a streaming response can be stored in the same shape as a non-streaming one.
// This makes the cache both wire-shape-agnostic AND stream-agnostic: an entry
// populated from a streaming call can serve a non-streaming request and vice
// versa, because everything collapses to a CanonicalResponse.
//
// Usage: feed every event through Observe; after the stream ends cleanly, call
// Response() to get the assembled CanonicalResponse (nil if the stream errored
// or produced nothing cacheable).
type StreamAccumulator struct {
	id         string
	model      string
	stopReason cif.CIFStopReason
	stopSeq    *string
	usage      *cif.CIFUsage
	errored    bool
	ended      bool

	blocks      []*blockAccum
	openByIndex map[int]*blockAccum
}

type blockKind uint8

const (
	blockText blockKind = iota + 1
	blockThinking
	blockTool
)

type blockAccum struct {
	kind      blockKind
	text      string
	signature *string
	tool      toolAccum
}

type toolAccum struct {
	id   string
	name string
	args map[string]interface{}

	rawArgs string
}

// NewStreamAccumulator returns a ready accumulator.
func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{openByIndex: make(map[int]*blockAccum)}
}

func (a *StreamAccumulator) openBlock(idx int, kind blockKind) *blockAccum {
	if block := a.openByIndex[idx]; block != nil && block.kind == kind {
		return block
	}
	block := &blockAccum{kind: kind}
	a.blocks = append(a.blocks, block)
	a.openByIndex[idx] = block
	return block
}

// Observe consumes one CIF stream event.
func (a *StreamAccumulator) Observe(event cif.CIFStreamEvent) {
	switch e := event.(type) {
	case cif.CIFStreamStart:
		a.id = e.ID
		a.model = e.Model

	case cif.CIFStreamError:
		a.errored = true

	case cif.CIFContentDelta:
		a.observeDelta(e)

	case cif.CIFStreamEnd:
		a.ended = true
		a.stopReason = e.StopReason
		a.stopSeq = e.StopSequence
		a.usage = e.Usage
	}
}

func (a *StreamAccumulator) observeDelta(e cif.CIFContentDelta) {
	idx := e.Index
	// Some upstreams re-attach ContentBlock on every delta. Reuse an open block
	// of the same kind so repeated announcements do not reset accumulated data.
	if e.ContentBlock != nil {
		switch cb := e.ContentBlock.(type) {
		case cif.CIFToolCallPart:
			block := a.openBlock(idx, blockTool)
			block.tool.id = cb.ToolCallID
			block.tool.name = cb.ToolName
			// Providers announce a tool call with an empty-but-non-nil
			// ToolArguments map and stream the real arguments as deltas.
			if len(cb.ToolArguments) > 0 {
				block.tool.args = cb.ToolArguments
			}
		case cif.CIFTextPart:
			block := a.openBlock(idx, blockText)
			if block.text == "" {
				block.text = cb.Text
			}
		case cif.CIFThinkingPart:
			block := a.openBlock(idx, blockThinking)
			if block.text == "" {
				block.text = cb.Thinking
			}
			if cb.Signature != nil {
				block.signature = cb.Signature
			}
		}
	}

	switch d := e.Delta.(type) {
	case cif.TextDelta:
		a.openBlock(idx, blockText).text += d.Text
	case cif.ThinkingDelta:
		a.openBlock(idx, blockThinking).text += d.Thinking
	case cif.ToolArgumentsDelta:
		a.openBlock(idx, blockTool).tool.rawArgs += d.PartialJSON
	}
}

// Response assembles the accumulated CanonicalResponse, or nil if the stream
// errored, never ended, or produced no content.
func (a *StreamAccumulator) Response() *cif.CanonicalResponse {
	if a.errored || !a.ended {
		return nil
	}
	resp := &cif.CanonicalResponse{
		ID:           a.id,
		Model:        a.model,
		StopReason:   a.stopReason,
		StopSequence: a.stopSeq,
		Usage:        a.usage,
	}
	for _, block := range a.blocks {
		switch block.kind {
		case blockText:
			resp.Content = append(resp.Content, cif.CIFTextPart{Type: "text", Text: block.text})
		case blockThinking:
			resp.Content = append(resp.Content, cif.CIFThinkingPart{
				Type: "thinking", Thinking: block.text, Signature: block.signature,
			})
		case blockTool:
			args := block.tool.args
			if block.tool.rawArgs != "" {
				args = decodeToolArgs(block.tool.rawArgs)
			}
			resp.Content = append(resp.Content, cif.CIFToolCallPart{
				Type:          "tool_call",
				ToolCallID:    block.tool.id,
				ToolName:      block.tool.name,
				ToolArguments: args,
			})
		}
	}
	if len(resp.Content) == 0 {
		return nil
	}
	return resp
}

// SynthesizeStream turns a stored CanonicalResponse back into an ordered slice
// of CIF stream events, ready to be fed through the existing
// ConvertCIFEventToOpenAISSE / ConvertCIFEventToAnthropicSSE serializers. This
// lets a cache hit replay as a normal SSE stream with zero shape-specific code
// in the cache layer.
func SynthesizeStream(resp *cif.CanonicalResponse) []cif.CIFStreamEvent {
	events := []cif.CIFStreamEvent{
		cif.CIFStreamStart{Type: "stream_start", ID: resp.ID, Model: resp.Model},
	}
	for i, part := range resp.Content {
		switch p := part.(type) {
		case cif.CIFTextPart:
			events = append(events,
				cif.CIFContentDelta{
					Type: "content_delta", Index: i,
					ContentBlock: cif.CIFTextPart{Type: "text"},
					Delta:        cif.TextDelta{Type: "text_delta", Text: p.Text},
				},
				cif.CIFContentBlockStop{Type: "content_block_stop", Index: i},
			)
		case cif.CIFThinkingPart:
			events = append(events,
				cif.CIFContentDelta{
					Type: "content_delta", Index: i,
					ContentBlock: cif.CIFThinkingPart{Type: "thinking", Signature: p.Signature},
					Delta:        cif.ThinkingDelta{Type: "thinking_delta", Thinking: p.Thinking},
				},
				cif.CIFContentBlockStop{Type: "content_block_stop", Index: i},
			)
		case cif.CIFToolCallPart:
			events = append(events,
				cif.CIFContentDelta{
					Type: "content_delta", Index: i,
					ContentBlock: cif.CIFToolCallPart{Type: "tool_call", ToolCallID: p.ToolCallID, ToolName: p.ToolName},
					Delta:        cif.ToolArgumentsDelta{Type: "tool_arguments_delta", PartialJSON: encodeToolArgs(p.ToolArguments)},
				},
				cif.CIFContentBlockStop{Type: "content_block_stop", Index: i},
			)
		}
	}
	events = append(events, cif.CIFStreamEnd{
		Type:         "stream_end",
		StopReason:   resp.StopReason,
		StopSequence: resp.StopSequence,
		Usage:        resp.Usage,
	})
	return events
}
