/**
 * Frontend Test Infrastructure & Utilities
 *
 * Provides mocking utilities, fixtures, and helper functions for component testing
 */

import { describe, expect, beforeEach, afterEach, mock } from "bun:test"

// ─── LocalStorage & SessionStorage Mocks ─────────────────────────────────────

export class StorageMock {
  private data: Record<string, string> = {}

  getItem(key: string): string | null {
    return this.data[key] ?? null
  }

  setItem(key: string, value: string): void {
    this.data[key] = value
  }

  removeItem(key: string): void {
    delete this.data[key]
  }

  clear(): void {
    this.data = {}
  }

  get length(): number {
    return Object.keys(this.data).length
  }

  key(index: number): string | null {
    const keys = Object.keys(this.data)
    return keys[index] ?? null
  }
}

export function setupStorageMocks(globalThis: any) {
  globalThis.localStorage = new StorageMock()
  globalThis.sessionStorage = new StorageMock()
}

// ─── EventSource Mock ───────────────────────────────────────────────────────

export class EventSourceMock {
  public onopen: ((event: Event) => void) | null = null
  public onmessage: ((event: MessageEvent) => void) | null = null
  public onerror: ((event: Event) => void) | null = null
  private listeners: Record<string, Array<(event: Event) => void>> = {}
  public url: string
  public readyState = 0

  constructor(url: string) {
    this.url = url
  }

  addEventListener(event: string, listener: (event: Event) => void) {
    if (!this.listeners[event]) {
      this.listeners[event] = []
    }
    this.listeners[event].push(listener)
  }

  removeEventListener(event: string, listener: (event: Event) => void) {
    if (this.listeners[event]) {
      this.listeners[event] = this.listeners[event].filter(
        (l) => l !== listener,
      )
    }
  }

  dispatchEvent(event: Event): boolean {
    const handlers = this.listeners[(event as any).type] ?? []
    for (const h of handlers) h(event)
    return true
  }

  close() {
    this.readyState = 2
  }

  _triggerOpen() {
    this.readyState = 1
    const event = new Event("open")
    this.dispatchEvent(event)
    if (this.onopen) this.onopen(event)
  }

  _triggerMessage(data: string) {
    const event = new MessageEvent("message", { data })
    this.dispatchEvent(event)
    if (this.onmessage) this.onmessage(event)
  }

  _triggerError() {
    const event = new Event("error")
    this.dispatchEvent(event)
    if (this.onerror) this.onerror(event)
  }
}

// ─── WebSocket Mock ─────────────────────────────────────────────────────────

export class WebSocketMock {
  public onopen: ((event: Event) => void) | null = null
  public onmessage: ((event: MessageEvent) => void) | null = null
  public onerror: ((event: Event) => void) | null = null
  public onclose: ((event: CloseEvent) => void) | null = null
  public readyState = 0
  public url: string
  private listeners: Record<string, Array<(event: Event) => void>> = {}

  constructor(url: string) {
    this.url = url
  }

  addEventListener(event: string, listener: (event: Event) => void) {
    if (!this.listeners[event]) {
      this.listeners[event] = []
    }
    this.listeners[event].push(listener)
  }

  removeEventListener(event: string, listener: (event: Event) => void) {
    if (this.listeners[event]) {
      this.listeners[event] = this.listeners[event].filter(
        (l) => l !== listener,
      )
    }
  }

  dispatchEvent(event: Event): boolean {
    const handlers = this.listeners[(event as any).type] ?? []
    for (const h of handlers) h(event)
    return true
  }

  close() {
    this.readyState = 2
  }

  _triggerOpen() {
    this.readyState = 1
    const event = new Event("open")
    this.dispatchEvent(event)
    if (this.onopen) this.onopen(event)
  }

  _triggerMessage(data: string) {
    const event = new MessageEvent("message", { data })
    this.dispatchEvent(event)
    if (this.onmessage) this.onmessage(event)
  }

  _triggerError() {
    const event = new Event("error")
    this.dispatchEvent(event)
    if (this.onerror) this.onerror(event)
  }

  _triggerClose() {
    this.readyState = 3
    const event = new CloseEvent("close")
    this.dispatchEvent(event)
    if (this.onclose) this.onclose(event)
  }
}

// ─── Fetch Mock Setup ────────────────────────────────────────────────────────

type FetchResponse = {
  ok: boolean
  status: number
  json: () => Promise<any>
  text: () => Promise<string>
  body: ReadableStream<Uint8Array> | null
}

type FetchMockSetup = Record<
  string,
  Record<string, { response: any; status?: number; error?: Error }>
>

export function setupFetchMocks(
  globalThis: any,
  endpoints: FetchMockSetup = {},
) {
  const originalFetch = globalThis.fetch
  const mockResponses: Map<string, FetchResponse> = new Map()

  const mockFetch = mock(
    async (url: string, init?: RequestInit): Promise<FetchResponse> => {
      const urlObj = new URL(url, "http://localhost:4141")
      const method = init?.method ?? "GET"
      const path = urlObj.pathname

      const key = `${method} ${path}`
      const endpointConfig =
        endpoints[path]?.[method] ?? endpoints[`${method} ${path}`]

      if (endpointConfig && endpointConfig.error) {
        throw endpointConfig.error
      }

      if (!endpointConfig) {
        return {
          ok: false,
          status: 404,
          json: async () => ({ error: "Endpoint not found" }),
          text: async () => "Not Found",
          body: null,
        }
      }

      const status =
        endpointConfig.status ?? (endpointConfig.response ? 200 : 500)
      return {
        ok: status >= 200 && status < 300,
        status,
        json: async () => endpointConfig.response,
        text: async () => JSON.stringify(endpointConfig.response),
        body: null,
      }
    },
  )

  globalThis.fetch = mockFetch
  return mockFetch
}

// ─── Mock Data Builders ──────────────────────────────────────────────────────

export function createMockProvider(overrides: any = {}) {
  return {
    id: "test-provider-1",
    type: "alibaba",
    name: "Alibaba",
    isActive: true,
    authStatus: "authenticated" as const,
    enabledModelCount: 5,
    totalModelCount: 10,
    priority: 1,
    ...overrides,
  }
}

export function createMockModelInfo(overrides: any = {}) {
  return {
    id: "gpt-4",
    display_name: "GPT-4",
    name: "gpt-4",
    owned_by: "openai",
    ...overrides,
  }
}

export function createMockChatSession(overrides: any = {}) {
  return {
    session_id: "session-123",
    title: "Test Chat Session",
    model_id: "gpt-4",
    api_shape: "openai",
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

export function createMockStatus(overrides: any = {}) {
  return {
    activeProvider: { id: "test-1", name: "Test Provider" },
    modelCount: 10,
    manualApprove: false,
    rateLimitSeconds: null,
    rateLimitWait: false,
    authFlow: null,
    ...overrides,
  }
}

export function createMockServerInfo(overrides: any = {}) {
  return {
    version: "0.1.0",
    port: 4141,
    ...overrides,
  }
}

export function createMockChatMessage(overrides: any = {}) {
  return {
    role: "user" as const,
    content: "Hello!",
    ...overrides,
  }
}

export function createMockChatResponse(overrides: any = {}) {
  return {
    id: "response-123",
    object: "chat.completion",
    created: Date.now(),
    model: "gpt-4",
    choices: [
      {
        index: 0,
        message: { role: "assistant" as const, content: "Hi there!" },
        finish_reason: "stop",
      },
    ],
    ...overrides,
  }
}

export function createMockAnthropicResponse(overrides: any = {}) {
  return {
    id: "msg-123",
    type: "message",
    role: "assistant",
    content: [{ type: "text" as const, text: "Hello!" }],
    model: "claude-3",
    stop_reason: "end_turn",
    ...overrides,
  }
}

export function createMockResponsesResponse(overrides: any = {}) {
  return {
    id: "response-123",
    object: "realtime.response",
    model: "gpt-4",
    output: [
      {
        type: "message",
        id: "msg-1",
        role: "assistant",
        content: [{ type: "output_text" as const, text: "Hello!" }],
      },
    ],
    ...overrides,
  }
}

export function createMockUsageData(overrides: any = {}) {
  return {
    access_type_sku: "copilot_pro",
    copilot_plan: "pro",
    quota_reset_date: new Date(
      Date.now() + 30 * 24 * 60 * 60 * 1000,
    ).toISOString(),
    chat_enabled: true,
    assigned_date: new Date().toISOString(),
    quota_snapshots: {
      copilot_chat_operations: {
        entitlement: 100,
        remaining: 75,
        percent_remaining: 75,
        unlimited: false,
      },
    },
    ...overrides,
  }
}

// ─── Test Assertions ─────────────────────────────────────────────────────────

export function expectFunctionCalled(fn: any, expectedArgs?: Array<any>) {
  expect(fn).toHaveBeenCalled()
  if (expectedArgs) {
    expect(fn).toHaveBeenCalledWith(...expectedArgs)
  }
}

export function expectToastShown(
  showToast: any,
  message: string,
  type: string = "error",
) {
  expect(showToast).toHaveBeenCalledWith(expect.stringContaining(message), type)
}

// ─── Setup & Teardown ────────────────────────────────────────────────────────

export function setupTestEnvironment() {
  setupStorageMocks(globalThis)
  ;(globalThis as any).document = {
    querySelector: (selector: string) => {
      if (selector === 'meta[name="omnillm-api-key"]') {
        return {
          getAttribute: (name: string) => (name === "content" ? "" : null),
        }
      }
      return null
    },
  }

  // Mock EventSource
  ;(globalThis as any).EventSource = EventSourceMock
  ;(globalThis as any).WebSocket = WebSocketMock
  ;(globalThis as any).MessageEvent = class MessageEvent extends Event {
    constructor(
      type: string,
      public data: any,
    ) {
      super(type)
    }
  }
  ;(globalThis as any).CloseEvent = class CloseEvent extends Event {
    constructor(type: string) {
      super(type)
    }
  }

  // Mock location
  ;(globalThis as any).location = {
    hostname: "localhost",
    port: "5173",
    protocol: "http:",
    hash: "",
    pathname: "/",
  }
}

export function resetTestEnvironment() {
  if (globalThis.localStorage instanceof StorageMock) {
    globalThis.localStorage.clear()
  }
  if (globalThis.sessionStorage instanceof StorageMock) {
    globalThis.sessionStorage.clear()
  }
}

// Helper to run a test suite with proper setup/teardown
export function describeWithSetup(name: string, fn: () => void) {
  describe(name, () => {
    beforeEach(() => {
      setupTestEnvironment()
    })

    afterEach(() => {
      resetTestEnvironment()
    })

    fn()
  })
}
