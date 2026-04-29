import i18n from "i18next"
import { initReactI18next } from "react-i18next"

import en_about from "./locales/en/about.json"
import en_chat from "./locales/en/chat.json"
// Import translation files
import en_common from "./locales/en/common.json"
import en_config from "./locales/en/config.json"
import en_logging from "./locales/en/logging.json"
import en_nav from "./locales/en/nav.json"
import en_providers from "./locales/en/providers.json"
import en_virtualmodel from "./locales/en/virtualmodel.json"
import zh_about from "./locales/zh/about.json"
import zh_chat from "./locales/zh/chat.json"
import zh_common from "./locales/zh/common.json"
import zh_config from "./locales/zh/config.json"
import zh_logging from "./locales/zh/logging.json"
import zh_nav from "./locales/zh/nav.json"
import zh_providers from "./locales/zh/providers.json"
import zh_virtualmodel from "./locales/zh/virtualmodel.json"

// Helper to load language preference from localStorage
function loadLanguagePreference(): string {
  try {
    const stored = localStorage.getItem("olp-language")
    if (stored === "zh" || stored === "en") {
      return stored
    }
  } catch {
    // ignore storage errors
  }

  // Browser language detection fallback
  try {
    const browserLang = navigator.language.toLowerCase()
    if (browserLang.startsWith("zh")) {
      return "zh"
    }
  } catch {
    // ignore
  }

  // Default to English
  return "en"
}

const resources = {
  en: {
    common: en_common,
    nav: en_nav,
    chat: en_chat,
    providers: en_providers,
    config: en_config,
    virtualmodel: en_virtualmodel,
    logging: en_logging,
    about: en_about,
  },
  zh: {
    common: zh_common,
    nav: zh_nav,
    chat: zh_chat,
    providers: zh_providers,
    config: zh_config,
    virtualmodel: zh_virtualmodel,
    logging: zh_logging,
    about: zh_about,
  },
}

void i18n.use(initReactI18next).init({
  resources,
  lng: loadLanguagePreference(),
  fallbackLng: "en",
  defaultNS: "common",
  ns: [
    "common",
    "nav",
    "chat",
    "providers",
    "config",
    "virtualmodel",
    "logging",
    "about",
  ],
  interpolation: {
    escapeValue: false, // React already prevents XSS
  },
  react: {
    useSuspense: false, // Disable suspense to avoid loading states
  },
})

export { default } from "i18next"
