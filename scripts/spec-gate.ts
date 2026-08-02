import { existsSync, readFileSync, readdirSync } from "node:fs"
import { basename, join, relative, resolve } from "node:path"

export interface GateInput {
  baseRef?: string
  changedFiles?: string[]
  commitMessage?: string
  repoRoot?: string
}

export interface GateResult {
  baseRef: string
  changeIds: string[]
  codeFiles: string[]
  errors: string[]
  exemption?: string
  messages: string[]
}

type CommandResult = { exitCode: number; stderr: string; stdout: string }

/** Paths that never represent behavior-affecting code. Unrecognized paths fail closed as code. */
const ROOT_NON_CODE = new Set([
  "AGENTS.md",
  "CLAUDE.md",
  "CONTRIBUTING.md",
  "LICENSE",
  "README.md",
  "README.zh-CN.md",
  "VERSION",
  ".gitignore",
])

const MIN_EXEMPTION_REASON_LENGTH = 15

export const GATE_ERRORS = {
  codeWithoutChange: "code changed without a complete active or archived OpenSpec change",
  specsWithoutArchive: "current-state specs changed without an archive in the same diff",
} as const

function run(command: string[], cwd: string): CommandResult {
  const result = Bun.spawnSync(command, { cwd, stderr: "pipe", stdout: "pipe" })
  return {
    exitCode: result.exitCode,
    stderr: result.stderr.toString(),
    stdout: result.stdout.toString(),
  }
}

function gitSucceeds(args: string[], cwd: string): boolean {
  return run(["git", ...args], cwd).exitCode === 0
}

export function isCodePath(path: string): boolean {
  const normalized = path.replaceAll("\\", "/")
  if (!normalized || ROOT_NON_CODE.has(normalized) || normalized.endsWith(".md")) return false
  if (normalized.startsWith("openspec/") || normalized.startsWith("docs/")) return false
  if (normalized.startsWith(".claude/") || normalized === ".github/pull_request_template.md") return false
  if (normalized.startsWith(".github/") && !normalized.startsWith(".github/workflows/") && !normalized.startsWith(".github/actions/")) return false
  if (normalized.includes("/target/")) return false
  return true
}

function resolveBase(repoRoot: string, explicit?: string): string {
  const candidates: Array<() => string | undefined> = [
    () => explicit || process.env.SPEC_GATE_BASE,
    () => {
      const branch = process.env.GITHUB_BASE_REF
      if (!branch) return undefined
      const result = run(["git", "merge-base", `origin/${branch}`, "HEAD"], repoRoot)
      return result.exitCode === 0 ? result.stdout.trim() : undefined
    },
    () => process.env.SPEC_GATE_BEFORE,
    () => {
      const result = run(["git", "merge-base", "origin/master", "HEAD"], repoRoot)
      return result.exitCode === 0 ? result.stdout.trim() : undefined
    },
    () => gitSucceeds(["rev-parse", "--verify", "master"], repoRoot) ? "master" : undefined,
    () => gitSucceeds(["rev-parse", "--verify", "HEAD~1"], repoRoot) ? "HEAD~1" : undefined,
  ]

  for (const candidate of candidates) {
    const value = candidate()
    if (value && gitSucceeds(["rev-parse", "--verify", `${value}^{commit}`], repoRoot)) return value
  }
  throw new Error("Unable to resolve a comparison base. Set SPEC_GATE_BASE to a reachable commit.")
}

function toPathList(stdout: string): string[] {
  return stdout.split("\n").map((path) => path.trim()).filter(Boolean)
}

function changedPaths(repoRoot: string, baseRef: string): string[] {
  const committed = run(["git", "diff", "--name-only", "--diff-filter=ACMRD", `${baseRef}...HEAD`], repoRoot)
  if (committed.exitCode !== 0) throw new Error(committed.stderr.trim() || "git diff failed")
  const working = run(["git", "diff", "--name-only", "--diff-filter=ACMRD", "HEAD"], repoRoot)
  const untracked = run(["git", "ls-files", "--others", "--exclude-standard"], repoRoot)
  return [...new Set([committed, working, untracked].flatMap((result) => toPathList(result.stdout)))]
}

function getChangeIds(paths: string[], segment: "active" | "archive"): string[] {
  const prefix = segment === "active" ? "openspec/changes/" : "openspec/changes/archive/"
  const ids = new Set<string>()
  for (const path of paths) {
    if (!path.startsWith(prefix)) continue
    const rest = path.slice(prefix.length)
    if (!rest.includes("/")) continue
    const id = rest.split("/")[0]
    if (id && (segment === "archive" || id !== "archive")) ids.add(id)
  }
  return [...ids].sort()
}

function walkSpecFiles(root: string): string[] {
  if (!existsSync(root)) return []
  const files: string[] = []
  for (const entry of readdirSync(root, { withFileTypes: true })) {
    const path = join(root, entry.name)
    if (entry.isDirectory()) files.push(...walkSpecFiles(path))
    else if (entry.name === "spec.md") files.push(path)
  }
  return files
}

function validateDelta(path: string, repoRoot: string, errors: string[]): void {
  const content = readFileSync(path, "utf8")
  const displayPath = relative(repoRoot, path)
  if (!/^## (ADDED|MODIFIED|REMOVED|RENAMED) Requirements$/m.test(content)) {
    errors.push(`${displayPath}: missing a delta requirements section`)
  }
  const requirements = [...content.matchAll(/^### Requirement: .+$/gm)]
  for (let index = 0; index < requirements.length; index++) {
    const start = requirements[index].index ?? 0
    const end = requirements[index + 1]?.index ?? content.length
    if (!/^#### Scenario: .+$/m.test(content.slice(start, end))) {
      errors.push(`${displayPath}: requirement lacks a #### Scenario`)
    }
  }
  if (requirements.length === 0) errors.push(`${displayPath}: no requirements found`)
}

function validateActiveChange(repoRoot: string, id: string, errors: string[]): void {
  const root = join(repoRoot, "openspec", "changes", id)
  for (const file of ["proposal.md", "tasks.md"]) {
    if (!existsSync(join(root, file))) errors.push(`${id}: missing ${file}`)
  }
  const specs = walkSpecFiles(join(root, "specs"))
  if (specs.length === 0) errors.push(`${id}: missing delta specs`)
  specs.forEach((path) => validateDelta(path, repoRoot, errors))
  const capabilities = new Set(specs.map((path) => basename(resolve(path, ".."))))
  if (capabilities.size > 1 && !existsSync(join(root, "design.md"))) {
    errors.push(`${id}: design.md is required for changes touching multiple capabilities`)
  }
}

function validateArchive(repoRoot: string, id: string, errors: string[]): string[] {
  const root = join(repoRoot, "openspec", "changes", "archive", id)
  for (const file of ["proposal.md", "tasks.md"]) {
    if (!existsSync(join(root, file))) errors.push(`${id}: archived change is missing ${file}`)
  }
  const tasksPath = join(root, "tasks.md")
  if (existsSync(tasksPath) && /^\s*- \[ \]/m.test(readFileSync(tasksPath, "utf8"))) {
    errors.push(`${id}: archived change contains unchecked tasks`)
  }
  const specs = walkSpecFiles(join(root, "specs"))
  if (specs.length === 0) errors.push(`${id}: archived change is missing delta specs`)
  return specs.map((path) => basename(resolve(path, "..")))
}

function exemptionFrom(message: string): string | undefined {
  const match = message.match(/SPEC-EXEMPT:\s*(.+)/)
  const reason = match?.[1]?.trim()
  return reason && reason.length >= MIN_EXEMPTION_REASON_LENGTH ? reason : undefined
}

export function evaluateGate(input: GateInput = {}): GateResult {
  const repoRoot = input.repoRoot ?? resolve(import.meta.dir, "..")
  const baseRef = resolveBase(repoRoot, input.baseRef)
  const paths = input.changedFiles ?? changedPaths(repoRoot, baseRef)
  const codeFiles = paths.filter(isCodePath)
  const activeIds = getChangeIds(paths, "active")
  const archivedIds = getChangeIds(paths, "archive")
  const errors: string[] = []
  const messages: string[] = []
  const commitMessage = input.commitMessage ?? run(["git", "log", "--pretty=%B", `${baseRef}..HEAD`], repoRoot).stdout
  const exemption = exemptionFrom(commitMessage)

  activeIds.forEach((id) => validateActiveChange(repoRoot, id, errors))
  const archivedCapabilities = new Set(archivedIds.flatMap((id) => validateArchive(repoRoot, id, errors)))

  const changedSpecCapabilities = new Set(paths
    .filter((path) => path.startsWith("openspec/specs/"))
    .map((path) => path.split("/")[2])
    .filter(Boolean))
  if (changedSpecCapabilities.size > 0 && archivedIds.length === 0) {
    errors.push(GATE_ERRORS.specsWithoutArchive)
  }
  if (archivedIds.length > 0 && changedSpecCapabilities.size === 0) {
    errors.push("archived change did not update current-state specs")
  }
  for (const capability of changedSpecCapabilities) {
    if (!archivedCapabilities.has(capability)) {
      errors.push(`current-state capability ${capability} has no matching archived delta`)
    }
  }

  if (codeFiles.length > 0 && activeIds.length === 0 && archivedIds.length === 0 && !exemption) {
    errors.push(GATE_ERRORS.codeWithoutChange)
  }
  if (codeFiles.length === 0) messages.push("No code changes detected; spec artifact requirement skipped.")
  if (exemption) messages.push(`WARNING: SPEC-EXEMPT used: ${exemption}`)

  return {
    baseRef,
    changeIds: [...new Set([...activeIds, ...archivedIds])],
    codeFiles,
    errors,
    exemption,
    messages,
  }
}

function main(): void {
  try {
    const result = evaluateGate()
    console.log(`Spec gate base: ${result.baseRef}`)
    console.log(`Code files: ${result.codeFiles.length}`)
    console.log(`Change IDs: ${result.changeIds.join(", ") || "none"}`)
    result.messages.forEach((message) => console.log(message))
    if (result.errors.length > 0) {
      result.errors.forEach((error) => console.error(`FAIL: ${error}`))
      process.exit(1)
    }
    console.log("PASS: mandatory OpenSpec gate")
  } catch (error) {
    console.error(`FAIL: ${error instanceof Error ? error.message : String(error)}`)
    process.exit(1)
  }
}

if (import.meta.main) main()
