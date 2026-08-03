package wc

import "testing"

func TestWordCount(t *testing.T) {
	got := WordCount("the quick brown fox jumps over the lazy dog the end")
	if got["the"] != 3 || got["fox"] != 1 || got["end"] != 1 {
		t.Fatalf("counts = %v", got)
	}
	if len(got) != 9 {
		t.Fatalf("distinct words = %d, want 9", len(got))
	}
	if got2 := WordCount("one-two one two"); got2["one"] != 2 || got2["two"] != 2 {
		t.Fatalf("punctuation must split words: %v", got2)
	}
}
