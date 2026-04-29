import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { I18nextProvider } from "react-i18next"

import "./index.css"
import "./i18n"
import App from "./App"
import i18n from "./i18n"

const rootEl = document.querySelector("#root")
if (rootEl) {
  createRoot(rootEl).render(
    <StrictMode>
      <I18nextProvider i18n={i18n}>
        <App />
      </I18nextProvider>
    </StrictMode>,
  )
}
