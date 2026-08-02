# Copilot Claude Tool-Use Parse Fix

## Context

Claude Code requests routed through OmniLLM's Anthropic /v1/messages compatibility path could fail with:

- The model's tool call could not be parsed (retry also failed).

The issue reproduced on localhost:5000 for GitHub Copilot-backed Claude models, including claude-haiku-4.5 and claude-sonnet-4.6.

## What Changed

Before:

- Copilot Anthropic streaming for Claude models was buffered through the non-streaming execution path.
- That buffered path could produce a response with stop_reason: "tool_use" or inish_reason: "tool_calls" even when the serialized body only contained assistant text and no actual tool-call block.
- Claude Code rejected that payload as an invalid tool call.

After:

- Anthropic streaming for Copilot Claude models stays on the native chat-completions streaming path instead of buffering.
- OpenAI and Anthropic serializers now downgrade 	ool_use / 	ool_calls to a normal completion when no actual tool-call block exists, preventing impossible payload shapes from reaching clients.
- Regression tests now cover both the Copilot Anthropic-Claude streaming path and the dangling stop-reason serialization guard.

## Why It Is Critical

This is a tool-use compatibility fix for Claude Code. When OmniLLM emits a tool-use stop reason without a real tool payload, agent workflows fail immediately and the client cannot continue execution. That breaks core repository-inspection and coding flows on the local proxy.

## Affected Files

- internal/providers/copilot/adapter.go
- internal/providers/copilot/copilot_test.go
- internal/serialization/to_openai.go
- internal/serialization/to_anthropic.go
- internal/serialization/serialization_test.go

## Validation

- go test ./internal/providers/copilot
- go test ./internal/serialization
- Live Claude Agent SDK checks against http://127.0.0.1:5000:
  - claude-haiku-4.5 emitted valid tool calls and no longer failed with the parse error
  - claude-sonnet-4.6 emitted valid tool calls and no longer failed with the parse error

## Commit Range

Pending commit.
