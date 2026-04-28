import { useEffect, useState } from "react"

/**
 * SSR-safe media-query subscription hook.
 * Used to drive responsive shell decisions (drawer vs sidebar, etc.).
 */
export function useMediaQuery(query: string): boolean {
  const get = () => {
    if (typeof globalThis.matchMedia !== "function") return false
    return globalThis.matchMedia(query).matches
  }

  const [matches, setMatches] = useState(get)

  useEffect(() => {
    if (typeof globalThis.matchMedia !== "function") return
    const mql = globalThis.matchMedia(query)
    const onChange = () => setMatches(mql.matches)
    onChange()
    mql.addEventListener("change", onChange)
    return () => mql.removeEventListener("change", onChange)
  }, [query])

  return matches
}
