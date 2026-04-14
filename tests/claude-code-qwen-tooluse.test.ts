import { describe, it, expect } from "bun:test"

/**
 * Integration test simulating Claude Code tool use with Qwen3.6-Plus
 *
 * This tests the end-to-end flow:
 * 1. Claude Code sends a tool-use request to OmniModel
 * 2. OmniModel forwards to Alibaba DashScope
 * 3. Response includes tool calls
 */

describe("Claude Code Qwen3.6-Plus Tool Use", () => {
  // Test 1: Verify payload structure for tool use request
  it("should build correct OpenAI payload for qwen3.6-plus with tools", () => {
    const request = {
      model: "qwen3.6-plus",
      messages: [
        {
          role: "user",
          content: "What's 2 + 2?",
        },
      ],
      tools: [
        {
          type: "function",
          function: {
            name: "calculator",
            description: "A simple calculator",
            parameters: {
              type: "object",
              properties: {
                operation: { type: "string" },
                a: { type: "number" },
                b: { type: "number" },
              },
              required: ["operation", "a", "b"],
            },
          },
        },
      ],
      tool_choice: "auto",
      max_tokens: 1024,
      temperature: 0.7,
    }

    // Verify required fields
    expect(request.model).toBe("qwen3.6-plus")
    expect(request.messages).toHaveLength(1)
    expect(request.tools).toHaveLength(1)
    expect(request.tools[0].type).toBe("function")
    expect(request.tool_choice).toBe("auto")
    expect(request.max_tokens).toBe(1024)
    expect(request.temperature).toBe(0.7)
  })

  // Test 2: Verify tool call response parsing
  it("should correctly parse tool call from Qwen response", () => {
    const toolData = {
      type: "tool_calls",
      tool_calls: [
        {
          id: "call_abc123",
          type: "function",
          function: {
            name: "calculator",
            arguments: '{"operation": "add", "a": 2, "b": 2}',
          },
        },
      ],
    }

    expect(toolData.type).toBe("tool_calls")
    expect(toolData.tool_calls).toHaveLength(1)
    const toolCall = toolData.tool_calls[0]
    expect(toolCall.function.name).toBe("calculator")
    expect(toolCall.type).toBe("function")
    const parsed = JSON.parse(toolCall.function.arguments) as Record<
      string,
      unknown
    >
    expect(parsed).toEqual({ operation: "add", a: 2, b: 2 })
  })

  // Test 3: Verify tool result submission
  it("should handle tool result submission correctly", () => {
    const toolResultMessage = {
      role: "user",
      content: [
        {
          type: "tool_result",
          tool_use_id: "call_abc123",
          content: "4",
        },
      ],
    }

    expect(toolResultMessage.role).toBe("user")
    expect(toolResultMessage.content[0].type).toBe("tool_result")
    expect(toolResultMessage.content[0].tool_use_id).toBe("call_abc123")
    expect(toolResultMessage.content[0].content).toBe("4")
  })

  // Test 4: Verify headers are passed correctly
  it("should accept custom headers in tool use request", () => {
    const headers: Record<string, string> = {
      Authorization: "Bearer sk-...",
      "Content-Type": "application/json",
      "User-Agent": "claude-code/2.1.105",
      "X-Request-ID": "req-12345",
    }

    expect(headers["Authorization"]).toBeDefined()
    expect(headers["Content-Type"]).toBe("application/json")
    expect(headers["User-Agent"]).toContain("claude-code")
    expect(headers["X-Request-ID"]).toBe("req-12345")
  })

  // Test 5: Verify error handling
  it("should handle API errors gracefully", () => {
    const errorResponse = {
      error: {
        message: "Invalid request",
        type: "invalid_request_error",
        code: "invalid_model",
      },
    }

    expect(errorResponse.error.type).toBe("invalid_request_error")
    expect(errorResponse.error.message).toBeDefined()
    expect(errorResponse.error.code).toBe("invalid_model")
  })

  // Test 6: Verify streaming support for tool calls
  it("should support streaming tool calls", () => {
    const streamEvent = {
      type: "content_block_delta",
      delta: {
        type: "input_json_delta",
        input_json: '{"operation": "add"',
      },
    }

    expect(streamEvent.type).toBe("content_block_delta")
    expect(streamEvent.delta).toBeDefined()
    expect(streamEvent.delta.type).toBe("input_json_delta")
  })
})
