package counter

import (
	"sync"
	"testing"
)

func TestConcurrentIncrements(t *testing.T) {
	c := &Counter{}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				c.Inc()
			}
		}()
	}
	wg.Wait()
	if got := c.Value(); got != 10000 {
		t.Fatalf("Value() = %d, want 10000 — increments were lost", got)
	}
}
