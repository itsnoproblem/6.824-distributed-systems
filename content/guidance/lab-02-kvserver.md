Pitfalls to check:

- Duplicate RPC detection: retried Puts/Appends must not apply twice.
- The reply for a duplicate must match the original reply, not recompute.
- Memory: completed request records must eventually be freed.
