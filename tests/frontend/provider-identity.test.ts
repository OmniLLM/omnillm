import { expect, mock, test } from "bun:test"
import { createElement } from "react"
import { renderToStaticMarkup } from "react-dom/server"

import { ConfirmProvider } from "../../frontend/src/lib/useConfirm"
import {
  isValidProviderIdentity,
  ProviderIdentityEditor,
  submitProviderIdentity,
} from "../../frontend/src/pages/providers/components/ProviderIdentityEditor"
import i18n from "../../frontend/src/i18n"

test("provider ID editor displays the current ID and accessible edit control", () => {
  const html = renderToStaticMarkup(createElement(
    ConfirmProvider,
    null,
    createElement(ProviderIdentityEditor, { id: "old-provider", onSave: async () => true }),
  ))
  expect(html).toContain("old-provider")
  expect(html).toContain('aria-label="Edit ID"')
})

test("ID change validates, confirms, and saves only after approval", async () => {
  const confirm = mock(async () => true)
  const save = mock(async () => true)
  expect(await submitProviderIdentity("old", "bad/id", confirm, save)).toBe("invalid")
  expect(confirm).not.toHaveBeenCalled()
  expect(save).not.toHaveBeenCalled()
  expect(await submitProviderIdentity("old", " new-id ", confirm, save)).toBe("saved")
  expect(confirm).toHaveBeenCalledWith("old", "new-id")
  expect(save).toHaveBeenCalledWith("old", "new-id")
})

test("canceled, unchanged, and failed ID changes do not claim success", async () => {
  const save = mock(async () => false)
  const deny = mock(async () => false)
  expect(await submitProviderIdentity("old", "new", deny, save)).toBe("canceled")
  expect(save).not.toHaveBeenCalled()
  expect(await submitProviderIdentity("old", "old", deny, save)).toBe("unchanged")
  expect(await submitProviderIdentity("old", "new", async () => true, save)).toBe("failed")
})

test("provider ID validation matches the API's URL-safe rules", () => {
  for (const id of ["a", "provider-01", "A.b_c-2", "a".repeat(64)]) {
    expect(isValidProviderIdentity(id)).toBe(true)
  }
  for (const id of ["", "-first", "with space", "path/part", "a".repeat(65)]) {
    expect(isValidProviderIdentity(id)).toBe(false)
  }
})

test("confirmation warns about references outside the provider", () => {
  const warning = i18n.t("providers:identity.confirmMessage", { oldId: "old", newId: "new", lng: "en" })
  expect(warning).toContain("old")
  expect(warning).toContain("new")
  expect(warning).toContain("external clients")
  expect(warning).toContain("saved model selections")
})
