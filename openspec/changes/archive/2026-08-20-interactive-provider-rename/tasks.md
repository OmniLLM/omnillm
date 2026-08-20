# Tasks

## 1. Specification and Regression Coverage

- [x] 1.1 Strictly validate this OpenSpec proposal, delta spec, design, and
  tasks, and obtain human approval before implementation.
- [x] 1.2 Add focused CLI tests for prompted name/alias values, both prompted
  fields empty, explicit flags skipping prompts, and non-interactive validation.

## 2. Interactive Rename

- [x] 2.1 Add command-stream-aware text prompting for provider rename.
- [x] 2.2 Prompt only in an interactive terminal when all rename metadata flags
  are absent, trim input, and submit only non-empty fields.
- [x] 2.3 Preserve alias/subtitle conflict handling and non-interactive behavior.

## 3. Verification and Archive

- [x] 3.1 Run focused provider CLI tests under the race detector.
- [x] 3.2 Run `bun run spec:check`, `go vet ./...`, `go build ./...`, and
  `go test -race ./...`.
- [x] 3.3 Check every task only after completion, archive with `bunx openspec
  archive interactive-provider-rename --yes`, and confirm archive is the only
  operation updating current-state specs.
