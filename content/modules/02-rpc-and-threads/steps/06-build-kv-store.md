---
title: "Exercise — Build: a concurrent-safe KV store"
type: code
code:
  mode: create
  editable: ["kv.go"]
  readonly: ["kv_test.go"]
  run: ["go", "test", "-race", "."]
  timeout: 2m
---

Every lab from Lab 2 onward has a key/value store at its heart. Build the
smallest honest version now: `Get`, `Put`, and `Append`, all safe under
concurrent callers.

Read the test file first — it defines the contract, including what `Append`
returns. Run early and often; the race detector is on.
