---
title: "Exercise — Fix: the racy counter"
type: code
code:
  mode: fix
  editable: ["counter.go"]
  readonly: ["counter_test.go"]
  run: ["go", "test", "-race", "."]
  timeout: 2m
---

This counter is incremented from fifty goroutines at once, and it loses
updates. The test runs with the race detector on, so it will tell you
*exactly* where the unsynchronized access happens — read its output before
you touch the code.

The mutex is already sitting in the struct, unused. Your job is to put it to
work so that `Inc` and `Value` are safe to call concurrently. When
`go test -race` passes, the step completes.
