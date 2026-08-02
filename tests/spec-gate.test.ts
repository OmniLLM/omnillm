import { describe, expect, test } from "bun:test"
import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { GATE_ERRORS, evaluateGate, isCodePath } from "../scripts/spec-gate"

function fixture(): string {
  const root = mkdtempSync(join(tmpdir(), "omnillm-spec-gate-"))
  Bun.spawnSync(["git", "init", "-b", "master"], { cwd: root })
  Bun.spawnSync(["git", "config", "user.email", "test@example.com"], { cwd: root })
  Bun.spawnSync(["git", "config", "user.name", "Test"], { cwd: root })
  writeFileSync(join(root, "README.md"), "fixture\n")
  Bun.spawnSync(["git", "add", "."], { cwd: root })
  Bun.spawnSync(["git", "commit", "-m", "fixture"], { cwd: root })
  return root
}

function activeChange(root: string, id = "change-one", capabilities = ["gateway-api"]): string[] {
  const changeRoot = join(root, "openspec", "changes", id)
  mkdirSync(changeRoot, { recursive: true })
  writeFileSync(join(changeRoot, "proposal.md"), "## Why\nFixture\n")
  writeFileSync(join(changeRoot, "tasks.md"), "- [ ] implement\n")
  if (capabilities.length > 1) writeFileSync(join(changeRoot, "design.md"), "## Context\nFixture\n")
  const paths = [`openspec/changes/${id}/proposal.md`, `openspec/changes/${id}/tasks.md`]
  for (const capability of capabilities) {
    const specRoot = join(changeRoot, "specs", capability)
    mkdirSync(specRoot, { recursive: true })
    writeFileSync(join(specRoot, "spec.md"), `## Purpose\nA fixture purpose long enough for strict validation behavior.\n\n## ADDED Requirements\n\n### Requirement: Fixture\nThe system SHALL work.\n\n#### Scenario: Works\n- **WHEN** invoked\n- **THEN** it works\n`)
    paths.push(`openspec/changes/${id}/specs/${capability}/spec.md`)
  }
  if (capabilities.length > 1) paths.push(`openspec/changes/${id}/design.md`)
  return paths
}

describe("spec gate path classification", () => {
  test("classifies behavior-affecting paths", () => {
    expect(isCodePath("internal/server/server.go")).toBe(true)
    expect(isCodePath("frontend/src/App.tsx")).toBe(true)
    expect(isCodePath("package.json")).toBe(true)
    expect(isCodePath(".github/workflows/ci.yml")).toBe(true)
    expect(isCodePath("pkg/new-feature/feature.go")).toBe(true)
    expect(isCodePath("server.ts")).toBe(true)
    expect(isCodePath(".github/actions/build/action.yml")).toBe(true)
  })

  test("exempts specs, governance, and documentation", () => {
    expect(isCodePath("openspec/changes/x/proposal.md")).toBe(false)
    expect(isCodePath("docs/README.md")).toBe(false)
    expect(isCodePath("CLAUDE.md")).toBe(false)
  })
})

describe("mandatory OpenSpec gate", () => {
  test("rejects a code-only change", () => {
    const root = fixture()
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: ["internal/server/server.go"], repoRoot: root })
    expect(result.errors).toContain(GATE_ERRORS.codeWithoutChange)
  })

  test("accepts code with a complete active change", () => {
    const root = fixture()
    const changeFiles = activeChange(root)
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: ["internal/server/server.go", ...changeFiles], repoRoot: root })
    expect(result.errors).toEqual([])
  })

  test("accepts docs-only and spec-only diffs", () => {
    const root = fixture()
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: ["docs/README.md", "openspec/config.yaml"], repoRoot: root })
    expect(result.errors).toEqual([])
    expect(result.messages.join("\n")).toContain("No code changes")
  })

  test("requires design.md when a change spans multiple capabilities", () => {
    const root = fixture()
    const changeFiles = activeChange(root, "change-multi", ["gateway-api", "routing"])
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: changeFiles, repoRoot: root })
    expect(result.errors).toEqual([])

    rmSync(join(root, "openspec", "changes", "change-multi", "design.md"))
    const withoutDesign = evaluateGate({ baseRef: "HEAD", changedFiles: changeFiles, repoRoot: root })
    expect(withoutDesign.errors).toContain(
      "change-multi: design.md is required for changes touching multiple capabilities",
    )
  })

  test("rejects direct current-state spec edits", () => {
    const root = fixture()
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: ["openspec/specs/gateway-api/spec.md"], repoRoot: root })
    expect(result.errors).toContain(GATE_ERRORS.specsWithoutArchive)
  })

  test("rejects archived changes with unchecked tasks", () => {
    const root = fixture()
    const archive = join(root, "openspec", "changes", "archive", "change-one")
    const archiveSpec = join(archive, "specs", "x")
    mkdirSync(archiveSpec, { recursive: true })
    writeFileSync(join(archive, "proposal.md"), "## Why\nFixture\n")
    writeFileSync(join(archive, "tasks.md"), "- [ ] incomplete\n")
    writeFileSync(join(archiveSpec, "spec.md"), "## ADDED Requirements\n")
    const result = evaluateGate({ baseRef: "HEAD", changedFiles: [
      "openspec/changes/archive/change-one/proposal.md",
      "openspec/changes/archive/change-one/tasks.md",
      "openspec/changes/archive/change-one/specs/x/spec.md",
      "openspec/specs/x/spec.md",
    ], repoRoot: root })
    expect(result.errors).toContain("change-one: archived change contains unchecked tasks")
  })

  test("rejects a fake archive and archive without current-state updates", () => {
    const root = fixture()
    const fakeArchive = join(root, "openspec", "changes", "archive", "fake")
    mkdirSync(fakeArchive, { recursive: true })
    writeFileSync(join(fakeArchive, "tasks.md"), "- [x] done\n")
    const fake = evaluateGate({ baseRef: "HEAD", changedFiles: [
      "openspec/changes/archive/fake/tasks.md",
      "openspec/specs/x/spec.md",
    ], repoRoot: root })
    expect(fake.errors).toContain("fake: archived change is missing proposal.md")
    expect(fake.errors).toContain("fake: archived change is missing delta specs")

    const archiveFiles = activeChange(root, "real")
    const activeRoot = join(root, "openspec", "changes", "real")
    const archiveRoot = join(root, "openspec", "changes", "archive", "real")
    mkdirSync(join(archiveRoot, "specs", "gateway-api"), { recursive: true })
    for (const name of ["proposal.md", "tasks.md"]) {
      writeFileSync(join(archiveRoot, name), readFileSync(join(activeRoot, name), "utf8").replace("- [ ]", "- [x]"))
    }
    writeFileSync(join(archiveRoot, "specs", "gateway-api", "spec.md"), readFileSync(join(activeRoot, "specs", "gateway-api", "spec.md"), "utf8"))
    const archivedOnly = evaluateGate({ baseRef: "HEAD", changedFiles: archiveFiles.map((path) => path.replace("openspec/changes/real/", "openspec/changes/archive/real/")), repoRoot: root })
    expect(archivedOnly.errors).toContain("archived change did not update current-state specs")
  })

  test("allows a substantive emergency exemption and warns", () => {
    const root = fixture()
    const result = evaluateGate({
      baseRef: "HEAD",
      changedFiles: ["internal/server/server.go"],
      commitMessage: "fix: revert\n\nSPEC-EXEMPT: urgent security rollback required",
      repoRoot: root,
    })
    expect(result.errors).toEqual([])
    expect(result.exemption).toBe("urgent security rollback required")
    expect(result.messages.join("\n")).toContain("WARNING")
  })

  test("ignores an exemption reason that is too short to justify the bypass", () => {
    const root = fixture()
    const result = evaluateGate({
      baseRef: "HEAD",
      changedFiles: ["internal/server/server.go"],
      commitMessage: "fix: revert\n\nSPEC-EXEMPT: oops",
      repoRoot: root,
    })
    expect(result.exemption).toBeUndefined()
    expect(result.errors).toContain(GATE_ERRORS.codeWithoutChange)
  })
})
