---
title: Submit Lab 1
type: submit
eval:
  workdir: src/main
  globs: ["../mr/*.go"]
  test_cmd: ["bash", "test-mr.sh"]
  timeout: 20m
---

When `test-mr.sh` passes locally, snapshot and evaluate your implementation
here. The evaluation runs the lab's own test harness and then reviews your
`src/mr` code against the rubric.
