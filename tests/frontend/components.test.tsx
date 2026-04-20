import { describe, expect, mock, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"

import { EmptyState } from "../../frontend/src/components/EmptyState"
import { Spinner } from "../../frontend/src/components/Spinner"

describe("rendered shared components", () => {
  test("EmptyState renders title, description, and action", () => {
    const onClick = mock(() => {})
    const html = renderToStaticMarkup(
      <EmptyState
        icon={<span>!</span>}
        title="No providers"
        description="Add a provider to get started."
        action={{ label: "Add provider", onClick }}
      />,
    )

    expect(html).toContain("No providers")
    expect(html).toContain("Add a provider to get started.")
    expect(html).toContain("Add provider")
  })

  test("Spinner renders loading icon classes", () => {
    const html = renderToStaticMarkup(<Spinner className="extra-class" />)

    expect(html).toContain("animate-spin")
    expect(html).toContain("extra-class")
  })
})
