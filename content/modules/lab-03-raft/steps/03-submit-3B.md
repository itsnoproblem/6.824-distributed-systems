---
title: Submit part 3B
type: submit
eval:
  workdir: src/raft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "3B"]
  timeout: 10m
---

When the part 3B tests pass locally, snapshot and evaluate here.
