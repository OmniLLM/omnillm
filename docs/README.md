# Documentation authority map

> Documentation in `docs/` is non-normative. The source of truth for current behavior is `openspec/specs/`.

## Current-state specifications

| Capability | Normative specification | Supporting references |
|---|---|---|
| Gateway APIs | `openspec/specs/gateway-api/spec.md` | `README.md`, `CIF_MIGRATION.md` |
| CIF translation | `openspec/specs/cif-translation/spec.md` | `CIF_MIGRATION.md`, `CIF_TESTS.md` |
| Providers | `openspec/specs/providers/spec.md` | `ADDING_A_PROVIDER.md`, provider compatibility reports |
| Routing and virtual models | `openspec/specs/routing-failover/spec.md` | `refactoring/channel-affinity.md` |
| Admin control plane | `openspec/specs/admin-api/spec.md`, `openspec/specs/admin-ui/spec.md` | `STRUCTURED_EDITORS.md`, `MATERIAL_UI.md` |
| Persistence and cache | `openspec/specs/persistence/spec.md`, `openspec/specs/caching/spec.md` | migration and scheduling reports |
| CLI and operations | `openspec/specs/cli-ops-config/spec.md` | `CONFIG_TEMPLATES.md` |
| Security | `openspec/specs/security/spec.md` | security sections in `README.md` |
| Desktop | `openspec/specs/desktop-app/spec.md` | historical desktop design |
| SDD governance | `openspec/specs/spec-governance/spec.md` | `CLAUDE.md`, `CONTRIBUTING.md` |

## Historical material

Dated designs, completed plans, fix reports, and migration narratives record why earlier changes were made. They are useful evidence, but they do not override current-state requirements. When a historical document disagrees with a specification, verify the implementation and update the specification through a normal OpenSpec change.
