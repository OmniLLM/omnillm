// Package openaicompat implements a generic OpenAI-compatible provider.
//
// Any upstream that speaks the OpenAI chat completions API can be used as an
// instance of this provider.  Alibaba DashScope, Kimi, and similar services
// are all just configured instances — the same pattern LiteLLM uses.
//
// Wire types are defined in types.go.
// CIF ↔ wire serialization lives in serialization.go.
// The Provider / Adapter structs are in provider.go.
package openaicompat
