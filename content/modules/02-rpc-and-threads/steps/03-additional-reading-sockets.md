---
title: "Additional reading: sockets from the ground up"
type: reading
---

[Beej's Guide to Network Programming](https://beej.us/guide/bgnet/html/split/)
is the classic free introduction to internet sockets — the plumbing that Go's
`net` and `net/rpc` packages wrap for you. The labs never make you call
`socket()` yourself, but knowing what the runtime is doing pays off every
time a lab test hangs on a connection that never completes.

Worth your time:

- **Socket fundamentals and theory** — what a stream vs. datagram socket
  actually is, and why TCP's guarantees shape every RPC design decision.
- **The core system calls** (`socket()`, `bind()`, `listen()`, `accept()`,
  `connect()`, `send()`, `recv()`) — map each one onto what `net.Listen`
  and `net.Dial` are doing for you in Go.
- **Blocking and multiplexing** — the problem `select()`/`poll()` solve is
  the same one goroutines solve; comparing the two models sharpens your
  understanding of both.

Safe to skim: the C-specific struct handling and the IPv4/IPv6 conversion
minutiae — Go's standard library absorbs all of it.
