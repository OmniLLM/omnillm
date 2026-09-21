import { expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"
import { createElement } from "react"
import { TypeSafeForm } from "../../frontend/src/components/TypeSafeForm"
import { isGenerationModel } from "../../frontend/src/lib/models"
import "../../frontend/src/i18n"

test("TypeSafe setup masks the key and describes evaluations", () => {
  const html = renderToStaticMarkup(createElement(TypeSafeForm, {
    onSubmit: async () => {}, onCancel: () => {}, submitting: false,
  }))
  expect(html).toContain('type="password"')
  expect(html).toContain("System One")
  expect(html).toContain("disabled")
})

test("Chat excludes evaluation models while preserving existing shapes", () => {
  expect(isGenerationModel({ id: "jev", api_shape: "systemone" }, "openai")).toBe(false)
  expect(isGenerationModel({ id: "jev", capabilities: { generation: false } }, "openai")).toBe(false)
  expect(isGenerationModel({ id: "chat" }, "openai")).toBe(true)
  expect(isGenerationModel({ id: "chat", api_shape: "anthropic" }, "openai")).toBe(false)
})
