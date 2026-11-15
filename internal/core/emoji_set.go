package core

import "github.com/some-du6e/clack/internal/emoji"

// ExpEmojiV17 picks Slack's standard emoji set (client-boot 1PPX, `"on"
// === getGroupForUser(state, "emoji_v17")`): with it on, the 17.0 table,
// categories and CDN images; without it, the 16.0 ones, which lack the
// eight Unicode 17 emoji (orca, trombone, hairy_creature…) and the new
// tones of dancers and wrestlers. Karim's account isn't in it
// (2026-11-14), so Slack never offers or draws those.
const ExpEmojiV17 = "emoji_v17"

// followEmojiSet points internal/emoji at the set the account's group
// picks. The ":" list, the picker, the composer's typed emoji and the
// emoji in message text all read that set, and what they built from the
// other one is rebuilt on its next use. Called before each
// "experiments" event, so the redraw it causes sees the new set.
func (a *App) followEmojiSet() { emoji.SetV17(a.experiment(ExpEmojiV17) == "on") }
