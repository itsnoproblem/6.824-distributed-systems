---
title: Build it
type: exercise
---

A working order of attack:

1. **Task handout** — coordinator RPC that gives an idle worker a map task;
   worker runs the map function and writes intermediate files partitioned by
   `ihash(key) % nReduce`.
2. **Reduce path** — once all map tasks finish, hand out reduce tasks;
   reducers read their partition from every map output, sort, and write
   `mr-out-*`.
3. **Completion** — coordinator learns tasks finished; `Done()` returns true
   only when all reduce output exists.
4. **Crash tolerance** — re-issue tasks not completed within a timeout;
  make output writes atomic (write temp file, then rename) so duplicated
  tasks are harmless.

Run `bash test-mr.sh` in `src/main` until every test passes — the crash test
is the one that finds real design flaws.
