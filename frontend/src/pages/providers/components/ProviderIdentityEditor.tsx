import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { useConfirm } from "@/lib/useConfirm"

export function isValidProviderIdentity(value: string): boolean {
  return /^[A-Z0-9][\w.-]{0,63}$/i.test(value)
}

export async function submitProviderIdentity(
  oldId: string,
  value: string,
  confirm: (oldId: string, newId: string) => Promise<boolean>,
  save: (oldId: string, newId: string) => Promise<boolean>,
): Promise<"invalid" | "unchanged" | "canceled" | "failed" | "saved"> {
  if (value === oldId) return "unchanged"
  const newId = value.trim()
  if (newId === oldId) return "unchanged"
  if (!isValidProviderIdentity(newId)) return "invalid"
  if (!(await confirm(oldId, newId))) return "canceled"
  return (await save(oldId, newId)) ? "saved" : "failed"
}

export function ProviderIdentityEditor({
  id,
  onSave,
}: {
  id: string
  onSave: (oldId: string, newId: string) => Promise<boolean>
}) {
  const { t } = useTranslation("providers")
  const confirm = useConfirm()
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(id)
  const [error, setError] = useState("")
  const [saving, setSaving] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (editing) inputRef.current?.select()
  }, [editing])

  const cancel = () => {
    setValue(id)
    setError("")
    setEditing(false)
  }

  const save = async () => {
    setSaving(true)
    try {
      const result = await submitProviderIdentity(
        id,
        value,
        (oldId, newId) =>
          confirm({
            title: t("identity.confirmTitle"),
            message: t("identity.confirmMessage", { oldId, newId }),
            confirmLabel: t("identity.save"),
            cancelLabel: t("identity.cancel"),
          }),
        onSave,
      )
      if (result === "invalid") setError(t("identity.invalid"))
      if (result === "saved" || result === "unchanged") setEditing(false)
    } finally {
      setSaving(false)
    }
  }

  if (!editing) {
    return (
      <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
        <span>{id}</span>
        <button
          type="button"
          aria-label={t("identity.edit")}
          title={t("identity.edit")}
          onClick={() => {
            setValue(id)
            setEditing(true)
          }}
          style={{
            border: 0,
            background: "none",
            color: "var(--color-blue)",
            cursor: "pointer",
            fontSize: 11,
            padding: "2px 4px",
          }}
        >
          {t("identity.edit")}
        </button>
      </span>
    )
  }

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        void save()
      }}
      style={{
        display: "inline-flex",
        flexWrap: "wrap",
        alignItems: "center",
        gap: 5,
      }}
    >
      <input
        ref={inputRef}
        aria-label={t("identity.input")}
        aria-invalid={Boolean(error)}
        value={value}
        onChange={(event) => {
          setValue(event.target.value)
          setError("")
        }}
        onKeyDown={(event) => {
          if (event.key === "Escape") cancel()
        }}
        disabled={saving}
        style={{
          width: 220,
          maxWidth: "100%",
          color: "var(--color-text)",
          background: "var(--color-bg-input, var(--color-bg))",
          border: "1px solid var(--color-blue)",
          borderRadius: 4,
          padding: "3px 6px",
          font: "inherit",
        }}
      />
      <button type="submit" disabled={saving}>
        {t("identity.save")}
      </button>
      <button type="button" disabled={saving} onClick={cancel}>
        {t("identity.cancel")}
      </button>
      {error && (
        <span role="alert" style={{ color: "var(--color-red)" }}>
          {error}
        </span>
      )}
    </form>
  )
}
