package composer

import (
	"strings"
	"testing"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/emoji"
)

// emoji_v17 (off for Karim) gates the eight Unicode 17 emoji in the
// composer's ":" list end to end: ":orc" is Slack's four rows with it off
// and orca first with it forced on, as an Experiments plugin forces it.
// The rows are the emoji-rank harness's "orc" cases (internal/acrank/
// testdata/parity/emoji-rank.json, remote model).
func TestAutocompleteSlackEmojiV17(t *testing.T) {
	app, _ := acSlackApp(t)
	t.Cleanup(func() { emoji.SetV17(false) })
	off := "racing_motorcycle flute harp auto_rickshaw"
	got := acSettle(t, newAutocompleteFor(t, app), ":orc", func(n []string) bool { return strings.Join(n, " ") == off })
	t.Logf(":orc, emoji_v17 off: %v", got)
	app.SetExperimentGroup(core.ExpEmojiV17, "on")
	got = acSettle(t, newAutocompleteFor(t, app), ":orc", func(n []string) bool { return strings.Join(n, " ") == "orca "+off })
	t.Logf(":orc, emoji_v17 on: %v", got)
	app.ClearExperimentGroup(core.ExpEmojiV17)
	acSettle(t, newAutocompleteFor(t, app), ":orc", func(n []string) bool { return strings.Join(n, " ") == off })
}
