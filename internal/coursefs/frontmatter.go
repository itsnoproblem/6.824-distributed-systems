package coursefs

import (
	"bytes"
	"fmt"
)

var fmDelim = []byte("---\n")

// splitFrontmatter splits "---\n<yaml>\n---\n<body>" into its two parts.
func splitFrontmatter(b []byte) (fm, body []byte, err error) {
	if !bytes.HasPrefix(b, fmDelim) {
		return nil, nil, fmt.Errorf("missing frontmatter delimiter")
	}
	rest := b[len(fmDelim):]
	end := bytes.Index(rest, []byte("\n---"))
	if end < 0 {
		return nil, nil, fmt.Errorf("unterminated frontmatter")
	}
	fm = rest[:end+1]
	body = rest[end+len("\n---"):]
	body = bytes.TrimPrefix(body, []byte("\n"))
	return fm, body, nil
}
