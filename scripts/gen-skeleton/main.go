// Command gen-skeleton writes module.yaml + step stubs for every course unit
// that does not already have a module directory. Existing directories are
// never touched, so hand-authored content survives re-runs.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
)

const schedule = "https://pdos.csail.mit.edu/6.824/schedule.html"

type lecture struct {
	slug, title string
	order       int
}

var lectures = []lecture{
	{"01-introduction", "Lecture 1: Introduction & MapReduce", 10},
	{"02-rpc-and-threads", "Lecture 2: RPC and Threads", 20},
	{"03-gfs", "Lecture 3: GFS", 30},
	{"04-paxos", "Lecture 4: Paxos", 40},
	{"05-go-patterns", "Lecture 5: Go Patterns", 50},
	{"06-raft-1", "Lecture 6: Fault Tolerance — Raft (1)", 60},
	{"07-raft-2", "Lecture 7: Fault Tolerance — Raft (2)", 70},
	{"08-linearizability", "Lecture 8: Consistency & Linearizability", 80},
	{"09-zookeeper", "Lecture 9: ZooKeeper", 90},
	{"10-lab-qa", "Lecture 10: Q&A — Raft Labs", 100},
	{"11-distributed-transactions", "Lecture 11: Distributed Transactions", 110},
	{"12-spanner", "Lecture 12: Spanner", 120},
	{"13-chain-replication", "Lecture 13: Chain Replication", 140},
	{"14-occ-farm", "Lecture 14: Optimistic Concurrency Control — FaRM", 150},
	{"15-verification", "Lecture 15: Verification of Distributed Systems", 160},
	{"16-memcached", "Lecture 16: Cache Consistency — Memcached at Facebook", 170},
	{"17-aws-lambda", "Lecture 17: AWS Lambda", 180},
	{"18-ray", "Lecture 18: Ray", 200},
	{"19-sundr", "Lecture 19: Fork Consistency — SUNDR", 210},
	{"20-bitcoin", "Lecture 20: Peer-to-peer — Bitcoin", 220},
	{"21-byzantine-ft", "Lecture 21: Byzantine Fault Tolerance", 230},
	{"22-project-demos", "Lecture 22: Project Demos", 240},
}

type labPart struct {
	name, workdir, runFilter, timeout string
}

type lab struct {
	slug, title, page string
	order             int
	parts             []labPart
}

var labs = []lab{
	{"lab-01-mapreduce", "Lab 1: MapReduce", "https://pdos.csail.mit.edu/6.824/labs/lab-mr.html", 15, nil},
	{"lab-02-kvserver", "Lab 2: Key/Value Server", "https://pdos.csail.mit.edu/6.824/labs/lab-kvsrv.html", 45,
		[]labPart{{"2", "src/kvsrv", "", "10m"}}},
	{"lab-03-raft", "Lab 3: Raft", "https://pdos.csail.mit.edu/6.824/labs/lab-raft.html", 65,
		[]labPart{
			{"3A", "src/raft", "3A", "10m"}, {"3B", "src/raft", "3B", "10m"},
			{"3C", "src/raft", "3C", "10m"}, {"3D", "src/raft", "3D", "15m"},
		}},
	{"lab-04-kvraft", "Lab 4: Fault-tolerant Key/Value Service", "https://pdos.csail.mit.edu/6.824/labs/lab-kvraft.html", 130,
		[]labPart{{"4A", "src/kvraft", "4A", "10m"}, {"4B", "src/kvraft", "4B", "15m"}}},
	{"lab-05-sharded", "Lab 5: Sharded Key/Value Service", "https://pdos.csail.mit.edu/6.824/labs/lab-shard.html", 190,
		[]labPart{{"5A", "src/shardctrler", "", "10m"}, {"5B", "src/shardkv", "", "20m"}}},
}

func must(err error) {
	if err != nil {
		log.Fatal(err)
	}
}

func skipOrMkdir(dir string) (bool, error) {
	if _, err := os.Stat(dir); err == nil {
		fmt.Printf("skip %s (exists)\n", dir)
		return true, nil
	}
	return false, os.MkdirAll(filepath.Join(dir, "steps"), 0o755)
}

func write(path, content string) error { return os.WriteFile(path, []byte(content), 0o644) }

func writeLecture(root string, l lecture) error {
	dir := filepath.Join(root, l.slug)
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"), fmt.Sprintf(
		"title: %q\nkind: lecture\norder: %d\nlinks:\n  paper: %q\n", l.title, l.order, schedule)))
	must(write(filepath.Join(dir, "steps", "01-read-the-paper.md"),
		`---
title: Read the paper
type: reading
---

Read this lecture's paper (find it on the schedule page linked above).
Full guidance for this module is not yet authored — while reading, capture
in the Notes drawer: the system's goal, its core mechanism, and one design
decision you would question.
`))
	must(write(filepath.Join(dir, "steps", "02-reading-question.md"),
		`---
title: Reading question
type: question
question: |
  Summarize the core idea of this lecture's paper in your own words, then
  name one design decision you find questionable and explain why.
---

Answer from the paper, not from summaries of it.
`))
	must(write(filepath.Join(dir, "steps", "03-wrap-up.md"),
		`---
title: Wrap-up
type: reading
---

Check you can restate the paper's main contribution from memory, then move on.
`))
	return nil
}

func writeLab(root string, lb lab) error {
	dir := filepath.Join(root, lb.slug)
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"), fmt.Sprintf(
		"title: %q\nkind: lab\norder: %d\nlinks:\n  lab: %q\n", lb.title, lb.order, lb.page)))
	must(write(filepath.Join(dir, "steps", "01-overview.md"), fmt.Sprintf(
		`---
title: Overview
type: reading
---

Work through %s per the lab page linked above. Detailed guidance for this
lab is not yet authored; the submit step(s) below still snapshot your code
and run the lab tests.
`, lb.title)))
	for i, p := range lb.parts {
		runFilter := ""
		if p.runFilter != "" {
			runFilter = fmt.Sprintf(", \"-run\", %q", p.runFilter)
		}
		must(write(filepath.Join(dir, "steps", fmt.Sprintf("%02d-submit-%s.md", i+2, p.name)), fmt.Sprintf(
			`---
title: Submit part %s
type: submit
eval:
  workdir: %s
  globs: ["*.go"]
  test_cmd: ["go", "test", "-race"%s]
  timeout: %s
---

When the part %s tests pass locally, snapshot and evaluate here.
`, p.name, p.workdir, runFilter, p.timeout, p.name)))
	}
	return nil
}

func writeProject(root string) error {
	dir := filepath.Join(root, "project")
	if skip, err := skipOrMkdir(dir); skip || err != nil {
		return err
	}
	must(write(filepath.Join(dir, "module.yaml"),
		fmt.Sprintf("title: %q\nkind: project\norder: 125\nlinks:\n  lab: %q\n",
			"Final Project", schedule)))
	must(write(filepath.Join(dir, "steps", "01-proposal.md"),
		`---
title: Proposal
type: reading
---

Pick a distributed-systems idea you can build and evaluate in the remaining
weeks. Write a one-page proposal: the problem, the design sketch, and how
you will know it works. Keep the scope honest — a small system measured well
beats a big system that almost runs.
`))
	must(write(filepath.Join(dir, "steps", "02-report.md"),
		`---
title: Build, measure, report
type: reading
---

Build it, measure it, and write the report: design, what worked, what
surprised you, and what you would do differently. Capture running notes in
the Notes drawer as you go — they become the report outline.
`))
	return nil
}

var guidanceStubs = map[string]string{
	"lab-02-kvserver": `Pitfalls to check:

- Duplicate RPC detection: retried Puts/Appends must not apply twice.
- The reply for a duplicate must match the original reply, not recompute.
- Memory: completed request records must eventually be freed.
`,
	"lab-03-raft": `Pitfalls to check:

- Figure 2 of the Raft paper is a specification, not a suggestion — check
  every rule it states, especially commitIndex advancement and log matching.
- Election timers must be reset only on: granting a vote, receiving
  AppendEntries from the current leader, or starting an election.
- Locks must not be held across RPC calls or channel sends.
- Persistent state (currentTerm, votedFor, log) must be persisted before
  replying to any RPC that changed it.
`,
	"lab-04-kvraft": `Pitfalls to check:

- Client operations must be deduplicated across leader changes.
- Reads must go through the log (or leases) — serving stale reads from a
  deposed leader is a linearizability bug.
- Snapshot installation must discard conflicting log state atomically.
`,
	"lab-05-sharded": `Pitfalls to check:

- Configuration changes must be serialized through the log; two groups
  disagreeing on config ownership loses keys.
- Shard migration must carry duplicate-detection state with the shard data.
- A group must reject keys for shards it no longer owns, even mid-migration.
`,
}

func writeGuidanceStubs(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for slug, body := range guidanceStubs {
		path := filepath.Join(dir, slug+".md")
		if _, err := os.Stat(path); err == nil {
			fmt.Printf("skip %s (exists)\n", path)
			continue
		}
		if err := write(path, body); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	root := "content/modules"
	if _, err := os.Stat(root); err != nil {
		log.Fatalf("run from the repo root: %v", err)
	}
	for _, l := range lectures {
		must(writeLecture(root, l))
	}
	for _, lb := range labs {
		must(writeLab(root, lb))
	}
	must(writeProject(root))
	must(writeGuidanceStubs("content/guidance"))
	fmt.Println("done")
}
