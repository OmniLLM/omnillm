import type { ReactNode } from "react"

import { Field } from "./Field"

interface FormFieldProps {
  label: string
  htmlFor?: string
  description?: string
  error?: string
  required?: boolean
  children: ReactNode
}

/**
 * Thin wrapper around the existing <Field> primitive. Kept as its own
 * component so future form-layout concerns (inline/stacked, two-column
 * grouping, etc.) can evolve here without touching every page.
 */
export function FormField(props: FormFieldProps) {
  return <Field {...props} />
}
