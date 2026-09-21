import type { ModelInfo } from "../api"

export function isGenerationModel(model: ModelInfo, apiShape: string): boolean {
  return (
    model.capabilities?.generation !== false
    && (!model.api_shape || model.api_shape === apiShape)
  )
}
