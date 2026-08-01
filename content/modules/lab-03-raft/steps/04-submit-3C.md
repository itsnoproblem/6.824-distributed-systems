---
title: Submit part 3C
type: submit
eval:
  workdir: src/raft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "3C"]
  timeout: 10m
---

When the part 3C tests pass locally, snapshot and evaluate here.
