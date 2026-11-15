package emoji

import (
	"bytes"
	"compress/gzip"
	_ "embed"
	"encoding/json"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// Slack has two standard emoji sets, and the emoji_v17 experiment picks
// the one everything reads (client-boot 1PPX: `"on" ===
// getGroupForUser(state, "emoji_v17")`, build bbd3a8c1f7bb6f10, the only
// place the client names it):
//
//   - getStandardEmojiData (52eK O): qU6j with it on, IYuW without. Every
//     standard emoji Slack draws, autocompletes, resolves by name
//     (getStandardEmoji, Ak7A), names from its characters
//     (getEmojiNameFromUnicodeString, L1nE) or finds in text
//     (getAllNativeEmoji, CY0F) comes from that table, and so does the
//     Add Emoji dialog's list of names already taken.
//   - getStandardEmojiCategories (52eK GU): 6tVA with it on, 7Kvt
//     without: the picker's groups (Categories).
//   - getEmojiVersion (Ak7A): the CDN's 17.0 images with it on, 16.0
//     without (Base).
//
// standard.json.gz is the 17.0 table (qU6j less its five skin-tone-N
// entries). IYuW is the same table in the same order without the eight
// Unicode 17 emoji (v17Only) and without the skin tones of the six emoji
// Unicode 17 gave tones to (v17Skins, multiSkinTone in qU6j only).
// Nothing else differs: IYuW's 1,972 names are qU6j's 1,980 less those
// eight, with the same unicode, aliases and keywords (2026-11-14). The
// 16.0 CDN has none of their images (403).
var (
	v17Only = map[string]bool{
		"ballet_dancer": true, "distorted_face": true, "fight_cloud": true, "hairy_creature": true,
		"landslide": true, "orca": true, "treasure_chest": true, "trombone": true,
	}
	v17Skins = map[string]bool{
		"dancers": true, "men-with-bunny-ears-partying": true, "women-with-bunny-ears-partying": true,
		"wrestlers": true, "man-wrestling": true, "woman-wrestling": true,
	}
)

var useV17 atomic.Bool

// V17 reports whether the 17.0 set is in use: the account's emoji_v17
// group is "on". Core follows the group (SetV17); until it does, the
// 16.0 set is in use, as for an account the experiment doesn't include.
func V17() bool { return useV17.Load() }

// SetV17 picks the 17.0 set (on) or the 16.0 one and reports whether
// that changed it. What was built from the other set (the picker's
// index, the text renderers' emoji tables) is built again on its next
// use.
func SetV17(on bool) bool { return useV17.Swap(on) != on }

// without17 is the 16.0 table: list without v17Only, and v17Skins'
// emoji without their tones.
func without17(list []std) []std {
	out := make([]std, 0, len(list))
	for _, e := range list {
		if len(e.Names) > 0 && v17Only[e.Names[0]] {
			continue
		}
		if len(e.Names) > 0 && v17Skins[e.Names[0]] {
			e.Skins = nil
		}
		out = append(out, e)
	}
	return out
}

var (
	hiddenOnce sync.Once
	hidden17   map[string]bool // unicode (lower case) of the 17.0 set's additions
)

// Hidden reports whether u, an emoji element's unicode ("1facd"), is one
// of the 17.0 set's additions (an emoji or a skin tone) while the 16.0
// set is in use. Slack's Emoji component finds no name for it
// (getEmojiNameFromUnicodeString) and draws its characters
// (renderUnicodeAsEmoji), whatever the element's name.
func Hidden(u string) bool {
	if u == "" || V17() {
		return false
	}
	hiddenOnce.Do(func() {
		once.Do(load)
		hidden17 = map[string]bool{}
		for _, e := range sets[1].list {
			if len(e.Names) == 0 || !v17Only[e.Names[0]] && !v17Skins[e.Names[0]] {
				continue
			}
			if v17Only[e.Names[0]] {
				hidden17[e.Image] = true
			}
			for _, img := range e.Skins {
				hidden17[img] = true
			}
		}
	})
	return hidden17[strings.ToLower(u)]
}

// categories.json.gz is Slack's two category tables
// (getStandardEmojiCategories, client-boot bbd3a8c1f7bb6f10: {"16.0":
// 7Kvt, "17.0": 6tVA}), each a list of [name, [primary names]]: 50 KB
// of JSON, 9 KB gzipped. The 17.0 table adds the eight emoji and puts
// trumpet before accordion and coin before moneybag.
//
//go:embed categories.json.gz
var categoriesGz []byte

// Category is one of getStandardEmojiCategories' groups: its English
// name ("Smileys & People") and its emoji's primary names, in order.
type Category struct {
	Name  string
	Names []string
}

var (
	catOnce sync.Once
	cats    [2][]Category
)

// Categories is the set in use's category table. The picker shows every
// group but "Skin Tones" with exactly these emoji, in this order
// (getTranslatedEmojiCategories, boot-shared bEcp): a standard emoji in
// no group (shrug, the gender-neutral people) is found only by search
// and the ":" list. Don't modify the slices.
func Categories() []Category { return CategoriesFor(V17()) }

// CategoriesFor is Categories for the 17.0 set (v17) or the 16.0 one.
func CategoriesFor(v17 bool) []Category {
	catOnce.Do(loadCategories)
	if v17 {
		return cats[1]
	}
	return cats[0]
}

func loadCategories() {
	r, err := gzip.NewReader(bytes.NewReader(categoriesGz))
	if err != nil {
		return
	}
	b, err := io.ReadAll(r)
	if err != nil {
		return
	}
	var f map[string][][]json.RawMessage
	if json.Unmarshal(b, &f) != nil {
		return
	}
	for i, ver := range [2]string{"16.0", "17.0"} {
		for _, row := range f[ver] {
			var c Category
			if len(row) != 2 || json.Unmarshal(row[0], &c.Name) != nil || json.Unmarshal(row[1], &c.Names) != nil {
				continue
			}
			cats[i] = append(cats[i], c)
		}
	}
}

// Native is the set in use's emoji as characters, for the text renderers
// that draw them as Slack's images: every emoji's and skin tone's code
// points (VS16 dropped) as a string → its image stem, and the longest
// sequence's length in code points. A character the set doesn't list
// stays text, as Slack finds only its set's emoji in text
// (getAllNativeEmoji, CY0F). Don't modify the map.
func Native() (index map[string]string, maxLen int) { return NativeFor(V17()) }

// NativeFor is Native for the 17.0 set (v17) or the 16.0 one.
func NativeFor(v17 bool) (index map[string]string, maxLen int) {
	s := setFor(v17)
	s.natOnce.Do(s.loadNative)
	return s.nat, s.natMax
}

func (s *set) loadNative() {
	idx := make(map[string]string, len(s.list)*3)
	maxLen := 0
	add := func(hexSeq, stem string) {
		var b strings.Builder
		n := 0
		for _, p := range strings.Split(hexSeq, "-") {
			v, err := strconv.ParseUint(p, 16, 32)
			if err != nil {
				return
			}
			if v == 0xFE0F {
				continue
			}
			b.WriteRune(rune(v))
			n++
		}
		if n == 0 {
			return
		}
		if _, ok := idx[b.String()]; !ok {
			idx[b.String()] = stem
		}
		maxLen = max(maxLen, n)
	}
	for _, e := range s.list {
		add(e.Image, e.Image)
		if e.Uni != "" {
			add(e.Uni, e.Image)
		}
		codes := make([]string, 0, len(e.Skins))
		for c := range e.Skins {
			codes = append(codes, c)
		}
		sort.Strings(codes)
		for _, c := range codes {
			add(e.Skins[c], e.Skins[c])
		}
	}
	s.nat, s.natMax = idx, maxLen
}
