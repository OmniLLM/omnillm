## Context

`internal/commands/client.go` currently reduces provider and virtual-model list
responses to slices of IDs before sending them to a searchable prompt. All
provider-instance and virtual-model entity pickers share those two resolvers.
The provider type picker already displays human-readable labels; other prompt
choices are authentication methods and configuration field options. See
`proposal.md` for the user-facing motivation.

## Goals / Non-Goals

**Goals:**
- Keep display identity and the underlying entity ID independent throughout
  rendering, searching, and selection, including duplicate names.
- Preserve existing behavior for zero or one entity, explicit arguments,
  non-interactive use, and cancelled prompts.
- Make label generation, search, and selected-ID mapping independently testable
  and exercise the real command path in regression tests.

**Non-Goals:**
- Changing the server API, persistence, provider reference semantics, machine
  output, command surface, or adding new interactive commands.

## Decisions

1. Construct structured choices from allowlisted identity fields already present
   in list responses. Show name, optional alias, and full ID for providers, and
   name and full ID for virtual models. Use `subtitle` only when `alias` is
   absent or empty. Skip records without a valid ID as today. Alternatives such
   as fetching each detail endpoint add unnecessary calls and failure modes.
2. Render a label but return the ID stored on the selected item rather than
   parsing a formatted string or resolving the name. Search the same visible
   fields case-insensitively. This prevents duplicate names, aliases, delimiters,
   or ID-looking names from selecting a different entity.
3. Use a shared entity-picker representation for provider and virtual-model
   resolvers, without changing generic authentication or input prompts. Existing
   single-item auto-selection and empty-list errors remain intact.
4. Escape control characters in display-only identity text to prevent terminal
   sequences while preserving the complete visible ID; never print unrecognized
   server fields. Preserve raw IDs for operations. Tests cover both the safe
   display and unchanged selected ID.
5. Add deterministic tests for list construction, search and selection, plus
   representative provider and virtual-model command requests using multiple
   entries. Inject or otherwise control interactive selection in tests without
   depending solely on a manually operated terminal.

## Risks / Trade-offs

- Long IDs make labels wider → retain full IDs for unambiguous selection and
  present names first for readability.
- Older servers omit descriptive fields → fall back to the full ID without an
  extra request or a fabricated name.
- Terminal control sequences in server-supplied metadata → escape control
  characters in display text while retaining unmodified IDs internally.
- Prompt UI regressions may not show up in pure formatting tests → verify the
  prompt's rendered choices and chosen ID through deterministic integration tests.

## Migration Plan

No data migration or server rollout is needed. Deploy the updated CLI; rollback
restores ID-only menus without changing stored identities or server state.
