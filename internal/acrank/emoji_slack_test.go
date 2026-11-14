package acrank

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/emoji"
	"github.com/some-du6e/clack/internal/frecency"
	"github.com/some-du6e/clack/internal/paritytest"
)

// The composer's ":" list over the whole standard emoji set, checked
// against Slack's own code. The expected rows were printed by a node
// harness that ran autocompleteEmoji's local Searcher path from Slack's
// web client, build efbd526e8fb3 (client-boot bbd3a8c1f7bb6f10): the
// SearcherMatcher (1uar: scoreEmojiNameAgainstQuery, matchKeywords), the
// SearcherSorter (j8FV: the group sort, preferExactMatch, frecency RI+6
// and bonus points), the model Sorter (I/0x: prepSuggestions, gSR8
// features, 94aK LinearModel, X46X fallback, J3WV rules,
// boostExactMatches) and the AutocompleteFeatures defaults (1Fxd), with
// Karim's experiments of 2026-11-14 (emoji_v17 off, emoji_exact_match_
// boost on), at 2026-11-14 12:00 UTC. The candidates are Clack's set
// (standard.json.gz) in Slack's consolidated order with Slack's English
// keywords (52eK). The standard set's rows are a parity snapshot
// (testdata/parity/emoji-rank.json, docs/PARITY.md) taken on Slack's own
// set, which lacks the 8 Unicode 17 emoji Clack's has (emoji_v17 is off
// for Karim): with the fallback model hairy_creature takes ":tu"'s row
// 25 in Clack, so those two cases are known divergences. The custom
// emoji cases below are still inline.
//
// ":tu" never offers +1/thumbsup in Slack: a query under 3 letters may
// skip no letters (scoreEmojiNameAgainstQuery), "thumbsup" has an "h"
// between the t and the u, and none of +1's keywords starts with "tu".
// Frecency only scores rows that matched, so no use of it, even picks
// stored under "tu" itself, brings it in.

var tSlackNow = time.Date(2026, 11, 14, 12, 0, 0, 0, time.UTC)

// slackEmojiCands is popups.AutocompleteEmoji's candidate list without
// its prefilters: every standard emoji in Slack's order, ids "E"+unicode,
// keywords normalized.
func slackEmojiCands(t *testing.T) []Cand {
	t.Helper()
	var f struct {
		Emoji []struct {
			Names []string `json:"n"`
			Image string   `json:"i"`
		} `json:"emoji"`
	}
	if err := json.Unmarshal(emoji.StandardData(), &f); err != nil {
		t.Fatal(err)
	}
	type row struct {
		names []string
		image string
		pos   int
	}
	var rows []row
	for i, e := range f.Emoji {
		if len(e.Names) == 0 || e.Image == "" {
			continue
		}
		p := emoji.SlackOrder(e.Names[0])
		if p < 0 {
			p = 1<<20 + i
		}
		rows = append(rows, row{e.Names, e.Image, p})
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].pos < rows[b].pos })
	out := make([]Cand, 0, len(rows))
	for _, r := range rows {
		var kws []string
		for _, k := range emoji.Keywords(r.names[0]) {
			kws = append(kws, Normalize(k))
		}
		out = append(out, Cand{Kind: Emoji, ID: "E" + r.image, Name: r.names[0], Names: r.names, Keywords: kws})
	}
	return out
}

// emojiRankIn is a case's input in testdata/parity/emoji-rank.json: the
// query, the model ("remote": the search.autocomplete.model weights in
// testdata; "fallback": FallbackWeights) and the frecency pref, if any.
type emojiRankIn struct {
	Q        string         `json:"q"`
	Model    string         `json:"model"`
	Frecency []emojiRankUse `json:"frecency,omitempty"`
}

// emojiRankUse is a frecency record: its key, the emoji's id, the count
// and how long before tSlackNow each visit was (time.ParseDuration). +1
// used a lot is reactions recording only its id key (text ""); the cases
// also store +1 or turtle picks under "tu" itself.
type emojiRankUse struct {
	Key   string   `json:"key"`
	ID    string   `json:"id"`
	Count int      `json:"count"`
	Ago   []string `json:"ago"`
}

// frecencyOf builds the frecency pref the records describe (nil: none).
func frecencyOf(t *testing.T, uses []emojiRankUse) *frecency.Cache {
	if len(uses) == 0 {
		return nil
	}
	pref := map[string]any{}
	for _, u := range uses {
		visits := make([]int64, len(u.Ago))
		for i, a := range u.Ago {
			d, err := time.ParseDuration(a)
			if err != nil {
				t.Fatal(err)
			}
			visits[i] = tSlackNow.Add(-d).UnixMilli()
		}
		pref[u.Key] = map[string]any{"id": u.ID, "count": u.Count, "visits": visits}
	}
	b, err := json.Marshal(pref)
	if err != nil {
		t.Fatal(err)
	}
	return frecency.Parse(b)
}

// The rows are in testdata/parity/emoji-rank.json (`tools/parity.py
// --refresh emoji-rank` runs the harness again). "sh" has 135 matches:
// frecency sorts before the model's cut to 75.
func TestEmojiAutocompleteSlackOracle(t *testing.T) {
	cands := slackEmojiCands(t)
	if len(cands) < 1900 {
		t.Fatalf("standard set: %d emoji", len(cands))
	}
	remote := remoteWeights(t)
	rank := func(t *testing.T, in emojiRankIn) []Result {
		w := remote
		switch in.Model {
		case "remote":
		case "fallback":
			w = FallbackWeights
		default:
			t.Fatalf("model %q", in.Model)
		}
		return Rank(in.Q, cands, Options{Source: SourceTexty, Limit: 25, Weights: w, Frecency: frecencyOf(t, in.Frecency), Now: tSlackNow,
			Passthrough: true, DemotePassthrough: true, EmojiExactBoost: true, ExactNameGroup: "treatment_5"})
	}
	s := paritytest.Load[emojiRankIn, []string](t, "testdata/parity/emoji-rank.json")
	s.Run(t, paritytest.Check[emojiRankIn, []string]{
		Clack: func(t *testing.T, in emojiRankIn) []string {
			got := names(cands, rank(t, in))
			if in.Q == "tu" && (idx(got, "+1") >= 0 || idx(got, "thumbsup") >= 0) {
				t.Errorf("Slack never offers +1 for :tu: %v", got)
			}
			return got
		},
		Explain: func(t *testing.T, in emojiRankIn, got []string) {
			var b strings.Builder
			for i, r := range rank(t, in) {
				fmt.Fprintf(&b, "\n  %2d %-28s score=%.4f frec=%.4f model=%.6f", i+1, got[i], r.Score, r.FrecencyScore, r.ModelScore)
			}
			t.Logf(":%s%s", in.Q, b.String())
		},
	})
}

// Keyword-only matches score Inf, keep the names scan's ratio and combo
// breaks, and tie in the list's order (Inf − Inf is NaN in Slack's sort).
// The same five through the harness, fallback model: tulip (model 2.538),
// then flag-tc, poultry_leg, bath (each -0.700647); ":like" is +1 alone.
func TestEmojiKeywordMatch(t *testing.T) {
	cs := []Cand{
		{Kind: Emoji, ID: "E1f1f9-1f1e8", Name: "flag-tc", Names: []string{"flag-tc"}, Keywords: []string{"flag", "turks and caicos islands"}},
		{Kind: Emoji, ID: "E1f357", Name: "poultry_leg", Names: []string{"poultry_leg"}, Keywords: []string{"turkey", "food"}},
		{Kind: Emoji, ID: "E1f6c0", Name: "bath", Names: []string{"bath"}, Keywords: []string{"tub"}},
		{Kind: Emoji, ID: "E1f337", Name: "tulip", Names: []string{"tulip"}, Keywords: []string{"flower"}},
		{Kind: Emoji, ID: "E1f44d", Name: "+1", Names: []string{"+1", "thumbsup"}, Keywords: []string{"thumb", "up", "like"}},
	}
	got := names(cs, Rank("tu", cs, Options{Now: tSlackNow, Weights: FallbackWeights}))
	if !eq(got, []string{"tulip", "flag-tc", "poultry_leg", "bath"}) {
		t.Errorf(":tu: %v", got)
	}
	rs := Rank("like", cs, Options{Now: tSlackNow, Weights: FallbackWeights})
	if len(rs) != 1 || cs[rs[0].Index].Name != "+1" || rs[0].BestName != "+1" || !isInf(rs[0].Score) {
		t.Errorf(":like: %+v", rs)
	}
	q := []rune(Normalize("tu"))
	if !KeywordsMayMatch([]string{"turks and caicos islands"}, q) || KeywordsMayMatch([]string{"thumb", "up"}, []rune("tx")) ||
		!KeywordsMayMatch([]string{"thumbs up"}, []rune("t u")) || KeywordsMayMatch(nil, q) {
		t.Error("KeywordsMayMatch")
	}
}

func isInf(f float64) bool { return f > 1e308 }

// The same harness with custom emoji in the set, as getConsolidatedEmoji
// (Ak7A) merges them after the standard ones: a custom emoji is a row
// with id "E"+name and no keywords, a custom alias one more name of its
// target. In Karim's workspace tu is "alias:thumbs-up", a custom image,
// so ":tu" is an exact name: score 0, +400 exact bonus (100, and 300 for
// emoji_exact_match_boost), first, shown as the alias. Synthetic custom
// emoji only.
func TestEmojiAutocompleteSlackOracleCustomAlias(t *testing.T) {
	std := slackEmojiCands(t)
	custom := Cand{Kind: Emoji, ID: "Ethumbs-up", Name: "thumbs-up", Names: []string{"thumbs-up", "tu"}}
	withAlias := func(target string, aliases ...string) []Cand {
		cs := append([]Cand(nil), std...)
		for i := range cs {
			if cs[i].Name == target {
				cs[i].Names = append(append([]string(nil), cs[i].Names...), aliases...)
				return cs
			}
		}
		t.Fatalf("no %s", target)
		return nil
	}
	karim := append(append([]Cand(nil), std...), custom)
	more := append(withAlias("tada", "yay"), custom,
		Cand{Kind: Emoji, ID: "Eturbo", Name: "turbo", Names: []string{"turbo"}},
		Cand{Kind: Emoji, ID: "Etux", Name: "tux", Names: []string{"tux"}})
	noAlias := append([]Cand(nil), more...)
	noAlias[len(std)].Names = []string{"thumbs-up"}
	toPlusOne := withAlias("+1", "tu")

	remote := remoteWeights(t)
	tuRemote := []string{"tulip", "root_vegetable", "turkey", "turtle", "tumbler_glass", "lotus", "student", "cactus", "bathtub", "test_tube", "stuck_out_tongue", "studio_microphone", "stuffed_flatbread", "flag-tc", "flag-tm", "flag-tn", "flag-tr", "flag-tv", "poultry_leg", "bath", "auto_rickshaw", "mammoth", "male-student", "fortune_cookie"}
	tuFallback := []string{"tulip", "turkey", "turtle", "tumbler_glass", "lotus", "student", "cactus", "stuck_out_tongue", "studio_microphone", "stuffed_flatbread", "bathtub", "stuck_out_tongue_closed_eyes", "stuck_out_tongue_winking_eye", "test_tube", "fortune_cookie", "statue_of_liberty", "male-student", "man_in_tuxedo", "congratulations", "female-student", "man-gesturing-no", "man-gesturing-ok", "woman_in_tuxedo", "man_with_turban"}
	cases := []struct {
		name, q string
		cands   []Cand
		w       Weights
		want    []string
		best    string // the first row's shown name ("" no exact row)
	}{
		{"tu, Karim's alias", "tu", karim, remote, append([]string{"thumbs-up"}, tuRemote...), "tu"},
		{"tu, Karim's alias, fallback model", "tu", karim, FallbackWeights, append([]string{"thumbs-up"}, tuFallback...), "tu"},
		{"tu with more custom emoji", "tu", more, remote,
			[]string{"thumbs-up", "tux", "tulip", "turbo", "root_vegetable", "turkey", "turtle", "tumbler_glass", "lotus", "student", "cactus", "bathtub", "test_tube", "stuck_out_tongue", "studio_microphone", "stuffed_flatbread", "flag-tc", "flag-tm", "flag-tn", "flag-tr", "flag-tv", "poultry_leg", "bath", "auto_rickshaw", "mammoth"}, "tu"},
		{"tu without the alias", "tu", noAlias, remote,
			[]string{"tux", "tulip", "turbo", "root_vegetable", "turkey", "turtle", "tumbler_glass", "lotus", "student", "cactus", "bathtub", "test_tube", "stuck_out_tongue", "studio_microphone", "stuffed_flatbread", "flag-tc", "flag-tm", "flag-tn", "flag-tr", "flag-tv", "poultry_leg", "bath", "auto_rickshaw", "mammoth", "male-student"}, ""},
		{"tu, an alias of +1", "tu", toPlusOne, remote, append([]string{"+1"}, tuRemote...), "tu"},
		{"tu, an alias of +1, fallback model", "tu", toPlusOne, FallbackWeights, append([]string{"+1"}, tuFallback...), "tu"},
		{"yay, an alias of tada", "yay", more, remote, []string{"tada", "clap", "smiley"}, "yay"},
	}
	// one subtest per case: tools/parity.py counts them (docs/PARITY.md)
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := Options{Source: SourceTexty, Limit: 25, Weights: c.w, Now: tSlackNow,
				Passthrough: true, DemotePassthrough: true, EmojiExactBoost: true, ExactNameGroup: "treatment_5"}
			rs := Rank(c.q, c.cands, o)
			got := names(c.cands, rs)
			if !eq(got, c.want) {
				t.Errorf("%s: :%s\n got  %v\n want %v", c.name, c.q, got, c.want)
				return
			}
			r := rs[0]
			if c.best == "" {
				if r.Exact {
					t.Errorf("%s: an exact first row without the alias: %+v", c.name, r)
				}
				return
			}
			if r.BestName != c.best || !r.Exact || r.Score != 0 || r.FrecencyScore != 400 {
				t.Errorf("%s: first row %+v, want %s exact at score 0, frecency 400 (the exact bonus)", c.name, r, c.best)
			}
		})
	}
}
