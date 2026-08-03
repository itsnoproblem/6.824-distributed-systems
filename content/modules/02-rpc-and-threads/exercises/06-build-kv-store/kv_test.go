package kv

import (
	"fmt"
	"sync"
	"testing"
)

func TestBasicOps(t *testing.T) {
	s := NewStore()
	if s == nil {
		t.Fatal("NewStore returned nil")
	}
	if got := s.Get("missing"); got != "" {
		t.Fatalf("Get(missing) = %q, want empty", got)
	}
	s.Put("k", "v1")
	if got := s.Get("k"); got != "v1" {
		t.Fatalf("Get = %q", got)
	}
	if old := s.Append("k", "+v2"); old != "v1" {
		t.Fatalf("Append returned %q, want previous value v1", old)
	}
	if got := s.Get("k"); got != "v1+v2" {
		t.Fatalf("after append: %q", got)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewStore()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", n%4)
			for j := 0; j < 100; j++ {
				s.Put(key, "v")
				s.Append(key, "+")
				_ = s.Get(key)
			}
		}(i)
	}
	wg.Wait()
}
