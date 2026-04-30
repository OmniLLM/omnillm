/* eslint-disable @typescript-eslint/no-floating-promises */
import { Eye, EyeOff } from "lucide-react"
import { useState, useEffect, useCallback } from "react"
import { useTranslation } from "react-i18next"

import {
  listAccessTokens,
  createAccessToken,
  deleteAccessToken,
  type AccessToken,
} from "@/api"
import { useConfirm } from "@/lib/useConfirm"

interface Props {
  showToast: (msg: string, type?: "success" | "error") => void
}

const EXPIRY_OPTIONS = [
  { value: "", labelKey: "noExpiration" },
  { value: "30", labelKey: "days30" },
  { value: "60", labelKey: "days60" },
  { value: "90", labelKey: "days90" },
  { value: "365", labelKey: "year1" },
]

export function AccessTokensPage({ showToast }: Props) {
  const { t } = useTranslation("tokens")
  const confirm = useConfirm()
  const [tokens, setTokens] = useState<Array<AccessToken>>([])
  const [loading, setLoading] = useState(true)
  const [showCreate, setShowCreate] = useState(false)
  const [newTokenName, setNewTokenName] = useState("")
  const [newTokenExpiry, setNewTokenExpiry] = useState("")
  const [createdToken, setCreatedToken] = useState<string | null>(null)
  const [showCreatedToken, setShowCreatedToken] = useState(false)
  const [copied, setCopied] = useState(false)
  const [creating, setCreating] = useState(false)

  const fetchTokens = useCallback(async () => {
    try {
      const data = await listAccessTokens()
      setTokens(data)
    } catch {
      showToast("Failed to load tokens", "error")
    } finally {
      setLoading(false)
    }
  }, [showToast])

  useEffect(() => {
    fetchTokens()
  }, [fetchTokens])

  const handleCreate = async () => {
    if (!newTokenName.trim()) return
    setCreating(true)
    try {
      let expiresAt: string | undefined
      if (newTokenExpiry) {
        const d = new Date()
        d.setDate(d.getDate() + Number.parseInt(newTokenExpiry, 10))
        expiresAt = d.toISOString()
      }
      const res = await createAccessToken(newTokenName.trim(), expiresAt)
      setCreatedToken(res.token)
      setShowCreatedToken(false)
      showToast(t("tokenCreatedSuccess"), "success")
      fetchTokens()
    } catch {
      showToast("Failed to create token", "error")
    } finally {
      setCreating(false)
    }
  }

  const handleDelete = async (token: AccessToken) => {
    const ok = await confirm({
      title: t("deleteConfirmTitle"),
      message: t("deleteConfirmMessage", { name: token.name }),
      confirmLabel: t("delete"),
    })
    if (!ok) return
    try {
      await deleteAccessToken(token.id)
      showToast(t("tokenDeleted"), "success")
      fetchTokens()
    } catch {
      showToast("Failed to delete token", "error")
    }
  }

  const handleCopy = async (text: string) => {
    await navigator.clipboard.writeText(text)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const formatDate = (dateStr: string | null) => {
    if (!dateStr) return t("never")
    return new Date(dateStr).toLocaleDateString(undefined, {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    })
  }

  const formatExpiry = (dateStr: string | null) => {
    if (!dateStr) return t("noExpiry")
    return formatDate(dateStr)
  }

  let createdTokenDisplay = ""
  if (createdToken) {
    createdTokenDisplay =
      showCreatedToken ? createdToken : (
        "•".repeat(Math.max(createdToken.length, 24))
      )
  }

  const tokenVisibilityLabel =
    showCreatedToken ? t("hideToken") : t("showToken")
  const tokenVisibilityIcon =
    showCreatedToken ? <EyeOff size={16} /> : <Eye size={16} />

  // Created token reveal dialog
  if (createdToken) {
    return (
      <div className="mx-auto max-w-2xl p-6">
        <div className="rounded-lg border border-green-500/30 bg-green-500/10 p-6">
          <h2 className="mb-2 text-lg font-semibold text-green-400">
            {t("tokenCreated")}
          </h2>
          <p className="mb-4 text-sm text-zinc-400">
            {t("tokenCreatedDescription")}
          </p>
          <div className="mb-4 flex items-center gap-2">
            <code className="flex-1 rounded bg-zinc-800 px-3 py-2 font-mono text-sm text-zinc-200 break-all">
              {createdTokenDisplay}
            </code>
            <button
              type="button"
              onClick={() => setShowCreatedToken((value) => !value)}
              className="shrink-0 rounded bg-zinc-700 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-600"
              aria-label={tokenVisibilityLabel}
              title={tokenVisibilityLabel}
            >
              {tokenVisibilityIcon}
            </button>
            <button
              type="button"
              onClick={() => handleCopy(createdToken)}
              className="shrink-0 rounded bg-zinc-700 px-3 py-2 text-sm text-zinc-200 hover:bg-zinc-600"
            >
              {copied ? t("copied") : t("copyToken")}
            </button>
          </div>
          <button
            type="button"
            onClick={() => {
              setCreatedToken(null)
              setShowCreatedToken(false)
              setShowCreate(false)
              setNewTokenName("")
              setNewTokenExpiry("")
            }}
            className="rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-200 hover:bg-zinc-600"
          >
            {t("done")}
          </button>
        </div>
      </div>
    )
  }

  let tokenListContent: React.ReactNode
  if (loading) {
    tokenListContent = (
      <div className="py-12 text-center text-zinc-500">Loading...</div>
    )
  } else if (tokens.length === 0) {
    tokenListContent = (
      <div className="rounded-lg border border-zinc-700 bg-zinc-800/30 py-12 text-center">
        <p className="text-zinc-400">{t("noTokens")}</p>
        <p className="mt-1 text-sm text-zinc-500">{t("noTokensDescription")}</p>
      </div>
    )
  } else {
    tokenListContent = (
      <div className="overflow-x-auto rounded-lg border border-zinc-700">
        <table className="w-full text-left text-sm">
          <thead className="border-b border-zinc-700 bg-zinc-800/50">
            <tr>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("name")}
              </th>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("prefix")}
              </th>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("createdAt")}
              </th>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("lastUsedAt")}
              </th>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("expiresAt")}
              </th>
              <th className="px-4 py-3 font-medium text-zinc-400">
                {t("actions")}
              </th>
            </tr>
          </thead>
          <tbody className="divide-y divide-zinc-700/50">
            {tokens.map((token) => (
              <tr key={token.id} className="hover:bg-zinc-800/30">
                <td className="px-4 py-3 font-medium text-zinc-200">
                  {token.name}
                </td>
                <td className="px-4 py-3">
                  <code className="rounded bg-zinc-800 px-2 py-0.5 text-xs text-zinc-400">
                    {token.prefix}...
                  </code>
                </td>
                <td className="px-4 py-3 text-zinc-400">
                  {formatDate(token.created_at)}
                </td>
                <td className="px-4 py-3 text-zinc-400">
                  {formatDate(token.last_used_at)}
                </td>
                <td className="px-4 py-3 text-zinc-400">
                  {formatExpiry(token.expires_at)}
                </td>
                <td className="px-4 py-3">
                  <button
                    type="button"
                    onClick={() => handleDelete(token)}
                    className="rounded px-2 py-1 text-xs text-red-400 hover:bg-red-500/10"
                  >
                    {t("delete")}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl">
      {/* Header */}
      <div className="mb-6 flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-zinc-100">{t("title")}</h1>
          <p className="mt-1 text-sm text-zinc-400">{t("description")}</p>
        </div>
        <button
          type="button"
          onClick={() => setShowCreate(true)}
          className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500"
        >
          {t("createToken")}
        </button>
      </div>

      {/* Create form */}
      {showCreate && (
        <div className="mb-6 rounded-lg border border-zinc-700 bg-zinc-800/50 p-4">
          <h3 className="mb-3 text-sm font-medium text-zinc-200">
            {t("createTitle")}
          </h3>
          <div className="flex flex-col gap-3 sm:flex-row sm:items-end">
            <div className="flex-1">
              <label className="mb-1 block text-xs text-zinc-400">
                {t("tokenName")}
              </label>
              <input
                type="text"
                value={newTokenName}
                onChange={(e) => setNewTokenName(e.target.value)}
                placeholder={t("tokenNamePlaceholder")}
                className="w-full rounded border border-zinc-600 bg-zinc-900 px-3 py-2 text-sm text-zinc-200 placeholder-zinc-500 focus:border-blue-500 focus:outline-none"
              />
            </div>
            <div className="w-40">
              <label className="mb-1 block text-xs text-zinc-400">
                {t("expiresIn")}
              </label>
              <select
                value={newTokenExpiry}
                onChange={(e) => setNewTokenExpiry(e.target.value)}
                className="w-full rounded border border-zinc-600 bg-zinc-900 px-3 py-2 text-sm text-zinc-200 focus:border-blue-500 focus:outline-none"
              >
                {EXPIRY_OPTIONS.map((opt) => (
                  <option key={opt.value} value={opt.value}>
                    {t(opt.labelKey)}
                  </option>
                ))}
              </select>
            </div>
            <div className="flex gap-2">
              <button
                type="button"
                onClick={handleCreate}
                disabled={!newTokenName.trim() || creating}
                className="rounded bg-blue-600 px-4 py-2 text-sm font-medium text-white hover:bg-blue-500 disabled:opacity-50"
              >
                {t("create")}
              </button>
              <button
                type="button"
                onClick={() => {
                  setShowCreate(false)
                  setNewTokenName("")
                  setNewTokenExpiry("")
                }}
                className="rounded bg-zinc-700 px-4 py-2 text-sm text-zinc-300 hover:bg-zinc-600"
              >
                {t("cancel")}
              </button>
            </div>
          </div>
        </div>
      )}

      {/* Token list */}
      {tokenListContent}
    </div>
  )
}
