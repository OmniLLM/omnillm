import { existsSync, readFileSync } from "node:fs"
import { join } from "node:path"
import { describe, expect, test } from "bun:test"

const repoRoot = join(import.meta.dir, "..")

function read(relativePath: string): string {
  return readFileSync(join(repoRoot, relativePath), "utf8")
}

describe("omniproxy binary removal", () => {
  test("the duplicate Go command package is absent", () => {
    expect(existsSync(join(repoRoot, "cmd", "omniproxy"))).toBe(false)
  })

  test("build and install workflows produce only omnillm", () => {
    for (const path of [
      "Makefile",
      "scripts/build-desktop-sidecar.ps1",
      "scripts/build-install-binaries.ps1",
      "scripts/build-install-binaries.sh",
    ]) {
      expect(read(path).toLowerCase()).not.toContain("omniproxy")
    }
  })

  test("current usage documentation advertises only omnillm", () => {
    for (const path of ["README.md", "README.zh-CN.md", "desktop/README.md"]) {
      const contents = read(path).toLowerCase()
      expect(contents).not.toContain("go run ./cmd/omniproxy")
      expect(contents).not.toContain("`omniproxy start`")
      expect(contents).not.toContain("| `omniproxy` |")
    }
  })

  test("desktop packaging launches the omnillm sidecar", () => {
    expect(read("desktop/src-tauri/tauri.conf.json")).toContain(
      '"externalBin": ["binaries/omnillm"]',
    )
    expect(read("desktop/src-tauri/src/commands.rs")).toContain('.sidecar("omnillm")')
    expect(read("desktop/src-tauri/capabilities/default.json")).not.toContain("omniproxy")
  })
})
