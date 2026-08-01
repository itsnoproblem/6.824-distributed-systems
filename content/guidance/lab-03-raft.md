Pitfalls to check:

- Figure 2 of the Raft paper is a specification, not a suggestion — check
  every rule it states, especially commitIndex advancement and log matching.
- Election timers must be reset only on: granting a vote, receiving
  AppendEntries from the current leader, or starting an election.
- Locks must not be held across RPC calls or channel sends.
- Persistent state (currentTerm, votedFor, log) must be persisted before
  replying to any RPC that changed it.
