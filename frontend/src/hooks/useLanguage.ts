import { useCallback } from "react"
import { useTranslation } from "react-i18next"

/**
 * Hook for managing language preference
 * Handles switching between English and Chinese with localStorage persistence
 */
export function useLanguage() {
  const { i18n } = useTranslation()

  const currentLanguage = useCallback(() => {
    return i18n.language
  }, [i18n.language])

  const toggleLanguage = useCallback(() => {
    const next = i18n.language === "en" ? "zh" : "en"
    changeLanguage(next)
  }, [i18n])

  const changeLanguage = useCallback(
    (lang: "en" | "zh") => {
      try {
        void i18n.changeLanguage(lang)
        localStorage.setItem("olp-language", lang)
      } catch {
        // ignore storage errors
        void i18n.changeLanguage(lang)
      }
    },
    [i18n],
  )

  return {
    currentLanguage,
    toggleLanguage,
    changeLanguage,
    isEnglish: i18n.language === "en",
    isChinese: i18n.language === "zh",
  }
}
