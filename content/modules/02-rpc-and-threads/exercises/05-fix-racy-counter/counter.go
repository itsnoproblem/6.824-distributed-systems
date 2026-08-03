package counter

import "sync"

// Counter counts events from many goroutines. Nothing here is synchronized
// yet — the mutex is waiting for you.
type Counter struct {
	mu sync.Mutex
	n  int
}

func (c *Counter) Inc() { c.n++ }

func (c *Counter) Value() int { return c.n }
