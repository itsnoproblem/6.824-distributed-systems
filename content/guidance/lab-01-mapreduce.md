Lab-specific pitfalls to check:

- The coordinator must tolerate workers that crash mid-task: tasks should be
  re-issued after a timeout, and duplicate completions must be harmless.
- Output must be atomic — look for write-temp-then-rename; direct writes to
  `mr-out-*` are a correctness bug under crashes.
- Intermediate files must be partitioned with `ihash(key) % nReduce`.
- RPC handlers must not hold the coordinator's mutex across blocking calls.
- Crash-test failures in `test-mr.sh` almost always mean missing task
  re-issue or non-atomic output — say so explicitly if the output shows it.
