import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"

import {
  Field,
  inputStyle,
  smallInputStyle,
} from "../../frontend/src/components/Field"
import { Section } from "../../frontend/src/components/Section"

describe("rendered config components", () => {
  test("Field renders label and child content", () => {
    const html = renderToStaticMarkup(
      <Field label="API Key">
        <input value="secret" readOnly />
      </Field>,
    )

    expect(html).toContain("API Key")
    expect(html).toContain("value=\"secret\"")
  })

  test("Section renders title, count, and children when open", () => {
    const html = renderToStaticMarkup(
      <Section title="Providers" count={3} defaultOpen>
        <div>Provider content</div>
      </Section>,
    )

    expect(html).toContain("Providers")
    expect(html).toContain(">3<")
    expect(html).toContain("Provider content")
  })

  test("Section hides children when defaultOpen is false", () => {
    const html = renderToStaticMarkup(
      <Section title="Providers" defaultOpen={false}>
        <div>Hidden content</div>
      </Section>,
    )

    expect(html).toContain("Providers")
    expect(html).not.toContain("Hidden content")
  })

  test("input styles expose expected sizing defaults", () => {
    expect(inputStyle.fontFamily).toBe("var(--font-mono)")
    expect(inputStyle.padding).toBe("6px 10px")
    expect(smallInputStyle.fontSize).toBe(11)
    expect(smallInputStyle.background).toBe("var(--color-bg-elevated)")
  })
})
