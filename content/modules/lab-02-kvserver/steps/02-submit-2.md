---
title: Submit part 2
type: submit
eval:
  workdir: src/kvsrv
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race"]
  timeout: 10m
---

When the part 2 tests pass locally, snapshot and evaluate here.
