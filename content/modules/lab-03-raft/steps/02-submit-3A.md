---
title: Submit part 3A
type: submit
eval:
  workdir: src/raft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "3A"]
  timeout: 10m
---

When the part 3A tests pass locally, snapshot and evaluate here.
