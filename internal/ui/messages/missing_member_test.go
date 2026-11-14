package messages

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/richview"
	"github.com/some-du6e/clack/internal/ui/theme"
	"github.com/some-du6e/clack/internal/ui/theme/pixeltest"
)

// A sender core can't name is BaseMissingMember in the sender's place
// (missing_member.go), never the id: the unknown stub's bar while
// users/info is due (this core runs no member loop, so it stays due),
// the non-existent bar once users/info leaves the id out, then the name
// when the member is stored; core's "names" event draws the row again
// each time, and the row is as tall as a named sender's throughout. A
// reaction from that member is the reaction tip's subtler bar. Synthetic
// ids.
func TestUnknownSenderDrawsBar(t *testing.T) {
	const ghost = "U0NOBODY0001"
	const tsAlice, tsGhost = "1700000500.000100", "1700000900.000100" // apart: both start a group
	msgs := []slack.Message{
		{Type: "message", TS: tsAlice, User: "UALICE", Text: "the same words"},
		{Type: "message", TS: tsGhost, User: ghost, Text: "the same words"},
	}
	e := newCtxEnvSetup(t, "C0MISSING1", msgs, nil)
	app := e.h.app
	var drawn []string
	labelHook = func(txt string, _ textStyle) { drawn = append(drawn, txt) }
	t.Cleanup(func() { labelHook = nil })
	frame := func() {
		drawn = drawn[:0]
		e.frame()
	}
	check := func(when string, want core.MemberState, name string) {
		t.Helper()
		frame()
		r := e.row(tsGhost)
		if r == nil {
			t.Fatalf("%s: no row", when)
		}
		if got := r.senderMiss(); got != want {
			t.Errorf("%s: sender state %v, want %v", when, got, want)
		}
		if r.nameShown != name {
			t.Errorf("%s: sender label %q, want %q", when, r.nameShown, name)
		}
		timeShown := false
		for _, l := range drawn {
			if strings.Contains(l, ghost) {
				t.Errorf("%s: drew %q", when, l)
			}
			timeShown = timeShown || l == clock(tsTime(tsGhost))
		}
		if !timeShown {
			t.Errorf("%s: no timestamp beside the sender (labels %q)", when, drawn)
		}
		if a, g := itemHeight(e.v.s, tsAlice), itemHeight(e.v.s, tsGhost); a <= 0 || a != g {
			t.Errorf("%s: the row is %dpx, a named sender's %dpx", when, g, a)
		}
		if a := e.row(tsAlice); a == nil || a.nameShown != "alice" || a.senderMiss() != core.MemberKnown {
			t.Errorf("%s: alice's row %+v", when, a)
		}
	}
	tip := func() string {
		return reactionTipLine(e.h, &core.Reaction{Name: "tada", Count: 2, Users: []string{"UBOB", ghost}}, tSelf, false)
	}

	check("unknown", core.MemberUnknown, "")
	if txt := tip(); strings.Contains(txt, ghost) || !strings.Contains(txt, tipMissing) || !strings.Contains(txt, "bob") {
		t.Errorf("unknown reactor's tip %q, want bob and the loading bar's mark", plainTip(txt))
	}

	// users/info left it out: a non-existent member
	app.S.PutNonexistent([]string{ghost})
	ApplyEvent(app, core.Event{Type: "names", Users: []string{ghost}})
	check("non-existent", core.MemberNonExistent, "")
	if txt := tip(); strings.Contains(txt, ghost) || !strings.Contains(txt, tipMissingGone) {
		t.Errorf("non-existent reactor's tip %q, want the outlined bar's mark", plainTip(txt))
	}

	// stored after all (a user_change, a later answer): the name
	app.S.PutUsers([]slack.User{{ID: ghost, Name: "ghosty", RealName: "Ghost Example", Profile: slack.Profile{DisplayName: "ghosty"}}})
	ApplyEvent(app, core.Event{Type: "names", Users: []string{ghost}})
	check("stored", core.MemberKnown, "ghosty")
	if txt := plainTip(tip()); txt != "bob and ghosty reacted with :tada:" {
		t.Errorf("stored reactor's tip %q", txt)
	}
}

// The reaction tip's missing reactor is one token, a 120px bar wide, that
// the comma after it stays with.
func TestReactionTipMissingToken(t *testing.T) {
	bold := textStyle{size: 13, lineH: 16.25}
	toks := tipTokens(tipRuns(reactionTipRuns(tipUsers, "users", listify([]string{tipMissing, "bob", tipMissingGone}), "emoji", ":tada:"), bold, bold))
	var bars []tipToken
	for i, tk := range toks {
		if tk.bar {
			bars = append(bars, tk)
			if i+1 < len(toks) && toks[i+1].s == "," && toks[i+1].brk {
				t.Errorf("a line may break between the bar and its comma: %+v", toks[i+1])
			}
		}
	}
	if len(bars) != 2 || bars[0].s != tipMissing || bars[1].s != tipMissingGone {
		t.Fatalf("bars %+v in %+v", bars, toks)
	}
	if !bars[1].brk {
		t.Error("no break opportunity before the second bar")
	}
}

// TestMissingMemberShots (MESSAGES_AUDIT_SHOTS_DIR, the
// messages-audit-shots workflow) draws messages from members core can't
// name, light and dark: the unknown stub's bar in the sender's place and
// a mention's beside a named one, a non-existent member's outlined bar;
// and a reaction tip naming a member, the stub and a non-existent member.
// Synthetic ids and text.
func TestMissingMemberShots(t *testing.T) {
	dir := os.Getenv("MESSAGES_AUDIT_SHOTS_DIR")
	if dir == "" {
		t.Skip("MESSAGES_AUDIT_SHOTS_DIR not set (the messages-audit-shots workflow)")
	}
	const ghost, gone, other = "U0NOBODY0001", "U0NOBODY0002", "U0NOBODY0003"
	msgs := []slack.Message{
		{Type: "message", TS: "1700000500.000100", User: "UALICE", Text: "the release notes are in"},
		{Type: "message", TS: "1700000900.000100", User: ghost, Text: "thanks <@" + other + "> and <@UBOB>, shipping now"},
		{Type: "message", TS: "1700001300.000100", User: gone, Text: "a member users/info didn't return"},
	}
	t.Cleanup(func() { richview.SetMemberRead(nil) })
	for _, dark := range []bool{false, true} {
		e := newCtxEnvSetup(t, "C0MISSING2", msgs, func(app *core.App) {
			app.S.PutNonexistent([]string{gone})
			// the mention hook as the ui root installs it
			// (internal/ui/lazy_users.go), before the rows lay out
			richview.SetMemberRead(func(id string) (string, richview.MemberMiss) {
				u, ok := app.S.User(id)
				switch {
				case ok:
					return store.DisplayName(u), richview.MemberLoaded
				case app.MissingMember(id) == core.MemberNonExistent:
					return "", richview.MemberGone
				}
				return "", richview.MemberLoading
			})
		})
		th := theme.New(dark)
		e.h.th = th
		const w, h = 900, 500
		ops := new(op.Ops)
		f := func() { shotFrame(e.v, th, w, h, ops) }
		loadList(e.v, f)
		for i := 0; i < 10; i++ {
			f()
			time.Sleep(10 * time.Millisecond)
		}
		f()
		img := pixeltest.Render(t, w, h, ops)
		y0, y1 := rowsSpan(e.v.s)
		mode := map[bool]string{false: "light", true: "dark"}[dark]
		savePNG(t, filepath.Join(dir, "missing-member-rows-"+mode+".png"), crop(img, max(y0-8, 0), min(y1+8, h)))
		if r := e.row("1700000900.000100"); r == nil || r.senderMiss() != core.MemberUnknown || len(r.rv.MissingMembers()) != 1 {
			t.Errorf("%s: the stub's row isn't drawn with its bars", mode)
		}

		const tw, tht = 480, 300
		c := colorsFor(th)
		var tops op.Ops
		paint.FillShape(&tops, c.Bg, clip.Rect{Max: image.Pt(tw, tht)}.Op())
		gtx := layout.Context{Ops: &tops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(tw, tht)), Now: time.Now()}
		txt := reactionTipLine(e.h, &core.Reaction{Name: "tada", Count: 3, Users: []string{"UBOB", ghost, gone}}, tSelf, false)
		reactionTooltip(gtx, e.h, th, c, txt, "", image.Rect(222, 256, 258, 280), image.Rect(0, 0, tw, tht))
		savePNG(t, filepath.Join(dir, "missing-member-tip-"+mode+".png"), pixeltest.Render(t, tw, tht, &tops))
	}
}
