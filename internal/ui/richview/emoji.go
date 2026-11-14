package richview

import (
	"strings"
	"sync"

	"github.com/some-du6e/clack/internal/emoji"
)

// Unicode emoji inside plain text (mrkdwn text, button labels, attachment
// text) are drawn as Slack's CDN emoji images, like the web client and the
// theme's label renderer; Lato has no emoji glyphs and the system color
// font isn't Slack's set. Only the standard set in use's emoji are
// (emoji.Native): with emoji_v17 off a Unicode 17 emoji stays text, as
// Slack's getAllNativeEmoji doesn't find it.

type emojiSeg struct {
	text string // plain text run, or the emoji's original characters
	stem string // non-empty for an emoji: CDN image stem
}

// splitCache holds splitEmoji's answers per standard set ([0] 16.0, [1]
// 17.0: emoji.V17): a Unicode 17 emoji is an image only in the 17.0 set.
var (
	splitMu    sync.Mutex
	splitCache = [2]map[string][]emojiSeg{{}, {}}
)

func mayStartEmoji(r rune) bool {
	return r >= 0x203C || r == '#' || r == '*' || (r >= '0' && r <= '9') || r == 0xA9 || r == 0xAE
}

// splitEmoji returns text/emoji segments, or nil when s has no emoji.
func splitEmoji(s string) []emojiSeg {
	has := false
	for i := 0; i < len(s); i++ {
		if s[i] >= 0xe2 || s[i] == 0xc2 {
			has = true
			break
		}
	}
	if !has {
		return nil
	}
	v17, si := emoji.V17(), 0
	if v17 {
		si = 1
	}
	splitMu.Lock()
	if segs, ok := splitCache[si][s]; ok {
		splitMu.Unlock()
		return segs
	}
	splitMu.Unlock()
	emojiIndex, emojiMaxLen := emoji.NativeFor(v17)
	rs := []rune(s)
	var segs []emojiSeg
	var txt strings.Builder
	found := false
	for i := 0; i < len(rs); {
		r := rs[i]
		if mayStartEmoji(r) && (r >= 0x203C || (i+1 < len(rs) && (rs[i+1] == 0xFE0F || rs[i+1] == 0x20E3))) {
			var key []rune
			best, bestEnd := "", 0
			for j := i; j < len(rs) && len(key) < emojiMaxLen; j++ {
				if rs[j] == 0xFE0F {
					continue
				}
				key = append(key, rs[j])
				if stem, ok := emojiIndex[string(key)]; ok {
					best, bestEnd = stem, j+1
				}
			}
			if best != "" {
				for bestEnd < len(rs) && rs[bestEnd] == 0xFE0F {
					bestEnd++
				}
				if txt.Len() > 0 {
					segs = append(segs, emojiSeg{text: txt.String()})
					txt.Reset()
				}
				segs = append(segs, emojiSeg{text: string(rs[i:bestEnd]), stem: best})
				found = true
				i = bestEnd
				continue
			}
		}
		txt.WriteRune(r)
		i++
	}
	if !found {
		segs = nil
	} else if txt.Len() > 0 {
		segs = append(segs, emojiSeg{text: txt.String()})
	}
	splitMu.Lock()
	if len(splitCache[si]) > 4096 {
		splitCache[si] = map[string][]emojiSeg{}
	}
	splitCache[si][s] = segs
	splitMu.Unlock()
	return segs
}

// emojiURL is the CDN image for an emoji stem in the Style's emoji style.
func (s *Style) emojiURL(stem string) string {
	if s.EmojiURL != nil {
		return s.EmojiURL(stem)
	}
	return emoji.URL(stem, "")
}
