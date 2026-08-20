## 1. Specification

- [x] 1.1 Strictly validate proposal, delta specs, design, and tasks.
- [x] 1.2 Obtain human approval before implementation.

## 2. Implementation

- [x] 2.1 Reduce the Makefile to the three desktop orchestration targets.
- [x] 2.2 Add a POSIX sidecar build script matching the Windows script contract.
- [x] 2.3 Update current English, Chinese, and desktop documentation to use direct lifecycle and development commands.
- [x] 2.4 Add regression coverage for the minimal Make target surface.
- [x] 2.5 Remove obsolete Make setup guidance and update repository-local skill commands.

## 3. Verification

- [x] 3.1 Run `bun run spec:check`.
- [x] 3.2 Run relevant Bun lint, typecheck, tests, and build checks.
- [x] 3.3 Run Go vet, build, and race tests.
- [x] 3.4 Build the desktop sidecar through the retained Make target.

## 4. Archive

- [x] 4.1 Archive only after all implementation and verification tasks pass.
