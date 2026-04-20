import { describe, expect, test } from "bun:test"
import { renderToStaticMarkup } from "react-dom/server"

import { ProviderBadge } from "../../frontend/src/components/ProviderBadge"

describe("rendered status components", () => {
  test("ProviderBadge renders label for each variant", () => {
    const active = renderToStaticMarkup(
      <ProviderBadge label="Active" variant="active" />,
    )
    const inactive = renderToStaticMarkup(
      <ProviderBadge label="Inactive" variant="inactive" />,
    )
    const auth = renderToStaticMarkup(
      <ProviderBadge label="Auth" variant="auth" />,
    )
    const error = renderToStaticMarkup(
      <ProviderBadge label="Error" variant="error" />,
    )

    expect(active).toContain("Active")
    expect(inactive).toContain("Inactive")
    expect(auth).toContain("Auth")
    expect(error).toContain("Error")
  })
})
