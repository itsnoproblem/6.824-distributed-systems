package exercise

import (
	"testing"
	"time"
)

func TestSlow(t *testing.T) {
	for i := 0; i < 20; i++ {
		t.Logf("tick %d", i)
		time.Sleep(150 * time.Millisecond)
	}
	if Value() != 1 {
		t.Fatal("wrong value")
	}
}
