---
title: Lab 1 overview
type: reading
---

You will build a working MapReduce system: a coordinator process that hands
out tasks, and worker processes that execute map and reduce functions, talk
to the coordinator over RPC, and survive worker crashes.

Setup:

1. Clone the course lab repo (instructions on the lab page linked above).
2. Point this app's `LAB_REPO_DIR` at your clone so the submit step can
   snapshot your code and run the lab's tests.

Everything you write lives in `src/mr/`; the test harness is
`src/main/test-mr.sh`.
