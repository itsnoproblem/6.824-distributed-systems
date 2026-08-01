---
title: Submit part 4B
type: submit
eval:
  workdir: src/kvraft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "4B"]
  timeout: 15m
---

When the part 4B tests pass locally, snapshot and evaluate here.
