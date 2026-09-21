import { useState } from "react"
import { useTranslation } from "react-i18next"

interface TypeSafeFormProps {
  onSubmit: (options: Record<string, string>) => Promise<void>
  onCancel: () => void
  submitting: boolean
}

export function TypeSafeForm({
  onSubmit,
  onCancel,
  submitting,
}: TypeSafeFormProps) {
  const { t } = useTranslation("providers")
  const [apiKey, setApiKey] = useState("")
  const [pending, setPending] = useState(false)
  const [failed, setFailed] = useState(false)
  const busy = submitting || pending
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault()
        if (apiKey.trim() && !busy) {
          setPending(true)
          setFailed(false)
          void onSubmit({ method: "api-key", apiKey: apiKey.trim() })
            .catch(() => setFailed(true))
            .finally(() => setPending(false))
        }
      }}
    >
      <p>{t("typesafe.description")}</p>
      {failed && <p role="alert">{t("typesafe.failed")}</p>}
      <label>
        {t("typesafe.apiKey")}
        <input
          type="password"
          autoComplete="off"
          value={apiKey}
          onChange={(event) => setApiKey(event.target.value)}
          disabled={busy}
        />
      </label>
      <div style={{ display: "flex", gap: 8, marginTop: 16 }}>
        <button
          className="btn btn-primary btn-sm"
          type="submit"
          disabled={busy || !apiKey.trim()}
        >
          {t("typesafe.connect")}
        </button>
        <button
          className="btn btn-ghost btn-sm"
          type="button"
          onClick={onCancel}
          disabled={busy}
        >
          {t("typesafe.cancel")}
        </button>
      </div>
    </form>
  )
}
