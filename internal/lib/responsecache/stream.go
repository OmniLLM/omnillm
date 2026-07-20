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

	// Per content-block accumulation, keyed by block index.
	textByBlock     map[int]*string
	thinkingByBlock map[int]*string
	sigByBlock      map[int]*string
	toolByBlock     map[int]*toolAccum
	order           []int // block indices in first-seen order
}

type toolAccum struct {
	id     string
	name   string
	rawArgs string
	args   map[string]interface{}
}

// NewStreamAccumulator returns a ready accumulator.
func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		textByBlock:     map[int]*string{},
		thinkingByBlock: map[int]*string{},
		sigByBlock:      map[int]*string{},
		toolByBlock:     map[int]*toolAccum{},
	}
}

func (a *StreamAccumulator) seen(idx int) {
	if _, ok := a.textByBlock[idx]; ok {
		return
	}
	if _, ok := a.thinkingByBlock[idx]; ok {
		return
	}
	if _, ok := a.toolByBlock[idx]; ok {
		return
	}
	a.order = append(a.order, idx)
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
	// A new block announces its concrete type via ContentBlock. NOTE: some
	// upstreams (e.g. Copilot) attach the ContentBlock on EVERY delta, not just
	// the first, so we must only initialize a block the first time we see its
	// index — never re-seed, or we'd wipe already-accumulated text.
	if e.ContentBlock != nil {
		switch cb := e.ContentBlock.(type) {
		case cif.CIFToolCallPart:
			if _, exists := a.toolByBlock[idx]; !exists {
				a.seen(idx)
				a.toolByBlock[idx] = &toolAccum{id: cb.ToolCallID, name: cb.ToolName, args: cb.ToolArguments}
			}
		case cif.CIFTextPart:
			if _, exists := a.textByBlock[idx]; !exists {
				a.seen(idx)
				s := cb.Text
				a.textByBlock[idx] = &s
			}
		case cif.CIFThinkingPart:
			if _, exists := a.thinkingByBlock[idx]; !exists {
				a.seen(idx)
				s := cb.Thinking
				a.thinkingByBlock[idx] = &s
				if cb.Signature != nil {
					a.sigByBlock[idx] = cb.Signature
				}
			}
		}
	}

	switch d := e.Delta.(type) {
	case cif.TextDelta:
		if a.textByBlock[idx] == nil {
			a.seen(idx)
			s := ""
			a.textByBlock[idx] = &s
		}
		*a.textByBlock[idx] += d.Text
	case cif.ThinkingDelta:
		if a.thinkingByBlock[idx] == nil {
			a.seen(idx)
			s := ""
			a.thinkingByBlock[idx] = &s
		}
		*a.thinkingByBlock[idx] += d.Thinking
	case cif.ToolArgumentsDelta:
		if a.toolByBlock[idx] == nil {
			a.seen(idx)
			a.toolByBlock[idx] = &toolAccum{}
		}
		a.toolByBlock[idx].rawArgs += d.PartialJSON
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
	for _, idx := range a.order {
		switch {
		case a.textByBlock[idx] != nil:
			resp.Content = append(resp.Content, cif.CIFTextPart{Type: "text", Text: *a.textByBlock[idx]})
		case a.thinkingByBlock[idx] != nil:
			resp.Content = append(resp.Content, cif.CIFThinkingPart{
				Type: "thinking", Thinking: *a.thinkingByBlock[idx], Signature: a.sigByBlock[idx],
			})
		case a.toolByBlock[idx] != nil:
			ta := a.toolByBlock[idx]
			args := ta.args
			if args == nil {
				args = decodeToolArgs(ta.rawArgs)
			}
			resp.Content = append(resp.Content, cif.CIFToolCallPart{
				Type: "tool_call", ToolCallID: ta.id, ToolName: ta.name, ToolArguments: args,
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
