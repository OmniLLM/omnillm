# Change: Add the TypeSafe System One provider

## Why

OmniLLM cannot currently register TypeSafe credentials or serve TypeSafe's native
typed evaluations. TypeSafe Jev evaluates state with Noul, Choice, and Score
questions; it does not generate chat text, embeddings, or native tool calls.
Treating it as an OpenAI-compatible chat provider would advertise capabilities it
does not implement.

The current TypeSafe documentation specifies `POST /v1/systemone` and authenticated
`GET /v1/models`. A read-only catalog request using the operator's existing
`TYPESAFE_API_KEY` returned HTTP 200 and the aliases `jev-latest` and `jev-preview`.
The key was loaded from `~/.config/typesafe/env` without printing its value.

## What Changes

- Add a distinct `typesafe` API-key provider with persisted instance credentials,
  restart reconstruction, live model discovery, and ordinary lifecycle controls.
- Add authenticated `POST /v1/systemone` accepting TypeSafe's native model, state,
  and questions fields and returning its structured model, answers, and usage.
- Mark TypeSafe models as evaluation-only and reject incompatible chat, Responses,
  Messages, streaming, embeddings, and native tool-call requests explicitly.
- Add TypeSafe setup to the CLI and administration console, including localized
  capability guidance and an environment-key fallback for CLI authentication.
- Reuse provider selection, enablement, authentication, concurrency, and metering
  mechanisms while preserving the existing generation provider behavior.
- Add deterministic evaluation and incompatibility coverage, followed by a bounded
  credentialed smoke test on an isolated gateway after approval.

## Capabilities

- `providers`: extend the catalog, authentication, model discovery, and execution.
- `gateway-api`: add the native System One route and validation/error contract.
- `routing-failover`: distinguish evaluation from generation candidates.
- `cli-ops-config`: support TypeSafe authentication and environment credentials.
- `admin-api`: support TypeSafe lifecycle operations and evaluation metering.
- `admin-ui`: expose TypeSafe setup and its evaluation-only capabilities.
- `model-compatibility-testing`: cover native evaluation and unsupported shapes.

## Runtime Impact

This changes runtime behavior by adding a provider and a public authenticated
endpoint. Existing generation clients retain their current contracts. No database
schema change or third-party SDK dependency is planned. Native evaluations bypass
the generation-only CIF and exact-response cache. Adding the provider does not
make Jev usable as a coding-agent chat model.

## Approval

Human approval received: the user instructed "proceed" after strict validation
passed and the proposal, delta specs, design, and tasks were presented for review.
