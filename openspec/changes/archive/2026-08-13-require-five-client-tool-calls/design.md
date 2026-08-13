## Context

The shared deterministic fixture currently provides three sequential exchanges, while the coding-client policy requires only two live calls and names three clients. GitHub Copilot CLI 1.0.79 supports a custom OpenAI Responses provider through environment variables. The current native custom-tool fixes have already passed two-call Claude Code, Codex CLI, and Droid smokes, making five-call sustained validation the remaining acceptance bar.

## Goals / Non-Goals

**Goals:**

- Make five sequential calls the single deterministic and live lower bound.
- Verify each next call happens only after the previous result.
- Include GitHub Copilot CLI custom-provider behavior in the permanent client matrix.
- Keep live execution isolated, bounded, opt-in, and sanitized.

**Non-Goals:**

- Run credentialed CLI smokes in ordinary CI.
- Require one implementation framework for every client.
- Change runtime gateway behavior beyond defects exposed by the stronger tests.

## Decisions

### One shared minimum of five calls

Define a shared minimum-call constant and expand the principal sequential fixture from three to five exchanges. Existing fixture-driven ingestion, serialization, stream, and route tests inherit the stronger history. The manifest test also asserts the exchange count so a name-only change cannot weaken coverage.

Keeping both three- and five-call scenarios was rejected because the principal compatibility row should enforce the normative lower bound directly.

### Client-specific live evidence

Each client uses its native terminal tool and machine-readable output:

- Claude Code: Anthropic Messages with `Bash`.
- Codex CLI: Responses native custom `exec`.
- Droid: Responses `Execute`/custom tool history.
- GitHub Copilot CLI: OpenAI Responses custom provider with `shell`.

Prompts use five distinct markers and prohibit parallel or combined commands. Passing requires exactly five calls, five results in order, terminal pass text, no sixth call, and exit zero.

### Keep client smokes isolated from user configuration

Use temporary settings or per-process environment variables and a temporary patched OmniLLM port. Never write normal client settings or include credentials in output. A five-minute outer timeout is a hard wall-clock failure boundary.

### Extend existing live matrix repeated-tools logic

The provider/model/shape live matrix remains distinct from actual CLI execution. Its repeated-tools scenario performs five tool calls plus terminal continuation for Chat, Messages, and Responses; the CLI smokes separately prove each real harness.

## Risks / Trade-offs

- **Five live calls cost more and take longer** → Keep execution opt-in, bounded, and limited to changes touching tool-call surfaces.
- **Models may combine calls despite prompting** → Treat parallel/combined output as failure and use distinct markers.
- **Client JSON formats evolve** → Parse stable tool-call/result/terminal fields and record client versions in sanitized evidence.
- **Ambient config can contaminate results** → Disable unrelated MCP/custom instructions and use temporary config roots where supported.

## Migration Plan

1. Raise deterministic fixtures and tests first.
2. Update live matrix logic and its unit tests.
3. Run four isolated client smokes against the patched binary.
4. Update policy/documentation, run full gates, sync the spec, and archive.
