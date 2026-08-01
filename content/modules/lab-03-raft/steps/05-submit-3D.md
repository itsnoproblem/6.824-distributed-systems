---
title: Submit part 3D
type: submit
eval:
  workdir: src/raft
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race", "-run", "3D"]
  timeout: 15m
---

When the part 3D tests pass locally, snapshot and evaluate here.
