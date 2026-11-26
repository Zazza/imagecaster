# One desktop instance per database

Opening Imagecaster again with the same database activates the existing window instead of starting another queue worker. Different database paths remain independent. Relative paths and symlinks to an existing database or its data folder resolve to the same instance.

After updating from a version without this protection, fully close every old desktop copy once before opening the new binary. The update cannot make an already running old process acquire a lock retroactively.

The OS releases the lock after normal exit or a crash, so a leftover `.instance.lock` file does not block restarting. Do not remove or replace this file while Imagecaster is running: its open file identity protects the queue.

## Implementation and rollback

The guard is acquired before SQLite opens, migrations run or the desktop queue starts. Linux and macOS use `flock`; Windows uses `LockFileEx` beyond the activation metadata range so another process can read that metadata. The lock file is retained, never unlinked on close.

Window activation uses a loopback-only endpoint with a random bearer token, private lock-file metadata, a bounded timeout and no redirects. Failure to notify the first window still prevents a second worker from starting. Requests received before GUI readiness are coalesced into a pending activation.

The MCP executable keeps its existing database access: it does not start the desktop queue worker. Queue recovery itself is unchanged. Roll back by restoring the previous desktop binary; close all newer instances before launching an older version.

Linux process locking, normal/crash restart and GUI activation were exercised. Windows and macOS guard tests were cross-compiled; runtime verification on those platforms remains outstanding.
