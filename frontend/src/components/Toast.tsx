import { useCallback } from "react"
import { toast, Toaster } from "sonner"

import { createLogger } from "@/lib/logger"

const log = createLogger("toast")


export function useToast() {
  const showToast = useCallback(
    (message: string, type: "success" | "error" = "success") => {
      log.info(message, { type })
      if (type === "error") {
        toast.error(message)
      } else {
        toast.success(message)
      }
    },
    [],
  )

  return { showToast }
}

export function ToastContainer() {
  return (
    <Toaster
      position="bottom-right"
      richColors
      closeButton
      expand={false}
      duration={3500}
      toastOptions={{
        style: {
          fontFamily: "var(--font-text)",
          fontSize: "13px",
        },
      }}
    />
  )
}
