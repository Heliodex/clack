package fakeslack

import (
	"testing"

	"github.com/some-du6e/clack/internal/paritytest"
)

// TestSlugDisplayURL holds the fake's display_url to the server's: each
// case in testdata/parity/slug-display.json is a live chat.slugifyUrl
// answer (enterprise host, 2026-11-14, synthetic example.com URLs,
// slug_type "link" for all of them). docs/PARITY.md counts it as a
// fake-server check: Clack shows the server's display_url.
func TestSlugDisplayURL(t *testing.T) {
	s := paritytest.Load[string, string](t, "testdata/parity/slug-display.json")
	s.Run(t, paritytest.Check[string, string]{Clack: func(t *testing.T, url string) string { return slugDisplayURL(url) }})
}
