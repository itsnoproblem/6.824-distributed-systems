package kv

import "sync"

// Store is a concurrency-safe string key/value store. Implement NewStore,
// Get, Put, and Append so the tests pass — every method may be called from
// many goroutines at once.
type Store struct {
	mu   sync.Mutex
	data map[string]string
}

func NewStore() *Store {
	// TODO: construct the store
	return nil
}

// Get returns the value for key, or "" when absent.
func (s *Store) Get(key string) string {
	// TODO
	return ""
}

// Put stores value under key.
func (s *Store) Put(key, value string) {
	// TODO
}

// Append appends value to the current value for key and returns the value
// from before the append.
func (s *Store) Append(key, value string) string {
	// TODO
	return ""
}
