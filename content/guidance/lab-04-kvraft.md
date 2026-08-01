Pitfalls to check:

- Client operations must be deduplicated across leader changes.
- Reads must go through the log (or leases) — serving stale reads from a
  deposed leader is a linearizability bug.
- Snapshot installation must discard conflicting log state atomically.
