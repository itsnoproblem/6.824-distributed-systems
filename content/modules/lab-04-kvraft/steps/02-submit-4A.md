---
title: Submit part 4A
type: submit
eval:
  workdir: src/kvraft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "4A"]
  timeout: 10m
---

When the part 4A tests pass locally, snapshot and evaluate here.
