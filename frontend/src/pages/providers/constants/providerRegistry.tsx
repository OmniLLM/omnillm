import type { ReactNode } from "react"

export const PROVIDER_ACCENT: Record<string, string> = {
  "github-copilot": "#0a84ff",
  antigravity: "#30d158",
  alibaba: "#ff9f0a",
  "alibaba-modelscope": "#ff6f0a",
  "azure-openai": "#0078d4",
  google: "#4285f4",
  kimi: "#e040fb",
  codex: "#6e40c9",
  "openai-compatible": "#10b981",
}

export const PROVIDER_ICONS: Record<string, ReactNode> = {
  "github-copilot": (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.477 2 2 6.477 2 12c0 4.418 2.865 8.166 6.839 9.489.5.092.682-.217.682-.482 0-.237-.009-.868-.013-1.703-2.782.604-3.369-1.342-3.369-1.342-.454-1.155-1.11-1.463-1.11-1.463-.908-.62.069-.608.069-.608 1.003.07 1.531 1.03 1.531 1.03.892 1.529 2.341 1.087 2.91.832.092-.647.35-1.088.636-1.338-2.22-.253-4.555-1.11-4.555-4.943 0-1.091.39-1.984 1.029-2.683-.103-.253-.446-1.27.098-2.647 0 0 .84-.269 2.75 1.025A9.578 9.578 0 0112 6.836c.85.004 1.705.114 2.504.336 1.909-1.294 2.747-1.025 2.747-1.025.546 1.377.203 2.394.1 2.647.64.699 1.028 1.592 1.028 2.683 0 3.842-2.339 4.687-4.566 4.935.359.309.678.919.678 1.852 0 1.336-.012 2.415-.012 2.743 0 .267.18.578.688.48C19.138 20.161 22 16.416 22 12c0-5.523-4.477-10-10-10z" />
    </svg>
  ),
  antigravity: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
        opacity=".7"
      />
      <path d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z" />
      <path d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z" />
      <path
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
        opacity=".7"
      />
    </svg>
  ),
  alibaba: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
    </svg>
  ),
  "alibaba-modelscope": (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5" />
      <circle cx="12" cy="12" r="2" />
    </svg>
  ),
  "azure-openai": (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path d="M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10c1.19 0 2.34-.21 3.41-.6.39-.14.65-.5.65-.93 0-.55-.45-1-1-1-.24 0-.46.08-.64.21-.82.3-1.7.45-2.59.45-3.86 0-7-3.14-7-7s3.14-7 7-7 7 3.14 7 7c0 .89-.15 1.77-.45 2.59-.13.18-.21.4-.21.64 0 .55.45 1 1 1 .43 0 .79-.26.93-.65.39-1.07.6-2.22.6-3.41C22 6.48 17.52 2 12 2z" />
      <circle cx="12" cy="12" r="3" />
    </svg>
  ),
  google: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path
        d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92c-.26 1.37-1.04 2.53-2.21 3.31v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.09z"
        fill="#4285F4"
      />
      <path
        d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"
        fill="#34A853"
      />
      <path
        d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"
        fill="#FBBC05"
      />
      <path
        d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"
        fill="#EA4335"
      />
    </svg>
  ),
  kimi: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path
        d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"
        stroke="currentColor"
        strokeWidth="2"
        fill="none"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  ),
  codex: (
    <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor">
      <path
        d="M8 6l-6 6 6 6M16 6l6 6-6 6"
        stroke="currentColor"
        strokeWidth="2"
        fill="none"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  ),
}

export const TYPE_NAMES: Record<string, string> = {
  "github-copilot": "GitHub Copilot",
  antigravity: "Antigravity (Google)",
  alibaba: "Alibaba DashScope",
  "alibaba-modelscope": "Alibaba ModelScope",
  "azure-openai": "Azure OpenAI",
  google: "Google Gemini",
  kimi: "Kimi (Moonshot)",
  codex: "Codex",
}

export const PROVIDER_TYPES = [
  "github-copilot",
  "antigravity",
  "alibaba",
  "alibaba-modelscope",
  "azure-openai",
  "google",
  "kimi",
  "codex",
  "openai-compatible",
] as const satisfies ReadonlyArray<string>
