import { chmodSync, existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs"
import { tmpdir } from "node:os"
import { join } from "node:path"
import { $ } from "bun"
import { describe, expect, test } from "bun:test"

const repoRoot = join(import.meta.dir, "..")

function read(relativePath: string): string {
  return readFileSync(join(repoRoot, relativePath), "utf8")
}

describe("minimal Makefile", () => {
  test("exposes only binary and desktop orchestration targets", () => {
    const targets = [...read("Makefile").matchAll(/^([a-z][a-z-]*):/gm)].map(
      ([, target]) => target,
    )

    expect(targets).toEqual([
      "build",
      "install",
      "uninstall",
      "build-desktop-sidecar",
      "build-desktop",
      "desktop-dev",
    ])
  })

  test("dispatches sidecar builds to platform scripts", () => {
    const makefile = read("Makefile")
    expect(makefile).toContain("scripts/build-desktop-sidecar.sh")
    expect(makefile).toContain("scripts/build-desktop-sidecar.ps1")
  })

  test("dispatches uninstall to exact-name platform scripts", () => {
    const makefile = read("Makefile")
    expect(makefile).toContain("scripts/uninstall-binaries.sh")
    expect(makefile).toContain("scripts/uninstall-binaries.ps1")

    for (const path of [
      "scripts/uninstall-binaries.sh",
      "scripts/uninstall-binaries.ps1",
    ]) {
      const script = read(path)
      expect(script).toContain("omnillm")
      expect(script).toContain("omniproxy")
    }
  })

  test("POSIX uninstall removes only known binaries and tolerates absence", async () => {
    const fixture = join(tmpdir(), `omnillm-uninstall-${crypto.randomUUID()}`)
    const fakeBin = join(fixture, "fake-bin")
    const installBin = join(fixture, "go-bin")
    mkdirSync(fakeBin, { recursive: true })
    mkdirSync(installBin, { recursive: true })

    const fakeGo = join(fakeBin, "go")
    writeFileSync(
      fakeGo,
      `#!/usr/bin/env bash\nif [[ "$2" == "GOBIN" ]]; then printf '%s\\n' '${installBin}'; else printf '%s\\n' '${fixture}/gopath'; fi\n`,
    )
    chmodSync(fakeGo, 0o755)

    for (const name of ["omnillm", "omniproxy", "keep-me"]) {
      writeFileSync(join(installBin, name), name)
    }

    await $`PATH=${`${fakeBin}:${process.env.PATH}`} ${join(repoRoot, "scripts/uninstall-binaries.sh")}`
    await $`PATH=${`${fakeBin}:${process.env.PATH}`} ${join(repoRoot, "scripts/uninstall-binaries.sh")}`

    expect(existsSync(join(installBin, "omnillm"))).toBe(false)
    expect(existsSync(join(installBin, "omniproxy"))).toBe(false)
    expect(existsSync(join(installBin, "keep-me"))).toBe(true)
  })

  test("current documentation uses omnillm for lifecycle operations", () => {
    for (const path of ["README.md", "README.zh-CN.md"]) {
      const contents = read(path)
      for (const target of ["start", "stop", "restart", "status", "logs"]) {
        expect(contents).not.toContain(`make ${target}`)
      }
    }
  })
})
