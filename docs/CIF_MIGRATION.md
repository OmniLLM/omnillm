# Canonical Internal Format (CIF) Migration Guide

## Overview

The omnillm has been migrated to use a **Canonical Internal Format (CIF)** that sits between all inbound API formats and all provider implementations. This replaces the previous N×M translation matrix with an N+M adapter system.

## Architecture

### Before (Legacy)
- **Anthropic Messages** → `translateToOpenAI()` → Provider → `translateToAnthropic()`
- **Responses API** → `translateRequestToOpenAI()` → Provider → `translateResponseToResponses()`
- **OpenAI Chat** → Provider (passthrough with provider-specific tweaks)

### After (CIF)
- **Anthropic Messages** → `parseAnthropicMessages()` → **CanonicalRequest** → `ProviderAdapter.execute()` → **CanonicalResponse** → `serializeToAnthropic()`
- **Responses API** → `parseResponsesPayload()` → **CanonicalRequest** → `ProviderAdapter.execute()` → **CanonicalResponse** → `serializeToResponses()`
- **OpenAI Chat** → `parseOpenAIChatCompletions()` → **CanonicalRequest** → `ProviderAdapter.execute()` → **CanonicalResponse** → `serializeToOpenAI()`

## Migration Status

✅ **Completed Phases:**
1. **Phase 1**: CIF type definitions (`src/cif/types.ts`)
2. **Phase 2**: Ingestion adapters (`src/ingestion/`)
3. **Phase 3**: Serialization adapters (`src/serialization/`)
4. **Phase 4**: Streaming serializers (`src/serialization/*-stream.ts`)
5. **Phase 5**: Simple provider adapters (Azure OpenAI, Alibaba)
6. **Phase 6**: Complex provider adapters (GitHub Copilot, Antigravity)
7. **Phase 7**: Route handler integration with fallback
8. **Phase 8**: Model name translation migration to adapters
9. **Phase 9**: Legacy cleanup and deprecation

## Provider Adapters

All providers now have `ProviderAdapter` implementations:

| Provider | Adapter | Status |
|----------|---------|---------|
| Azure OpenAI | `AzureOpenAIAdapter` | ✅ Complete |
| Alibaba | `AlibabaAdapter` | ✅ Complete |
| GitHub Copilot | `GitHubCopilotAdapter` | ✅ Complete |
| Antigravity | `AntigravityAdapter` | ✅ Complete |

## Benefits

- **Reduced Complexity**: N+M adapters instead of N×M translations
- **Easier Provider Additions**: Only need one adapter per provider
- **Easier Format Additions**: Only need one ingestion adapter per format
- **Better Type Safety**: Canonical format eliminates format-specific edge cases
- **Cleaner Streaming**: Unified streaming event model
- **Native Feature Support**: Thinking blocks, tool results with names, etc.

## Backward Compatibility

- **Zero Downtime Migration**: Legacy translation path preserved as fallback
- **Gradual Adoption**: Providers without adapters continue using old path
- **Deprecated Functions**: Legacy functions marked `@deprecated` but functional

## Usage

### For New Development
Use CIF path through provider adapters:
```typescript
// Parse inbound format to canonical
const canonicalReq = parseAnthropicMessages(payload)

// Execute through adapter
const canonicalResp = await provider.adapter.execute(canonicalReq)

// Serialize to outbound format  
const response = serializeToAnthropic(canonicalResp)
```

### Legacy Code
Old translation functions still work but are deprecated:
```typescript
// DEPRECATED - but still functional
const openAIPayload = translateToOpenAI(anthropicPayload)
const response = await provider.createChatCompletions(openAIPayload)
const anthropicResponse = translateToAnthropic(response.json())
```

## Future Work

- Remove deprecated functions once all code paths use CIF
- Add more inbound format support (native Gemini)
- Extend CIF for embeddings and other API types
- Performance optimizations for streaming