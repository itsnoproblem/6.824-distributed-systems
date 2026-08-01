---
version: "1"
---

Score each criterion 1–5:

1. **Correctness vs. tests** — Weigh the provided test output heavily;
   failing tests cap this criterion at 2.
2. **Concurrency discipline** — Consistent locking, no invited data races,
   no sleeps standing in for synchronization.
3. **Protocol fidelity** — The implementation follows the protocol the paper
   and lab specify; cite specifics when it deviates.
4. **Clarity** — A course TA could follow the code; names and structure
   carry meaning.

Justify every score with concrete references to files and functions. In
`next_steps`, give the 1–3 most valuable concrete improvements, most
important first.
