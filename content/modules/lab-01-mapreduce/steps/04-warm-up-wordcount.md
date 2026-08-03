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

Before (or while) building the real thing, get the kernel of Lab 1 working
sequentially: count words. This is exactly the map half of the lab's
word-count application, minus the distribution.

A word is a maximal run of letters — `strings.FieldsFunc` with
`unicode.IsLetter` does the splitting; the counting is yours.
