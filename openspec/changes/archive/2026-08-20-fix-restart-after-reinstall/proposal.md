## Why

Replacing an installed `omnillm` executable while its managed background
server is running changes `/proc/<pid>/exe` to a path ending in ` (deleted)`.
Lifecycle validation currently resolves that proc link through the filesystem;
resolution fails, the live server is treated as stale, and `omnillm restart`
removes its state before starting a second process that fails to bind the same
port.

## What Changes

- Read the kernel's process executable link without requiring its on-disk file
  to still exist.
- Normalize Linux's ` (deleted)` marker before comparing lifecycle identity.
- Continue requiring the recorded PID, executable path, and process start ID
  so PID reuse or an unrelated process cannot be signaled.
- Add deterministic coverage for stop/restart identity after executable
  replacement.

## Capabilities

- `cli-ops-config`: preserve managed restart behavior when the running binary
  has been reinstalled or atomically replaced.

## Impact

The change affects local lifecycle identity validation only. API, provider,
persistence, and server configuration behavior are unchanged. A lifecycle
state already removed by the old buggy restart cannot be reconstructed; the
currently orphaned process requires one explicit cleanup before the corrected
lifecycle behavior takes effect.
