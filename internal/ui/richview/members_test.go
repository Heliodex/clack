package richview

import (
	"strings"
	"testing"

	"gioui.org/op"

	rt "github.com/some-du6e/clack/internal/richtext"
)

// A user mention names the member as plugins present it (Nicknames'
// patched members slice, through the ui root's MemberLabel): the label
// replaces the parsed "@name", the doc is laid out again when the label
// changes, and the parsed name comes back when the plugin stops changing
// the member. Synthetic names.
func TestMentionLabelFromPlugins(t *testing.T) {
	labels := map[string]string{}
	calls := 0
	SetMemberLabel(func(id string) (string, bool) {
		calls++
		n, ok := labels[id]
		return n, ok
	})
	t.Cleanup(func() { SetMemberLabel(nil) })
	st := NewStyle(themeShaper(), noImages{}, LightPalette())
	doc := rt.Doc{Blocks: []rt.Block{{Kind: rt.Paragraph, Spans: []rt.Span{
		{Kind: rt.Text, Text: "ping "},
		{Kind: rt.User, Text: "@alice", ID: "UALICE0001"},
		{Kind: rt.Text, Text: " about the "},
		{Kind: rt.Channel, Text: "#general", ID: "C0GENERAL1"},
	}}}}
	const w = 480
	v := layoutAt(st, doc, Options{}, w)
	text := func() string {
		var ops op.Ops
		v.Layout(testGtx(&ops, w), st, &recHandler{})
		return v.TextRange(0, v.PlainLen())
	}
	if got := text(); !strings.Contains(got, "@alice") {
		t.Fatalf("unpatched: %q, want the parsed @alice", got)
	}
	labels["UALICE0001"] = "Ally the Nicknamed"
	if got := text(); !strings.Contains(got, "@Ally the Nicknamed") || strings.Contains(got, "@alice") {
		t.Errorf("with a nickname: %q", got)
	}
	if got := text(); !strings.Contains(got, "#general") {
		t.Errorf("the channel mention changed: %q", got)
	}
	delete(labels, "UALICE0001")
	if got := text(); !strings.Contains(got, "@alice") || strings.Contains(got, "Ally") {
		t.Errorf("the nickname removed: %q", got)
	}
	// no hook (no plugin runtime): nothing is asked or recorded
	SetMemberLabel(nil)
	calls = 0
	v2 := layoutAt(st, doc, Options{}, w)
	if len(v2.d.names) != 0 || calls != 0 {
		t.Errorf("without a hook the layout recorded %d mentions and asked %d times", len(v2.d.names), calls)
	}
}

// A mention whose member the store didn't hold when its message loaded
// (core builds message docs without asking: core/lazy_users.go) reads the
// member as it draws (MemberRead, Slack's getMemberById in MemberSlug):
// each layout reads it, and until the member is known the mention is
// BaseMissingMember's bar (MissingText: 120px by 8, its bottom on the
// baseline; outlined once users/info says there's no such member), never
// the parsed id, in the drawing or the plain text. Then the doc is laid
// out again with the name; a plugin's label still wins. Synthetic names.
func TestMentionReadsMemberWhenDrawn(t *testing.T) {
	known := map[string]string{}
	gone := map[string]bool{}
	reads := 0
	SetMemberRead(func(id string) (string, MemberMiss) {
		reads++
		if n, ok := known[id]; ok {
			return n, MemberLoaded
		}
		if gone[id] {
			return "", MemberGone
		}
		return "", MemberLoading
	})
	t.Cleanup(func() { SetMemberRead(nil) })
	st := NewStyle(themeShaper(), noImages{}, LightPalette())
	doc := rt.Doc{Blocks: []rt.Block{{Kind: rt.Paragraph, Spans: []rt.Span{
		{Kind: rt.Text, Text: "ping "},
		{Kind: rt.User, Text: "@UBOB000001", ID: "UBOB000001"},
		{Kind: rt.Text, Text: " please"},
	}}}}
	const w = 480
	v := layoutAt(st, doc, Options{}, w)
	text := func() string {
		var ops op.Ops
		v.Layout(testGtx(&ops, w), st, &recHandler{})
		return v.TextRange(0, v.PlainLen())
	}
	// the bar: a 120x8 box whose bottom is the text's baseline
	bar := func() (b box, ok bool) {
		for _, bx := range v.d.boxes {
			if bx.r.Dx() == 120 && bx.r.Dy() == 8 {
				return bx, true
			}
		}
		return box{}, false
	}
	baseline := func() int {
		for _, f := range textFrags(&v.d) {
			return int(f.y)
		}
		return -1
	}
	got := text()
	if strings.Contains(got, "UBOB000001") || !strings.Contains(got, "ping") || !strings.Contains(got, "please") {
		t.Fatalf("unknown member: %q, want the text around the mention and no id", got)
	}
	if m := v.MissingMembers(); len(m) != 1 || m[0] != "UBOB000001" {
		t.Fatalf("unknown member: bars for %v, want UBOB000001's", m)
	}
	b, ok := bar()
	if !ok {
		t.Fatalf("unknown member: no 120x8 MissingText bar among %d boxes", len(v.d.boxes))
	}
	if b.kind != bFill || b.fill != st.Palette.Missing || b.r.Max.Y != baseline() {
		t.Errorf("unknown member's bar: kind %v fill %v bottom %d (baseline %d), want base-ter on the baseline", b.kind, b.fill, b.r.Max.Y, baseline())
	}
	n := reads
	text()
	if reads <= n {
		t.Errorf("a drawn mention didn't read its member again (%d reads, then %d)", n, reads)
	}
	// users/info didn't return it: the outlined bar
	gone["UBOB000001"] = true
	if got := text(); strings.Contains(got, "UBOB000001") {
		t.Errorf("non-existent member: %q shows the id", got)
	}
	if b, ok := bar(); !ok || b.kind != bBorder || b.stroke != st.Palette.MissingLine || b.fill.A != 0 {
		t.Errorf("non-existent member: bar %+v (found %v), want the 1px outline", b, ok)
	}
	known["UBOB000001"] = "bob"
	if got := text(); !strings.Contains(got, "@bob") || strings.Contains(got, "UBOB000001") {
		t.Errorf("the member arrived: %q, want @bob", got)
	}
	if m := v.MissingMembers(); len(m) != 0 {
		t.Errorf("the member arrived: still bars for %v", m)
	}
	if _, ok := bar(); ok {
		t.Error("the member arrived: the bar is still drawn")
	}
	SetMemberLabel(func(id string) (string, bool) { return "Bobby", id == "UBOB000001" })
	t.Cleanup(func() { SetMemberLabel(nil) })
	if got := text(); !strings.Contains(got, "@Bobby") {
		t.Errorf("a plugin's label: %q, want @Bobby", got)
	}
}

// A system message (Muted) names its people with RollupName, whose name
// for a member the store can't name is "": no bar and no id, then the
// name once it loads. Synthetic names.
func TestSystemMentionUnknownIsEmpty(t *testing.T) {
	known := map[string]string{}
	SetMemberRead(func(id string) (string, MemberMiss) {
		if n, ok := known[id]; ok {
			return n, MemberLoaded
		}
		return "", MemberLoading
	})
	t.Cleanup(func() { SetMemberRead(nil) })
	st := NewStyle(themeShaper(), noImages{}, LightPalette())
	doc := rt.Doc{Blocks: []rt.Block{{Kind: rt.Paragraph, Spans: []rt.Span{
		{Kind: rt.User, Text: "", ID: "UCAROL0001"},
		{Kind: rt.Text, Text: " joined the channel"},
	}}}}
	const w = 480
	v := layoutAt(st, doc, Options{Muted: true}, w)
	text := func() string {
		var ops op.Ops
		v.Layout(testGtx(&ops, w), st, &recHandler{})
		return v.TextRange(0, v.PlainLen())
	}
	if got := text(); strings.Contains(got, "UCAROL0001") || !strings.Contains(got, "joined the channel") {
		t.Errorf("unknown member: %q", got)
	}
	if m := v.MissingMembers(); len(m) != 0 {
		t.Errorf("a system message drew bars for %v", m)
	}
	known["UCAROL0001"] = "carol"
	if got := text(); !strings.Contains(got, "carol joined the channel") {
		t.Errorf("the member arrived: %q, want carol joined the channel", got)
	}
}
