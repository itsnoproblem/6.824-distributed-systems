---
title: Submit Beta
type: submit
eval:
  workdir: src/beta
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race"]
  timeout: 5m
---

Submit your work.
