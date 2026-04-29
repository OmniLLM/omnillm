import { Search, ChevronDown, X } from "lucide-react"
import { useEffect, useState, useRef, useMemo, useCallback } from "react"

interface SearchableSelectProps {
  options: Array<string>
  value: string
  onChange: (value: string) => void
  placeholder?: string
}

function getOptionBackground(isHighlighted: boolean, isSelected: boolean) {
  if (isHighlighted) return "var(--color-surface-2)"
  if (isSelected) return "var(--color-blue-fill)"
  return "transparent"
}

export function SearchableSelect({
  options,
  value,
  onChange,
  placeholder = "Select...",
}: SearchableSelectProps) {
  const [open, setOpen] = useState(false)
  const [filter, setFilter] = useState("")
  const [highlighted, setHighlighted] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = useMemo(() => {
    if (!filter) return options
    const lower = filter.toLowerCase()
    return options.filter((o) => o.toLowerCase().includes(lower))
  }, [options, filter])

  const close = useCallback(() => {
    setOpen(false)
    setFilter("")
  }, [])

  useEffect(() => {
    setHighlighted(0)
  }, [filter])

  useEffect(() => {
    if (!open) return
    inputRef.current?.focus()
  }, [open])

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

  const select = useCallback(
    (v: string) => {
      onChange(v)
      close()
    },
    [onChange, close],
  )

  const handleKeyDown = (e: React.KeyboardEvent) => {
    switch (e.key) {
      case "ArrowDown": {
        e.preventDefault()
        setHighlighted((prev) => Math.min(prev + 1, filtered.length - 1))
        break
      }
      case "ArrowUp": {
        e.preventDefault()
        setHighlighted((prev) => Math.max(prev - 1, 0))
        break
      }
      case "Enter": {
        e.preventDefault()
        if (filtered[highlighted]) select(filtered[highlighted])
        break
      }
      case "Escape": {
        close()
        break
      }
      default: {
        break
      }
    }
  }

  return (
    <div ref={containerRef} style={{ position: "relative" }}>
      <button
        type="button"
        className="sys-select"
        onClick={() => setOpen((prev) => !prev)}
        style={{
          display: "flex",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 8,
          width: "100%",
          textAlign: "left",
        }}
      >
        <span
          style={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            color: value ? "var(--color-text)" : "var(--color-text-tertiary)",
          }}
        >
          {value || placeholder}
        </span>
        <span
          style={{
            display: "flex",
            alignItems: "center",
            gap: 6,
            flexShrink: 0,
          }}
        >
          {value && (
            <span
              role="button"
              aria-label="Clear selection"
              onClick={(e) => {
                e.stopPropagation()
                onChange("")
                setFilter("")
                setOpen(false)
              }}
              style={{
                display: "inline-flex",
                alignItems: "center",
                justifyContent: "center",
                width: 18,
                height: 18,
                borderRadius: 999,
                color: "var(--color-text-tertiary)",
              }}
            >
              <X size={12} />
            </span>
          )}
          <ChevronDown
            size={14}
            style={{
              opacity: 0.5,
              transform: open ? "rotate(180deg)" : "rotate(0deg)",
              transition: "transform 0.15s",
            }}
          />
        </span>
      </button>

      {open && (
        <div
          style={{
            position: "absolute",
            top: "100%",
            left: 0,
            right: 0,
            zIndex: 200,
            marginTop: 4,
            background: "var(--color-surface)",
            border: "1px solid var(--color-separator)",
            borderRadius: "var(--radius-md)",
            boxShadow: "var(--shadow-modal)",
            overflow: "hidden",
          }}
        >
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
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Filter..."
              style={{
                flex: 1,
                background: "transparent",
                border: "none",
                color: "var(--color-text)",
                fontSize: 13,
                fontFamily: "inherit",
                outline: "none",
              }}
            />
          </div>
          <div style={{ maxHeight: 200, overflowY: "auto" }}>
            {filtered.length === 0 ?
              <div
                style={{
                  padding: "10px 12px",
                  fontSize: 12,
                  color: "var(--color-text-tertiary)",
                  textAlign: "center",
                }}
              >
                No matches
              </div>
            : filtered.map((opt, i) => (
                <div
                  key={opt}
                  onClick={() => select(opt)}
                  onMouseEnter={() => setHighlighted(i)}
                  style={{
                    padding: "8px 12px",
                    fontSize: 13,
                    cursor: "pointer",
                    background: getOptionBackground(
                      i === highlighted,
                      opt === value,
                    ),
                    color:
                      opt === value && i !== highlighted ?
                        "white"
                      : "var(--color-text)",
                  }}
                >
                  {opt}
                </div>
              ))
            }
          </div>
        </div>
      )}
    </div>
  )
}
