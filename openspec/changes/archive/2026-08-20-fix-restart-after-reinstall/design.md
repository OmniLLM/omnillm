## Context

Lifecycle state stores PID, executable path, and Linux process start time. The
start time prevents signaling a new process that reused a PID. The executable
path adds another ownership check. The current implementation obtains that
path with `filepath.EvalSymlinks("/proc/<pid>/exe")`, which follows the proc
link back into the filesystem.

After `go install` replaces the binary, Linux exposes the still-running image
as `/path/to/omnillm (deleted)`. Following that path fails even though the
process is alive and still owns the listening socket.

## Decision

Read `/proc/<pid>/exe` with `os.Readlink`, remove only the kernel-defined
trailing ` (deleted)` marker, and canonicalize the remaining path when it still
exists. If canonicalization fails because the old file no longer exists, keep
the normalized proc path. PID and start ID comparisons remain unchanged.

## Safety

The fix does not discover or kill processes by name or port. Only a process
matching the previously recorded PID, normalized executable path, and start ID
is eligible for SIGTERM. This preserves the existing protection against stale
state and PID reuse.

## Rollout

Processes whose state was deleted by an earlier failed restart need one manual
termination because no trusted identity record remains. Servers started after
the fix retain valid state through subsequent installs and restart normally.
