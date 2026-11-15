package popups

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/acrank"
	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/emoji"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/theme"
)

// Slack's emoji_v17 experiment in the picker and the ":" list. Off (Karim's
// account, 2026-11-14) the eight Unicode 17 emoji are nowhere: not in the
// grid, the picker's search or the ":" list, and the picker's groups are
// Slack's 16.0 category table (7Kvt) as it lists them. Forced on through
// core, as the Experiments plugins do, they are back, the groups are the
// 17.0 table (6tVA) and the images come from the CDN's 17.0 set. The ":"
// rows are Slack's own, the emoji-rank harness (internal/acrank/testdata/
// parity/emoji-rank.json: "tu cold fallback model", "orc" and their
// emoji_v17 on twins).
func TestEmojiV17PickerAndList(t *testing.T) {
	a := core.New(slack.New(slack.Creds{}), store.New(""))
	t.Cleanup(func() { emoji.SetV17(false) })
	h := &tzHost{a: a, th: theme.New(false)}
	o := acrank.Options{Source: acrank.SourceTexty, Limit: 25, Weights: acrank.FallbackWeights, Now: time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC),
		Passthrough: true, DemotePassthrough: true, EmojiExactBoost: true, ExactNameGroup: "treatment_5"}
	unicode17 := []string{"ballet_dancer", "distorted_face", "fight_cloud", "hairy_creature", "landslide", "orca", "treasure_chest", "trombone"}
	// the picker's group sizes: Slack's tables less "Skin Tones"
	sizes := map[bool][]int{false: {503, 159, 131, 218, 85, 264, 224, 270}, true: {507, 160, 131, 219, 85, 266, 224, 270}}
	run := func(on bool, tu25 string, orc []string, objects ...string) {
		t.Helper()
		x, full := epIndexFor(h)
		if !full || x.key.v17 != on || !x.current() {
			t.Fatalf("v17 %v: index of set v17=%v, complete %v", on, x.key.v17, full)
		}
		for g := epGroupSmileys; g <= epGroupFlags; g++ {
			if want := sizes[on][g-epGroupSmileys]; len(x.group[g]) != want {
				t.Errorf("v17 %v: %s has %d emoji, Slack's table %d", on, epGroupNames[g], len(x.group[g]), want)
			}
		}
		for _, n := range unicode17 {
			_, listed := x.lookup(n)
			found := slices.ContainsFunc(SearchEmoji(h, n, 0), func(m EmojiMatch) bool { return m.Name == n })
			if listed != on || found != on {
				t.Errorf("v17 %v: %s in the index %v, in the picker's search %v", on, n, listed, found)
			}
		}
		// shrug is in no category: searchable, never in the grid
		if id, ok := x.lookup("shrug"); !ok || slices.Contains(x.group[epGroupSmileys], id) {
			t.Errorf("v17 %v: shrug in the index %v, in the Smileys & People grid", on, ok)
		}
		names := func(ids []int32) string { return strings.Join(epNames(x, ids), " ") }
		for _, want := range objects {
			if !strings.Contains(names(x.group[epGroupObjects]), want) {
				t.Errorf("v17 %v: Objects lacks the run %q", on, want)
			}
		}
		dancers, _ := x.lookup("dancers")
		if x.hasSkins(dancers) != on {
			t.Errorf("v17 %v: dancers have tones: %v", on, x.hasSkins(dancers))
		}
		base := emoji.CDN
		if on {
			base = emoji.CDN17
		}
		if plus, _ := x.lookup("+1"); x.url(plus, 3) != base+"google-medium/1f44d-1f3fc.png" && x.url(plus, 3) != base+"apple-medium/1f44d-1f3fc.png" {
			t.Errorf("v17 %v: +1 at tone 3 is %s", on, x.url(plus, 3))
		}
		ms := epMatchNames(AutocompleteEmoji(h, "tu", o))
		if len(ms) != 25 || ms[24] != tu25 {
			t.Errorf("v17 %v: :tu row 25 = %v, Slack %s", on, ms[min(24, len(ms)-1):], tu25)
		}
		ro := o
		ro.Weights = epRemoteModel(t)
		if got := epMatchNames(AutocompleteEmoji(h, "orc", ro)); strings.Join(got, " ") != strings.Join(orc, " ") {
			t.Errorf("v17 %v: :orc\n got  %v\n want %v", on, got, orc)
		}
	}
	orcOff := []string{"racing_motorcycle", "flute", "harp", "auto_rickshaw"}
	run(false, "person_in_tuxedo", orcOff,
		"saxophone accordion guitar musical_keyboard trumpet violin", "label moneybag coin yen")
	a.SetExperimentGroup(core.ExpEmojiV17, "on")
	run(true, "hairy_creature", append([]string{"orca"}, orcOff...),
		"saxophone trumpet trombone accordion guitar musical_keyboard violin", "label coin moneybag treasure_chest yen")
	a.ClearExperimentGroup(core.ExpEmojiV17)
	run(false, "person_in_tuxedo", orcOff, "musical_keyboard trumpet violin")
}

// An open picker takes the other set's index when the set changes.
func TestEmojiPickerFollowsEmojiSet(t *testing.T) {
	a := core.New(slack.New(slack.Creds{}), store.New(""))
	t.Cleanup(func() { emoji.SetV17(false) })
	p := NewEmojiPicker(&tzHost{a: a, th: theme.New(false)}, nil)
	p.refreshIndex()
	if p.idx == nil || p.idx.key.v17 {
		t.Fatal("the picker opened on the 17.0 set")
	}
	a.SetExperimentGroup(core.ExpEmojiV17, "on")
	p.refreshIndex()
	if !p.idx.key.v17 {
		t.Fatal("the open picker kept the 16.0 set")
	}
	orca, ok := p.idx.lookup("orca")
	if !ok || !slices.Contains(p.lists[epGroupAnimals], orca) {
		t.Error("the open picker's Animals & Nature lacks orca")
	}
}
