import { existsSync, readFileSync, readdirSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, test } from "bun:test"

const repoRoot = join(import.meta.dir, "..")
const worktreeInstruction = `# Worktree cleanup

After completing work in a temporary worktree you created specifically for the task, leave and remove that worktree only after its changes are safely preserved, such as by committing, merging, or explicitly retaining them.

Never remove a pre-existing or user-created worktree, and never discard uncommitted or unmerged changes. If safe removal cannot be confirmed, keep the worktree and report the remaining cleanup.`

function read(relativePath: string): string {
  return readFileSync(join(repoRoot, relativePath), "utf8")
}

describe("spec-driven governance", () => {
  test("preserves worktree safety and mandates OpenSpec", () => {
    const instructions = read("CLAUDE.md")
    expect(instructions).toContain(worktreeInstruction)
    expect(instructions).toContain("Spec-Driven Development (mandatory)")
    expect(instructions).toContain("proposal.md")
    expect(instructions).toContain("tasks.md")
    expect(instructions).toContain("MUST")
  })

  test("provides governance entry points", () => {
    for (const path of ["AGENTS.md", "CONTRIBUTING.md", ".github/pull_request_template.md", "docs/README.md", "openspec/config.yaml"]) {
      expect(existsSync(join(repoRoot, path))).toBe(true)
      expect(read(path).trim().length).toBeGreaterThan(0)
    }
  })

  test("keeps package and CI enforcement wired", () => {
    const packageJson = JSON.parse(read("package.json"))
    expect(packageJson.scripts["spec:gate"]).toBeTruthy()
    expect(packageJson.scripts["spec:validate"]).toContain("--strict")
    expect(packageJson.scripts["spec:check"]).toBeTruthy()

    const ci = read(".github/workflows/ci.yml")
    expect(ci).toContain("fetch-depth: 0")
    expect(ci).toContain("bun run spec:gate")
    expect(ci.match(/needs: spec/g)?.length).toBe(2)
  })

  test("every baseline capability contains requirements and scenarios", () => {
    const currentRoot = join(repoRoot, "openspec", "specs")
    const baselineRoot = join(repoRoot, "openspec", "changes", "2026-08-02-baseline-capture", "specs")
    const specsRoot = readdirSync(currentRoot, { withFileTypes: true }).some((entry) => entry.isDirectory()) ? currentRoot : baselineRoot
    const capabilities = readdirSync(specsRoot, { withFileTypes: true }).filter((entry) => entry.isDirectory())
    expect(capabilities.length).toBeGreaterThan(0)
    for (const capability of capabilities) {
      const content = readFileSync(join(specsRoot, capability.name, "spec.md"), "utf8")
      expect(content).toContain("### Requirement:")
      expect(content).toContain("#### Scenario:")
    }
  })
})
