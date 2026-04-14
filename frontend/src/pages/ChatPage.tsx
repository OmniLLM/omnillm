import { Send as SendIcon, Settings as SettingsIcon, Bot, User, X, Trash2, MessageSquare } from "lucide-react"
import { useEffect, useState, useRef, useCallback } from "react"

import { EmptyState } from "@/components/EmptyState"

import {
  getModels,
  createChatCompletion,
  listChatSessions,
  getChatSession,
  createChatSession,
  addChatMessage,
  deleteChatSession,
  deleteAllChatSessions,
  type ChatMessage,
  type ModelInfo,
  type ChatApiResponse,
  type ChatSessionSummary,
  type ChatCompletionResponse,
  type AnthropicResponse,
  type ResponsesResponse
} from "@/api"

import { createLogger } from "@/lib/logger"

const _log = createLogger("chat-page")

interface ChatPageProps {
  showToast: (msg: string, type?: "success" | "error") => void
}

type ApiShape = "openai" | "anthropic" | "responses"

// Helper function to extract message content from different API response formats
function extractMessageContent(response: ChatApiResponse, apiShape: ApiShape): string {
  // Check if it's an OpenAI-style response
  if ('choices' in response && response.choices && response.choices.length > 0) {
    return response.choices[0].message.content || ""
  }

  // Check if it's an Anthropic-style response
  if ('content' in response && Array.isArray(response.content)) {
    const textBlocks = response.content.filter(block => block.type === "text")
    return textBlocks.map(block => block.text || "").join("")
  }

  // Check if it's a Responses API response
  if ('output' in response && Array.isArray(response.output)) {
    const messageItems = response.output.filter(item => item.type === "message")
    if (messageItems.length > 0 && messageItems[0].content) {
      const textBlocks = messageItems[0].content.filter(block => block.type === "output_text")
      return textBlocks.map(block => block.text || "").join("")
    }
  }

  return ""
}

function generateUUID(): string {
  return `${Date.now()}-${Math.random().toString(36).substr(2, 9)}`
}

function formatChatError(error: unknown): string {
  if (error instanceof Error) {
    if (error.message.includes("fetch") || error.message.includes("network")) {
      return "Network error. Please check your connection and try again."
    }
    if (error.message.includes("401") || error.message.includes("unauthorized")) {
      return "Authentication failed. Please check your API key."
    }
    if (error.message.includes("429") || error.message.includes("rate limit")) {
      return "Rate limited. Please wait a moment and try again."
    }
    if (error.message.includes("5") || error.message.includes("server")) {
      return "Server error. Please try again shortly."
    }
    return "Something went wrong. Please try again."
  }
  return "An unexpected error occurred. Please try again."
}

export function ChatPage({ showToast }: ChatPageProps) {
  const [models, setModels] = useState<Array<ModelInfo>>([])
  const [selectedModel, setSelectedModel] = useState<string>("")
  const [apiShape, setApiShape] = useState<ApiShape>("openai")
  const [messages, setMessages] = useState<Array<ChatMessage>>([])
  const [inputValue, setInputValue] = useState("")
  const [isLoading, setIsLoading] = useState(false)
  const [modelsLoading, setModelsLoading] = useState(true)
  const [sessions, setSessions] = useState<Array<ChatSessionSummary>>([])
  const [currentSessionId, setCurrentSessionId] = useState<string | null>(null)
  const [sessionsLoading, setSessionsLoading] = useState(true)
  const [showDeleteAllConfirm, setShowDeleteAllConfirm] = useState(false)
  const unavailableModelToastRef = useRef<string | null>(null)
  const messagesEndRef = useRef<HTMLDivElement>(null)
  const messagesContainerRef = useRef<HTMLDivElement>(null)
  const availableModels = models.filter((model) => !model.api_shape || model.api_shape === apiShape)

  // Load sessions and models on component mount
  useEffect(() => {
    const loadData = async () => {
      try {
        const [modelsRes, sessionsRes] = await Promise.all([
          getModels(),
          listChatSessions()
        ])
        const loadedModels = modelsRes.data || []
        setModels(loadedModels)
        const initialModels = loadedModels.filter((model) => !model.api_shape || model.api_shape === apiShape)
        if (initialModels.length > 0) {
          setSelectedModel(initialModels[0].id)
        }
        setSessions(sessionsRes)
      } catch (error) {
        showToast(`Failed to load data: ${error}`, "error")
      } finally {
        setModelsLoading(false)
        setSessionsLoading(false)
      }
    }

    loadData()
  }, [showToast])

  useEffect(() => {
    if (modelsLoading) return

    if (availableModels.length === 0) {
      if (selectedModel) {
        setSelectedModel("")
      }
      return
    }

    if (!selectedModel) {
      setSelectedModel(availableModels[0].id)
      return
    }

    if (!availableModels.some((model) => model.id === selectedModel)) {
      const fallbackModel = availableModels[0]
      setSelectedModel(fallbackModel.id)
      if (unavailableModelToastRef.current !== selectedModel) {
        unavailableModelToastRef.current = selectedModel
        showToast(
          `Model "${selectedModel}" is unavailable for the current provider list; switched to ${fallbackModel.display_name || fallbackModel.id}`,
          "error"
        )
      }
      return
    }

    unavailableModelToastRef.current = null
  }, [availableModels, modelsLoading, selectedModel, showToast])

  // Load chat session when selected
  useEffect(() => {
    if (!currentSessionId) return

    const loadSession = async () => {
      try {
        const session = await getChatSession(currentSessionId)
        setMessages(session.messages.map(msg => ({
          role: msg.role as "user" | "assistant" | "system",
          content: msg.content
        })))
        setSelectedModel(session.session.model_id)
        setApiShape(session.session.api_shape as ApiShape)
      } catch (error) {
        showToast(`Failed to load session: ${error}`, "error")
      }
    }

    loadSession()
  }, [currentSessionId, showToast])

  // Auto-scroll to bottom only when user is already at the bottom
  useEffect(() => {
    const container = messagesContainerRef.current
    if (!container) return

    const isAtBottom = container.scrollHeight - container.scrollTop - container.clientHeight < 50
    if (isAtBottom) {
      messagesEndRef.current?.scrollIntoView({ behavior: "smooth" })
    }
  }, [messages])

  const handleSendMessage = async () => {
    if (!inputValue.trim() || !selectedModel || isLoading) return

    const userMessage: ChatMessage = {
      role: "user",
      content: inputValue.trim()
    }

    const newMessages = [...messages, userMessage]
    setMessages(newMessages)
    setInputValue("")
    setIsLoading(true)

    try {
      // Create session if not exists
      let sessionId = currentSessionId
      if (!sessionId) {
        sessionId = generateUUID()
        const title = inputValue.substring(0, 50) + (inputValue.length > 50 ? "..." : "")
        const createdSession = await createChatSession({
          session_id: sessionId,
          title: title,
          model_id: selectedModel,
          api_shape: apiShape
        })
        sessionId = createdSession.session_id || sessionId
        setCurrentSessionId(sessionId)
      }

      // Add user message to DB
      await addChatMessage(sessionId, {
        message_id: generateUUID(),
        role: "user",
        content: userMessage.content
      })

      // Get AI response
      const response = await createChatCompletion({
        model: selectedModel,
        messages: newMessages,
        stream: false,
      }, apiShape)

      if (response && !(response instanceof ReadableStream)) {
        const messageContent = extractMessageContent(response as ChatApiResponse, apiShape)
        if (messageContent) {
          const assistantMessage: ChatMessage = {
            role: "assistant",
            content: messageContent
          }
          const finalMessages = [...newMessages, assistantMessage]
          setMessages(finalMessages)

          // Add assistant message to DB
          await addChatMessage(sessionId, {
            message_id: generateUUID(),
            role: "assistant",
            content: messageContent
          })

          // Refresh sessions list
          const updatedSessions = await listChatSessions()
          setSessions(updatedSessions)
        } else {
          throw new Error("No content in response")
        }
      } else {
        throw new Error("No response received")
      }
    } catch (error) {
      const friendlyMessage = formatChatError(error)
      showToast(`Chat error: ${friendlyMessage}`, "error")
    } finally {
      setIsLoading(false)
    }
  }

  const handleKeyPress = useCallback((e: React.KeyboardEvent) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      handleSendMessage()
    }
  }, [inputValue, selectedModel, isLoading, messages])

  const handleNewChat = () => {
    setCurrentSessionId(null)
    setMessages([])
    setInputValue("")
  }

  const handleDeleteSession = async (sessionId: string, e: React.MouseEvent) => {
    e.stopPropagation()
    try {
      await deleteChatSession(sessionId)
      setSessions(sessions.filter(s => s.session_id !== sessionId))
      if (currentSessionId === sessionId) {
        handleNewChat()
      }
      showToast("Chat deleted", "success")
    } catch (error) {
      showToast(`Failed to delete chat: ${error}`, "error")
    }
  }

  const handleDeleteAllSessions = async () => {
    try {
      await deleteAllChatSessions()
      setSessions([])
      handleNewChat()
      showToast("All chats deleted", "success")
    } catch (error) {
      showToast(`Failed to delete all chats: ${error}`, "error")
    } finally {
      setShowDeleteAllConfirm(false)
    }
  }

  const clearChat = () => {
    handleNewChat()
  }

  return (
    <div style={{
      height: "calc(100dvh - 64px)",
      display: "flex",
      background: "var(--color-bg)",
      padding: "20px 20px 0 20px",
    }}>
      {/* Left Sidebar */}
      <div className="panel" style={{
        width: 280,
        height: "calc(100% - 20px)",
        display: "flex",
        flexDirection: "column",
        padding: "20px",
        gap: "12px",
        marginRight: "20px",
        marginBottom: "20px"
      }}>
        {/* New Chat Button */}
        <button
          onClick={handleNewChat}
          className="btn btn-primary"
          style={{ width: "100%", marginBottom: "8px" }}
        >
          + New Chat
        </button>

        {/* API Shape Selector */}
        <div>
          <label htmlFor="api-shape-select" className="sys-label" style={{
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            fontSize: "11px"
          }}>
            API Endpoint
          </label>
          <select
            id="api-shape-select"
            value={apiShape}
            onChange={(e) => setApiShape(e.target.value as ApiShape)}
            className="sys-select"
            style={{ background: "var(--color-surface-2)", fontSize: "13px" }}
          >
            <option value="openai">OpenAI</option>
            <option value="anthropic">Anthropic</option>
            <option value="responses">Responses</option>
          </select>
        </div>

        {/* Model Selector */}
        <div>
          <label htmlFor="model-select" className="sys-label" style={{
            textTransform: "uppercase",
            letterSpacing: "0.05em",
            fontSize: "11px"
          }}>
            Model
          </label>
          {modelsLoading ? (
            <div style={{
              padding: "10px 12px",
              border: "1px solid var(--color-separator)",
              borderRadius: "var(--radius-md)",
              background: "var(--color-surface)",
              color: "var(--color-text-secondary)",
              fontSize: 12,
            }}>
              Loading...
            </div>
          ) : (
            <select
              id="model-select"
              value={selectedModel}
              onChange={(e) => setSelectedModel(e.target.value)}
              className="sys-select"
              style={{ background: "var(--color-surface-2)", fontSize: "13px" }}
            >
              {availableModels.map((model) => (
                <option key={`${model.owned_by ?? ""}-${model.id}`} value={model.id}>
                  {model.display_name || model.id}
                </option>
              ))}
            </select>
          )}
        </div>

        {/* Chat History */}
        <div style={{ marginTop: "16px" }}>
          <div style={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "center",
            marginBottom: "8px"
          }}>
            <label className="sys-label" style={{
              textTransform: "uppercase",
              letterSpacing: "0.05em",
              fontSize: "11px",
              margin: 0
            }}>
              History
            </label>
            {sessions.length > 0 && (
              <button
                onClick={() => setShowDeleteAllConfirm(true)}
                style={{
                  height: 32,
                  minWidth: 32,
                  background: "none",
                  border: "none",
                  color: "var(--color-text-secondary)",
                  cursor: "pointer",
                  padding: "4px 8px",
                  display: "flex",
                  alignItems: "center",
                  gap: "4px",
                  fontSize: "11px",
                  borderRadius: "var(--radius-sm)",
                  transition: "color 0.2s, background 0.2s"
                }}
                onMouseEnter={(e) => {
                  e.currentTarget.style.color = "var(--color-red)"
                  e.currentTarget.style.background = "var(--color-red-fill)"
                }}
                onMouseLeave={(e) => {
                  e.currentTarget.style.color = "var(--color-text-secondary)"
                  e.currentTarget.style.background = "none"
                }}
                aria-label="Delete all chat history"
              >
                <Trash2 size={14} />
              </button>
            )}
          </div>

          {/* Delete All Confirmation Dialog */}
          {showDeleteAllConfirm && (
            <div
              role="alertdialog"
              aria-modal="true"
              aria-label="Confirm delete all chats"
              style={{
                position: "absolute",
                zIndex: 200,
                inset: 0,
                background: "var(--color-bg-elevated)",
                border: "1px solid var(--color-separator)",
                borderRadius: "var(--radius-lg)",
                padding: "16px",
                display: "flex",
                flexDirection: "column",
                gap: "12px",
                boxShadow: "var(--shadow-modal)"
              }}
            >
              <p style={{
                fontSize: "14px",
                color: "var(--color-text)",
                margin: 0,
                fontWeight: 500
              }}>
                Delete all chat history?
              </p>
              <p style={{
                fontSize: "12px",
                color: "var(--color-text-secondary)",
                margin: 0
              }}>
                This action cannot be undone.
              </p>
              <div style={{
                display: "flex",
                gap: "8px",
                marginTop: "auto"
              }}>
                <button
                  onClick={() => setShowDeleteAllConfirm(false)}
                  className="btn btn-ghost"
                  style={{ flex: 1 }}
                >
                  Cancel
                </button>
                <button
                  onClick={handleDeleteAllSessions}
                  className="btn btn-danger"
                  style={{ flex: 1 }}
                >
                  Delete All
                </button>
              </div>
            </div>
          )}

          {sessionsLoading ? (
            <div style={{
              padding: "12px",
              color: "var(--color-text-secondary)",
              fontSize: "12px",
              textAlign: "center"
            }}>
              Loading...
            </div>
          ) : sessions.length === 0 ? (
            <div style={{
              padding: "12px",
              color: "var(--color-text-secondary)",
              fontSize: "12px",
              textAlign: "center"
            }}>
              No chat history
            </div>
          ) : (
            <div className="scrollable-list">
              {sessions.map((session) => (
                <div
                  key={session.session_id}
                  role="button"
                  tabIndex={0}
                  aria-label={`Open chat: ${session.title}`}
                  onClick={() => setCurrentSessionId(session.session_id)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault()
                      setCurrentSessionId(session.session_id)
                    }
                  }}
                  className="list-item-interactive"
                  style={{
                    background: currentSessionId === session.session_id
                      ? "var(--color-blue-fill)"
                      : "var(--color-surface-2)",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    gap: "8px",
                  }}
                >
                  <div style={{
                    flex: 1,
                    overflow: "hidden"
                  }}>
                    <div style={{
                      fontSize: "12px",
                      color: currentSessionId === session.session_id
                        ? "white"
                        : "var(--color-text)",
                      whiteSpace: "nowrap",
                      overflow: "hidden",
                      textOverflow: "ellipsis"
                    }}>
                      {session.title}
                    </div>
                    <div style={{
                      fontSize: "11px",
                      color: currentSessionId === session.session_id
                        ? "rgba(255,255,255,0.7)"
                        : "var(--color-text-secondary)",
                      marginTop: "2px"
                    }}>
                      {new Date(session.updated_at).toLocaleDateString()}
                    </div>
                  </div>
                  <button
                    onClick={(e) => handleDeleteSession(session.session_id, e)}
                    style={{
                      height: 32,
                      width: 32,
                      background: "none",
                      border: "none",
                      color: "var(--color-text-secondary)",
                      cursor: "pointer",
                      padding: 0,
                      display: "flex",
                      alignItems: "center",
                      justifyContent: "center",
                      flexShrink: 0,
                      borderRadius: "var(--radius-sm)",
                      transition: "color 0.2s, background 0.2s"
                    }}
                    onMouseEnter={(e) => {
                      e.currentTarget.style.color = "var(--color-red)"
                      e.currentTarget.style.background = "var(--color-red-fill)"
                    }}
                    onMouseLeave={(e) => {
                      e.currentTarget.style.color = "var(--color-text-secondary)"
                      e.currentTarget.style.background = "none"
                    }}
                    aria-label={`Delete chat: ${session.title}`}
                  >
                    <X size={16} />
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>

        {/* Clear Current Chat */}
        <div style={{ marginTop: "auto", borderTop: "1px solid var(--color-separator)", paddingTop: "12px" }}>
          <button
            onClick={clearChat}
            className="btn btn-ghost"
            style={{ width: "100%", fontSize: "13px" }}
          >
            Clear Current
          </button>
        </div>
      </div>

      {/* Chat Area */}
      <div className="panel" style={{
        flex: 1,
        height: "calc(100% - 20px)",
        display: "flex",
        flexDirection: "column",
        background: "var(--color-bg-elevated)",
        marginBottom: "20px"
      }}>
        {/* Messages Container */}
        <div
          ref={messagesContainerRef}
          role="log"
          aria-label="Chat messages"
          aria-live="polite"
          style={{
            flex: 1,
            overflowY: "auto",
            padding: "24px",
            display: "flex",
            flexDirection: "column",
            gap: "20px",
          }}
        >
          {messages.length === 0 ? (
            <EmptyState
              icon={<MessageSquare size={24} />}
              title="Start a conversation"
              description="Select a model and send your first message to begin chatting."
            />
          ) : (
            messages.map((message, index) => (
              <div
                key={index}
                className="animate-slide-in"
                role={message.role === "user" ? "note" : "article"}
                aria-label={`${message.role === "user" ? "You" : "Assistant"} said`}
                style={{
                  display: "flex",
                  justifyContent: message.role === "user" ? "flex-end" : "flex-start",
                  animationDelay: `${index * 0.1}s`
                }}
              >
                <div style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: "12px",
                  maxWidth: "75%",
                  flexDirection: message.role === "user" ? "row-reverse" : "row"
                }}>
                  {/* Avatar */}
                  <div
                    aria-hidden="true"
                    style={{
                    width: "32px",
                    height: "32px",
                    borderRadius: "var(--radius-md)",
                    background: message.role === "user"
                      ? "var(--color-blue)"
                      : "var(--color-green)",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    flexShrink: 0,
                    boxShadow: "var(--shadow-btn)",
                  }}>
                    {message.role === "user" ?
                      <User size={16} style={{ color: "white" }} /> :
                      <Bot size={16} style={{ color: "white" }} />
                    }
                  </div>

                  {/* Message Bubble */}
                  <div style={{
                    padding: "12px 16px",
                    borderRadius: "var(--radius-lg)",
                    background: message.role === "user"
                      ? "var(--color-blue)"
                      : "var(--color-surface)",
                    color: message.role === "user"
                      ? "white"
                      : "var(--color-text)",
                    fontSize: "14px",
                    lineHeight: 1.6,
                    whiteSpace: "pre-wrap",
                    fontFamily: "var(--font-text)",
                    border: message.role === "user" ? "none" : "1px solid var(--color-separator)",
                    boxShadow: "var(--shadow-btn)"
                  }}>
                    {message.content}
                  </div>
                </div>
              </div>
            ))
          )}

          {isLoading && (
            <div className="animate-slide-in" style={{
              display: "flex",
              justifyContent: "flex-start"
            }}>
              <div style={{
                display: "flex",
                alignItems: "flex-start",
                gap: "12px",
                maxWidth: "75%"
              }}>
                <div
                  aria-hidden="true"
                  style={{
                    width: "32px",
                    height: "32px",
                    borderRadius: "var(--radius-md)",
                    background: "var(--color-green)",
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    flexShrink: 0,
                    boxShadow: "var(--shadow-btn)",
                  }}>
                  <Bot size={16} style={{ color: "white" }} />
                </div>

                <div style={{
                  padding: "12px 16px",
                  borderRadius: "var(--radius-lg)",
                  background: "var(--color-surface)",
                  border: "1px solid var(--color-separator)",
                  color: "var(--color-text-secondary)",
                  fontSize: "14px",
                  fontFamily: "var(--font-text)",
                  fontStyle: "italic",
                  boxShadow: "var(--shadow-btn)",
                  display: "flex",
                  alignItems: "center",
                  gap: "8px"
                }}>
                  <span className="spinner" />
                  Thinking...
                </div>
              </div>
            </div>
          )}

          <div ref={messagesEndRef} />
        </div>

        {/* Input Area */}
        <div style={{
          padding: "20px 24px",
          borderTop: "1px solid var(--color-separator)",
          background: "var(--color-surface)",
          borderBottomLeftRadius: "var(--radius-lg)",
          borderBottomRightRadius: "var(--radius-lg)",
        }}>
          <div style={{
            display: "flex",
            gap: "12px",
            alignItems: "flex-end",
          }}>
            <div style={{ flex: 1 }}>
              <label htmlFor="chat-input" className="sr-only" style={{
                position: "absolute",
                width: "1px",
                height: "1px",
                padding: 0,
                margin: "-1px",
                overflow: "hidden",
                clip: "rect(0, 0, 0, 0)",
                whiteSpace: "nowrap",
                border: 0
              }}>
                Chat message
              </label>
              <textarea
                id="chat-input"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                onKeyDown={handleKeyPress}
                placeholder="Chat with the model..."
                disabled={!selectedModel || isLoading}
                aria-describedby={!selectedModel ? "model-required-hint" : undefined}
                style={{
                  width: "100%",
                  minHeight: "44px",
                  maxHeight: "120px",
                  padding: "12px 16px",
                  border: "1px solid var(--color-separator)",
                  borderRadius: "var(--radius-lg)",
                  background: "var(--color-bg)",
                  color: "var(--color-text)",
                  fontSize: "14px",
                  fontFamily: "var(--font-text)",
                  resize: "none",
                  outline: "none",
                  transition: "border-color 0.15s var(--ease), box-shadow 0.15s var(--ease)"
                }}
                onFocus={(e) => {
                  e.currentTarget.style.borderColor = "var(--color-blue)"
                  e.currentTarget.style.boxShadow = "0 0 0 3px var(--color-blue-fill)"
                }}
                onBlur={(e) => {
                  e.currentTarget.style.borderColor = "var(--color-separator)"
                  e.currentTarget.style.boxShadow = "none"
                }}
              />
            </div>

            <button
              onClick={handleSendMessage}
              disabled={!inputValue.trim() || !selectedModel || isLoading}
              className="btn btn-primary"
              style={{
                width: "44px",
                height: "44px",
                padding: "0",
                display: "flex",
                alignItems: "center",
                justifyContent: "center",
                borderRadius: "var(--radius-lg)"
              }}
              aria-label="Send message"
            >
              <SendIcon size={18} />
            </button>
          </div>
          {!selectedModel && (
            <span id="model-required-hint" style={{
              fontSize: "12px",
              color: "var(--color-text-tertiary)",
              marginTop: "8px",
              display: "block"
            }}>
              No model available for this API shape.
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
