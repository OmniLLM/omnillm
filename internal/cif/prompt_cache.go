package cif

import "fmt"

const MaxPromptCacheBreakpoints = 4

func ValidateCacheControl(control *CIFCacheControl) error {
	if control == nil {
		return nil
	}
	if control.Type != "ephemeral" {
		return fmt.Errorf("cache_control.type must be \"ephemeral\"")
	}
	if control.TTL != nil && *control.TTL != CIFCacheTTL5m && *control.TTL != CIFCacheTTL1h {
		return fmt.Errorf("cache_control.ttl must be \"5m\" or \"1h\"")
	}
	return nil
}

func ValidatePromptCache(request *CanonicalRequest) error {
	if request == nil {
		return nil
	}
	if request.PromptCache != nil {
		if err := ValidateCacheControl(request.PromptCache.Automatic); err != nil {
			return err
		}
	}

	breakpoints := 0
	validate := func(control *CIFCacheControl) error {
		if err := ValidateCacheControl(control); err != nil {
			return err
		}
		if control != nil {
			breakpoints++
			if breakpoints > MaxPromptCacheBreakpoints {
				return fmt.Errorf("cache_control supports at most %d explicit breakpoints", MaxPromptCacheBreakpoints)
			}
		}
		return nil
	}

	for i := range request.Tools {
		if err := validate(request.Tools[i].CacheControl); err != nil {
			return fmt.Errorf("tools[%d]: %w", i, err)
		}
	}
	for i := range request.System {
		if err := validate(request.System[i].CacheControl); err != nil {
			return fmt.Errorf("system[%d]: %w", i, err)
		}
	}
	for messageIndex, message := range request.Messages {
		var controls []*CIFCacheControl
		switch typed := message.(type) {
		case CIFSystemMessage:
			for _, block := range typed.Content {
				controls = append(controls, block.CacheControl)
			}
		case CIFUserMessage:
			controls = contentCacheControls(typed.Content)
		case CIFAssistantMessage:
			controls = contentCacheControls(typed.Content)
		}
		for blockIndex, control := range controls {
			if err := validate(control); err != nil {
				return fmt.Errorf("messages[%d].content[%d]: %w", messageIndex, blockIndex, err)
			}
		}
	}
	return nil
}

func contentCacheControls(parts []CIFContentPart) []*CIFCacheControl {
	controls := make([]*CIFCacheControl, 0, len(parts))
	for _, part := range parts {
		switch typed := part.(type) {
		case CIFTextPart:
			controls = append(controls, typed.CacheControl)
		case CIFImagePart:
			controls = append(controls, typed.CacheControl)
		case CIFThinkingPart:
			controls = append(controls, typed.CacheControl)
		case CIFToolCallPart:
			controls = append(controls, typed.CacheControl)
		case CIFToolResultPart:
			controls = append(controls, typed.CacheControl)
		}
	}
	return controls
}

func UsageFromTotal(input, output int, cacheRead *int) *CIFUsage {
	usage := &CIFUsage{InputTokens: input, OutputTokens: output}
	if cacheRead != nil && *cacheRead >= 0 && *cacheRead <= input {
		read := *cacheRead
		usage.CacheReadInputTokens = &read
		uncached := input - read
		usage.UncachedInputTokens = &uncached
	}
	return usage
}

func UsageFromExclusiveBuckets(uncached, read, write, output int, write5m, write1h *int) (*CIFUsage, error) {
	if uncached < 0 || read < 0 || write < 0 || output < 0 {
		return nil, fmt.Errorf("usage counters must not be negative")
	}
	if write5m != nil && *write5m < 0 || write1h != nil && *write1h < 0 {
		return nil, fmt.Errorf("cache write detail must not be negative")
	}
	detailed := 0
	if write5m != nil {
		detailed += *write5m
	}
	if write1h != nil {
		detailed += *write1h
	}
	if detailed > write {
		return nil, fmt.Errorf("cache write detail exceeds total cache writes")
	}
	return &CIFUsage{
		InputTokens:             uncached + read + write,
		OutputTokens:            output,
		UncachedInputTokens:     intPointer(uncached),
		CacheReadInputTokens:    intPointer(read),
		CacheWriteInputTokens:   intPointer(write),
		CacheWrite5mInputTokens: write5m,
		CacheWrite1hInputTokens: write1h,
	}, nil
}

func (u *CIFUsage) AnthropicUncachedInput() int {
	if u == nil {
		return 0
	}
	if u.UncachedInputTokens != nil {
		return *u.UncachedInputTokens
	}
	uncached := u.InputTokens
	if u.CacheReadInputTokens != nil {
		uncached -= *u.CacheReadInputTokens
	}
	if u.CacheWriteInputTokens != nil {
		uncached -= *u.CacheWriteInputTokens
	}
	if uncached < 0 {
		return 0
	}
	return uncached
}

func intPointer(value int) *int { return &value }
