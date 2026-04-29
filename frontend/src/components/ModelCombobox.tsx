import { Search, ChevronDown } from "lucide-react"
import { useEffect, useState, useRef, useMemo, useCallback } from "react"

import { type ModelInfo } from "@/api"

// ─── Helpers ──────────────────────────────────────────────────────────────────

function getModelItemBackground(
  isHighlighted: boolean,
  isSelected: boolean,
): string {
  if (isHighlighted) return "var(--color-surface-2)"
  if (isSelected) return "var(--color-blue-fill)"
  return "transparent"
}

function filterModels(
  models: Array<ModelInfo>,
  text: string,
): Array<ModelInfo> {
  if (!text) return models
  const lower = text.toLowerCase()
  return models.filter(
    (m) =>
      m.id.toLowerCase().includes(lower)
      || m.display_name?.toLowerCase().includes(lower)
      || m.owned_by?.toLowerCase().includes(lower),
  )
}

interface KeyDownContext {
  e: React.KeyboardEvent
  filteredModels: Array<ModelInfo>
  highlightedIndex: number
  close: () => void
  select: (modelId: string) => void
  setIndex: (fn: (prev: number) => number) => void
}

function handleComboboxKeyDown(ctx: KeyDownContext) {
  switch (ctx.e.key) {
    case "ArrowDown": {
      ctx.e.preventDefault()
      ctx.setIndex((prev) => Math.min(prev + 1, ctx.filteredModels.length - 1))
      break
    }
    case "ArrowUp": {
      ctx.e.preventDefault()
      ctx.setIndex((prev) => Math.max(prev - 1, 0))
      break
    }
    case "Enter": {
      ctx.e.preventDefault()
      if (ctx.filteredModels[ctx.highlightedIndex])
        ctx.select(ctx.filteredModels[ctx.highlightedIndex].id)
      break
    }
    case "Escape": {
      ctx.close()
      break
    }
    default: {
      break
    }
  }
}

// ─── Custom Hook ──────────────────────────────────────────────────────────────

interface ComboboxOptions {
  models: Array<ModelInfo>
  selectedModel: string
  onSelect: (modelId: string) => void
  disabled?: boolean
}

function useModelComboboxState(options: ComboboxOptions) {
  const { models, selectedModel, onSelect, disabled } = options
  const [isOpen, setIsOpen] = useState(false)
  const [filterText, setFilterText] = useState("")
  const [highlightedIndex, setHighlightedIndex] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const filteredModels = useMemo(
    () => filterModels(models, filterText),
    [models, filterText],
  )
  const selectedDisplay = useMemo(
    () =>
      models.find((m) => m.id === selectedModel)?.display_name || selectedModel,
    [models, selectedModel],
  )

  const close = useCallback(() => {
    setIsOpen(false)
    setFilterText("")
  }, [])

  useEffect(() => {
    setHighlightedIndex(0)
  }, [filterText])
  useEffect(() => {
    const list = listRef.current
    if (!list || !isOpen) return
    const item = list.children[highlightedIndex] as HTMLElement | undefined
    if (item) item.scrollIntoView({ block: "nearest" })
  }, [highlightedIndex, isOpen])
  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (
        containerRef.current
        && !containerRef.current.contains(e.target as Node)
      )
        close()
    }
    document.addEventListener("mousedown", handler)
    return () => document.removeEventListener("mousedown", handler)
  }, [close])

  const handleToggle = useCallback(() => {
    if (disabled) return
    const nextOpen = !isOpen
    setIsOpen(nextOpen)
    if (nextOpen) {
      setFilterText("")
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [disabled, isOpen])

  const handleSelect = useCallback(
    (modelId: string) => {
      onSelect(modelId)
      setIsOpen(false)
      setFilterText("")
    },
    [onSelect],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) =>
      handleComboboxKeyDown({
        e,
        filteredModels,
        highlightedIndex,
        close,
        select: handleSelect,
        setIndex: setHighlightedIndex,
      }),
    [filteredModels, highlightedIndex, close, handleSelect],
  )

  return {
    isOpen,
    filterText,
    highlightedIndex,
    filteredModels,
    selectedDisplay,
    containerRef,
    inputRef,
    listRef,
    handleToggle,
    handleSelect,
    handleKeyDown,
    setFilterText,
    setHighlightedIndex,
  }
}

// ─── Sub-Components ───────────────────────────────────────────────────────────

function ModelListItem({
  model,
  isSelected,
  isHighlighted,
  onSelect,
  onHover,
}: {
  model: ModelInfo
  isSelected: boolean
  isHighlighted: boolean
  onSelect: () => void
  onHover: () => void
}) {
  const displayName = model.display_name || model.id
  return (
    <div
      role="option"
      aria-selected={isSelected}
      aria-label={displayName}
      onClick={onSelect}
      onMouseEnter={onHover}
      style={{
        padding: "8px 12px",
        fontSize: 13,
        cursor: "pointer",
        display: "flex",
        alignItems: "center",
        justifyContent: "space-between",
        gap: 8,
        background: getModelItemBackground(isHighlighted, isSelected),
        color: isSelected ? "white" : "var(--color-text)",
        borderRadius: 0,
        transition: "background 0.1s",
        borderLeft:
          isSelected ? "3px solid var(--color-blue)" : "3px solid transparent",
      }}
    >
      <span
        style={{
          overflow: "hidden",
          textOverflow: "ellipsis",
          whiteSpace: "nowrap",
        }}
      >
        {displayName}
      </span>
      {model.owned_by && (
        <span
          style={{
            fontSize: 11,
            color:
              isSelected ?
                "rgba(255,255,255,0.7)"
              : "var(--color-text-tertiary)",
            whiteSpace: "nowrap",
          }}
        >
          {model.owned_by}
        </span>
      )}
    </div>
  )
}

function ModelSearchInput({
  filterText,
  onChangeFilter,
  onKeyDown,
  inputRef,
}: {
  filterText: string
  onChangeFilter: (value: string) => void
  onKeyDown: (e: React.KeyboardEvent) => void
  inputRef: React.RefObject<HTMLInputElement | null>
}) {
  return (
    <div
      style={{
        padding: "8px 10px",
        borderBottom: "1px solid var(--color-separator)",
        display: "flex",
        alignItems: "center",
        gap: 8,
      }}
    >
      <Search
        size={14}
        style={{ color: "var(--color-text-tertiary)", flexShrink: 0 }}
      />
      <input
        ref={inputRef}
        type="text"
        value={filterText}
        onChange={(e) => onChangeFilter(e.target.value)}
        onKeyDown={onKeyDown}
        placeholder="Filter models..."
        aria-label="Filter models"
        style={{
          flex: 1,
          background: "transparent",
          border: "none",
          color: "var(--color-text)",
          fontSize: 13,
          fontFamily: "inherit",
        }}
      />
    </div>
  )
}

function ModelDropdownPanel({
  filteredModels,
  filterText,
  selectedModel,
  highlightedIndex,
  listRef,
  inputRef,
  onChangeFilter,
  onKeyDown,
  onSelect,
  onHover,
}: {
  filteredModels: Array<ModelInfo>
  filterText: string
  selectedModel: string
  highlightedIndex: number
  listRef: React.RefObject<HTMLDivElement | null>
  inputRef: React.RefObject<HTMLInputElement | null>
  onChangeFilter: (value: string) => void
  onKeyDown: (e: React.KeyboardEvent) => void
  onSelect: (modelId: string) => void
  onHover: (index: number) => void
}) {
  return (
    <div
      role="listbox"
      aria-label="Select a model"
      style={{
        position: "absolute",
        top: "100%",
        left: 0,
        right: 0,
        zIndex: 50,
        marginTop: 4,
        background: "var(--color-surface)",
        border: "1px solid var(--color-separator)",
        borderRadius: "var(--radius-md)",
        boxShadow: "var(--shadow-modal)",
        overflow: "hidden",
      }}
    >
      <ModelSearchInput
        filterText={filterText}
        onChangeFilter={onChangeFilter}
        onKeyDown={onKeyDown}
        inputRef={inputRef}
      />
      <div
        ref={listRef}
        style={{ maxHeight: 240, overflowY: "auto", padding: "4px 0" }}
      >
        {filteredModels.length === 0 ?
          <div
            role="option"
            aria-disabled="true"
            style={{
              padding: "10px 12px",
              fontSize: 12,
              color: "var(--color-text-tertiary)",
              textAlign: "center",
            }}
          >
            No models match &quot;{filterText}&quot;
          </div>
        : filteredModels.map((model, index) => (
            <ModelListItem
              key={`${model.owned_by ?? ""}-${model.id}`}
              model={model}
              isSelected={model.id === selectedModel}
              isHighlighted={index === highlightedIndex}
              onSelect={() => onSelect(model.id)}
              onHover={() => onHover(index)}
            />
          ))
        }
      </div>
    </div>
  )
}

// ─── Model Combobox ───────────────────────────────────────────────────────────

export function ModelCombobox({
  models,
  selectedModel,
  onSelect,
  disabled,
}: ComboboxOptions) {
  const state = useModelComboboxState({
    models,
    selectedModel,
    onSelect,
    disabled,
  })

  return (
    <div ref={state.containerRef} style={{ position: "relative" }}>
      <button
        type="button"
        onClick={state.handleToggle}
        disabled={disabled}
        aria-expanded={state.isOpen}
        aria-haspopup="listbox"
        className="sys-select"
        style={{
          background: "var(--color-surface-2)",
          fontSize: 13,
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 8,
          cursor: disabled ? "not-allowed" : "pointer",
          width: "100%",
        }}
      >
        <span
          style={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
          }}
        >
          {selectedModel ? state.selectedDisplay : "Select a model..."}
        </span>
        <ChevronDown
          size={14}
          style={{
            opacity: 0.5,
            transform: state.isOpen ? "rotate(180deg)" : "rotate(0deg)",
            transition: "transform 0.15s var(--ease)",
          }}
        />
      </button>

      {state.isOpen && (
        <ModelDropdownPanel
          filteredModels={state.filteredModels}
          filterText={state.filterText}
          selectedModel={selectedModel}
          highlightedIndex={state.highlightedIndex}
          listRef={state.listRef}
          inputRef={state.inputRef}
          onChangeFilter={state.setFilterText}
          onKeyDown={state.handleKeyDown}
          onSelect={state.handleSelect}
          onHover={state.setHighlightedIndex}
        />
      )}
    </div>
  )
}
