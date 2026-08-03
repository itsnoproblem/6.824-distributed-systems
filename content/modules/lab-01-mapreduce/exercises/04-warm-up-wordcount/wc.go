package wc

import (
	"strings"
	"unicode"
)

// WordCount returns how many times each word appears in contents, where a
// word is any maximal run of letters. This mirrors the map function you
// will implement for real in Lab 1's word-count application.
func WordCount(contents string) map[string]int {
	// TODO: split contents into letter-runs and count them.
	_ = strings.FieldsFunc
	_ = unicode.IsLetter
	return nil
}
