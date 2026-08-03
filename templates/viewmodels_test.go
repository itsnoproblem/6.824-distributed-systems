package templates_test

import (
	"testing"

	"github.com/itsnoproblem/mit-distributed-systems/templates"
)

func TestYouTubeEmbedURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"https://www.youtube.com/watch?v=cQP8WApzIQQ", "https://www.youtube-nocookie.com/embed/cQP8WApzIQQ"},
		{"https://youtu.be/cQP8WApzIQQ", "https://www.youtube-nocookie.com/embed/cQP8WApzIQQ"},
		{"https://www.youtube.com/watch?v=abc123&t=42", "https://www.youtube-nocookie.com/embed/abc123"},
		{"https://vimeo.com/12345", ""},
		{"not a url", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := templates.YouTubeEmbedURL(tc.in); got != tc.want {
			t.Errorf("YouTubeEmbedURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
