---
title: "Exercise — Warm-up: sequential word count"
type: code
attribution: "Exercise concept adapted from the MIT 6.5840 Lab 1 (MapReduce) word-count application (CC BY 3.0 US)."
code:
  mode: create
  editable: ["wc.go"]
  readonly: ["wc_test.go"]
  run: ["go", "test", "."]
  timeout: 1m
---

This is Lab 1's word-count application distilled to its sequential kernel:
count words, no coordinator, no workers, no RPCs — just the map function on
its own. It stands on its own regardless of when you land here: tackle it
early as a warm-up before the distributed machinery, or come back to it now
as a quick, low-stakes recap of the same logic you just shipped, stripped of
everything but the part that actually counts words.

A word is a maximal run of letters — `strings.FieldsFunc` with
`unicode.IsLetter` does the splitting; the counting is yours.
