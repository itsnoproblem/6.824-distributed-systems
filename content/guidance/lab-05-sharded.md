Pitfalls to check:

- Configuration changes must be serialized through the log; two groups
  disagreeing on config ownership loses keys.
- Shard migration must carry duplicate-detection state with the shard data.
- A group must reject keys for shards it no longer owns, even mid-migration.
