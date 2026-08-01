---
title: Read the MapReduce paper
type: reading
---

Read *MapReduce: Simplified Data Processing on Large Clusters* (Dean &
Ghemawat, 2004) — linked above as the paper for this module.

Focus while you read:

- **The programming model (§2)** — why do `map` and `reduce` as pure
  functions make distribution possible at all?
- **Execution overview (§3.1)** — trace one job end to end: input splits,
  the master, intermediate files, the shuffle to reducers.
- **Fault tolerance (§3.3)** — what exactly happens when a worker dies
  mid-task, and why is simply re-running it safe?
- **Stragglers (§3.6)** — backup tasks are a blunt instrument; why do they
  work so well anyway?

Skim the refinements (§4); read the sort benchmark story (§5) for a feel of
the scale this was built for.
