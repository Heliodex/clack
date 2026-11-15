package emoji

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// useSet17 switches to the 17.0 set for one test.
func useSet17(t *testing.T) {
	t.Helper()
	SetV17(true)
	t.Cleanup(func() { SetV17(false) })
}

var unicode17 = []string{"ballet_dancer", "distorted_face", "fight_cloud", "hairy_creature", "landslide", "orca", "treasure_chest", "trombone"}

// The 16.0 set (emoji_v17 off, Slack's IYuW) is the 17.0 one without the
// eight Unicode 17 emoji and without the six new toned emoji's tones; the
// 17.0 set (qU6j) has both and loads the CDN's 17.0 images.
func TestEmojiV17Sets(t *testing.T) {
	if V17() {
		t.Fatal("the 17.0 set is in use before anything asked for it")
	}
	off := StandardList()
	for _, n := range unicode17 {
		if IsStandard(n) || StandardUnicode(n) != "" {
			t.Errorf("16.0 set: %s is standard", n)
		}
		if _, _, ok := Standard(n); ok {
			t.Errorf("16.0 set: Standard(%s) resolved", n)
		}
	}
	if img, _, ok := Standard("dancers::skin-tone-3"); !ok || img != "1f46f" {
		t.Errorf("16.0 set: dancers at tone 3 = %q %v, want its untoned 1f46f", img, ok)
	}
	if img, _, _ := Standard("+1::skin-tone-3"); img != "1f44d-1f3fc" {
		t.Errorf("16.0 set: +1 at tone 3 = %q", img)
	}
	if u, _ := StandardURL("grin", "google", ""); u != CDN+"google-medium/1f601.png" || Base() != CDN || URL("1f601", "google") != u {
		t.Errorf("16.0 set: %s, base %s", u, Base())
	}

	useSet17(t)
	on := StandardList()
	if len(on) != len(off)+len(unicode17) {
		t.Errorf("17.0 set %d emoji, 16.0 set %d: want %d more", len(on), len(off), len(unicode17))
	}
	for _, n := range unicode17 {
		if !IsStandard(n) {
			t.Errorf("17.0 set: %s isn't standard", n)
		}
	}
	if img, _, ok := Standard("orca"); !ok || img != "1facd" {
		t.Errorf("17.0 set: orca = %q %v", img, ok)
	}
	if img, _, _ := Standard("dancers::skin-tone-3"); img != "1f46f-1f3fc" {
		t.Errorf("17.0 set: dancers at tone 3 = %q", img)
	}
	if u, _ := StandardURL("orca", "google", "large"); u != CDN17+"google-large/1facd.png" || Base() != CDN17 {
		t.Errorf("17.0 set: %s, base %s", u, Base())
	}
	// the 16.0 list is the 17.0 one in the same order, less the eight
	var kept []string
	for _, e := range on {
		if !slices.Contains(unicode17, e.Names[0]) {
			kept = append(kept, e.Names[0])
		}
	}
	for i, e := range off {
		if i >= len(kept) || kept[i] != e.Names[0] {
			t.Fatalf("16.0 set row %d is %s, the 17.0 order has %v", i, e.Names[0], kept[i:min(i+1, len(kept))])
		}
	}
}

// StandardData is the set in use's table: the 16.0 one drops the eight
// rows and the six emoji's "s" (their tones).
func TestStandardDataFollowsSet(t *testing.T) {
	type row struct {
		Names []string          `json:"n"`
		Image string            `json:"i"`
		Skins map[string]string `json:"s"`
	}
	parse := func(b []byte) map[string]row {
		var f struct {
			Categories []string `json:"categories"`
			Emoji      []row    `json:"emoji"`
		}
		if err := json.Unmarshal(b, &f); err != nil {
			t.Fatal(err)
		}
		if len(f.Categories) == 0 {
			t.Error("no categories")
		}
		m := map[string]row{}
		for _, e := range f.Emoji {
			m[e.Names[0]] = e
		}
		return m
	}
	off, on := parse(StandardData()), parse(StandardDataFor(true))
	if len(on)-len(off) != len(unicode17) {
		t.Errorf("17.0 table %d rows, 16.0 %d", len(on), len(off))
	}
	for _, n := range unicode17 {
		if _, ok := off[n]; ok {
			t.Errorf("16.0 table lists %s", n)
		}
		if _, ok := on[n]; !ok {
			t.Errorf("17.0 table lacks %s", n)
		}
	}
	for n := range v17Skins {
		if len(off[n].Skins) != 0 || len(on[n].Skins) != 5 {
			t.Errorf("%s tones: 16.0 %v, 17.0 %v", n, off[n].Skins, on[n].Skins)
		}
	}
	if len(off["+1"].Skins) != 5 || off["+1"].Image != "1f44d" {
		t.Errorf("16.0 table's +1: %+v", off["+1"])
	}
}

// The picker's groups are Slack's category tables: the 17.0 table adds
// exactly the eight (v17Only came from comparing Slack's two tables) and
// moves trumpet and coin; every name but the skin tones' is a primary
// name of the set.
func TestCategoriesFollowSet(t *testing.T) {
	names := func(cs []Category) map[string]string {
		m := map[string]string{}
		for _, c := range cs {
			for _, n := range c.Names {
				m[n] = c.Name
			}
		}
		return m
	}
	off, on := CategoriesFor(false), CategoriesFor(true)
	if len(off) != 9 || len(on) != 9 || off[0].Name != "Smileys & People" || off[1].Name != "Skin Tones" {
		t.Fatalf("categories: %d and %d", len(off), len(on))
	}
	no, nn := names(off), names(on)
	var added []string
	for n := range nn {
		if _, ok := no[n]; !ok {
			added = append(added, n)
		}
	}
	slices.Sort(added)
	if strings.Join(added, " ") != strings.Join(unicode17, " ") {
		t.Errorf("the 17.0 categories add %v, want %v", added, unicode17)
	}
	objects := func(cs []Category, from, to string) string {
		for _, c := range cs {
			if c.Name == "Objects" {
				i, j := slices.Index(c.Names, from), slices.Index(c.Names, to)
				if i < 0 || j < i {
					return "?"
				}
				return strings.Join(c.Names[i:j+1], " ")
			}
		}
		return "?"
	}
	if got := objects(off, "saxophone", "violin"); got != "saxophone accordion guitar musical_keyboard trumpet violin" {
		t.Errorf("16.0 Objects: %s", got)
	}
	if got := objects(on, "saxophone", "violin"); got != "saxophone trumpet trombone accordion guitar musical_keyboard violin" {
		t.Errorf("17.0 Objects: %s", got)
	}
	if got := objects(off, "label", "yen"); got != "label moneybag coin yen" {
		t.Errorf("16.0 Objects: %s", got)
	}
	if got := objects(on, "label", "yen"); got != "label coin moneybag treasure_chest yen" {
		t.Errorf("17.0 Objects: %s", got)
	}
	for v, cs := range map[bool][]Category{false: off, true: on} {
		SetV17(v)
		for _, c := range cs {
			if c.Name == "Skin Tones" {
				continue
			}
			for _, n := range c.Names {
				if StandardUnicode(n) == "" {
					t.Errorf("V17 %v: %s (%s) isn't a standard emoji of the set", v, n, c.Name)
				}
			}
		}
	}
	SetV17(false)
	if Categories()[0].Name != off[0].Name || len(Categories()[6].Names) != len(off[6].Names) {
		t.Error("Categories isn't the 16.0 table")
	}
}

// Characters: only the set in use's emoji are found in text or typed,
// and an element's unicode of a 17.0 addition is Hidden under 16.0.
func TestNativeFollowsSet(t *testing.T) {
	orca, tone := "\U0001FACD", "\U0001F46F\U0001F3FB"
	idx, _ := Native()
	if _, ok := idx[orca]; ok {
		t.Error("16.0 set: 🫍 is an emoji in text")
	}
	if _, ok := idx[tone]; ok {
		t.Error("16.0 set: toned dancers are an emoji in text")
	}
	if idx["\U0001F44D"] != "1f44d" {
		t.Error("16.0 set: 👍 isn't")
	}
	if _, _, ok := MatchUnicode([]rune(orca), 0); ok {
		t.Error("16.0 set: a typed 🫍 became an emoji")
	}
	if !Hidden("1facd") || !Hidden("1F46F-1F3FB") || !Hidden("1f9d1-1f3fd-200d-1fa70") || Hidden("1f44d") || Hidden("1f46f") || Hidden("") {
		t.Error("16.0 set: Hidden")
	}
	useSet17(t)
	idx, _ = Native()
	if idx[orca] != "1facd" || idx[tone] != "1f46f-1f3fb" {
		t.Errorf("17.0 set: 🫍 %q, toned dancers %q", idx[orca], idx[tone])
	}
	if name, end, ok := MatchUnicode([]rune(orca), 0); !ok || name != "orca" || end != 1 {
		t.Errorf("17.0 set: typed 🫍 = %q %d %v", name, end, ok)
	}
	if Hidden("1facd") {
		t.Error("17.0 set: Hidden(1facd)")
	}
	if rest, ok := TrimCDN(CDN17 + "google-medium/1facd.png"); !ok || rest != "google-medium/1facd.png" {
		t.Errorf("TrimCDN: %q %v", rest, ok)
	}
	if _, ok := TrimCDN("https://example.com/16.0/x.png"); ok {
		t.Error("TrimCDN took another host")
	}
}
