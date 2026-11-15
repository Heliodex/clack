package ui

// A canvas tab whose title mentions a channel (Karim, α.85: #ship's tab
// read "<#C0M8PUPU6> Guidelines"). Slack labels an unlabeled canvas tab
// with the file's title_blocks (useTabLabel → Name → QuipTitleParsedId),
// so the channel element reads #name in the tab's own colour. Synthetic
// fakeslack workspace; nothing talks to Slack.

import (
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/richtext"
)

func TestTabTitleSpans(t *testing.T) {
	in := []richtext.Span{
		{Kind: richtext.Channel, ID: "C2", Text: "#ship-guidelines"},
		{Kind: richtext.Text, Text: " Guidelines "},
		{Kind: richtext.User, ID: "U1", Text: "@alice"},
		{Kind: richtext.Emoji, ID: "cat", Text: ":cat:", URL: "https://emoji.example/cat.png"},
		{Kind: richtext.Link, Text: "notes", URL: "https://x.example"},
		{Kind: richtext.Break},
		{Kind: richtext.Text, Text: "second line"},
	}
	got := tabTitleSpans(in)
	if len(got) != 3 {
		t.Fatalf("spans: %+v", got)
	}
	if got[0].Kind != richtext.Text || got[0].Text != "#ship-guidelines Guidelines @alice" || got[0].Style&richtext.Bold == 0 {
		t.Errorf("mentions as bold plain text: %+v", got[0])
	}
	if got[1].Kind != richtext.Emoji || got[1].URL == "" {
		t.Errorf("emoji kept: %+v", got[1])
	}
	if got[2].Kind != richtext.Text || got[2].Text != "notes" || got[2].URL != "" {
		t.Errorf("link as plain text (noInlineLinks): %+v", got[2])
	}
	// a tab without title_blocks keeps its label's emoji path
	if s := tabSpans(core.Tab{Title: "<#C2> plain"}, "<#C2> plain", nil); s != nil {
		t.Errorf("plain title: %+v", s)
	}
	if s := tabSpans(core.Tab{TitleSpans: in[:2]}, "#ship-guidelines Guidelines ", nil); richtext.PlainSpans(s) != "#ship-guidelines Guidelines " {
		t.Errorf("title spans: %+v", s)
	}
}

func TestTabTitleChannelRef(t *testing.T) {
	fs, err := fakeslack.Start(fakeslack.Small)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	const ch, ref, unknown = "C0FAKE00001", "C0FAKE00000", "C0TABREF404"
	const known, missing = "F0TABREF001", "F0TABREF002"
	e := startEnv(t, fs, t.TempDir())
	e.waitReady()
	cv, ok := e.a.Core().Conv(ref)
	if !ok || cv.Name == "" {
		t.Fatalf("no %s", ref)
	}
	section := func(els ...any) []any {
		return []any{map[string]any{"type": "rich_text", "block_id": "t", "elements": []any{
			map[string]any{"type": "rich_text_section", "elements": els}}}}
	}
	text := func(s string) any { return map[string]any{"type": "text", "text": s} }
	channel := func(id string) any { return map[string]any{"type": "channel", "channel_id": id} }
	fs.AddCanvas(fakeslack.FakeCanvas{ID: known, Channel: ch, Title: "<#" + ref + "> Guidelines",
		TitleBlocks: section(channel(ref), text(" Guidelines"))})
	fs.AddCanvas(fakeslack.FakeCanvas{ID: missing, Channel: ch, Title: "<#" + unknown + "> notes",
		TitleBlocks: section(channel(unknown), text(" notes"))})
	e.openChannel(ch)
	labels := map[string]string{}
	e.until("the canvas tabs", 15*time.Second, func() bool {
		if !e.a.data.tabsDone[ch] {
			return false
		}
		for _, tb := range e.a.channelTabs(ch) {
			if tb.fileID == known || tb.fileID == missing {
				labels[tb.fileID] = richtext.PlainSpans(tb.spans)
				if tb.spans == nil {
					labels[tb.fileID] = "(no spans) " + tb.label
				}
			}
		}
		return len(labels) == 2
	})
	if want := "#" + cv.Name + " Guidelines"; labels[known] != want {
		t.Errorf("known channel: %q, want %q", labels[known], want)
	}
	// channel_not_found: BaseMissingChannel's Nonexistent label
	if want := richtext.PrivateChannelLabel + " notes"; labels[missing] != want {
		t.Errorf("unknown channel: %q, want %q", labels[missing], want)
	}
	for _, tb := range e.a.channelTabs(ch) {
		if tb.fileID == known && tb.label != "#"+cv.Name+" Guidelines" {
			t.Errorf("label (menus, More): %q", tb.label)
		}
	}
	e.frame() // the tab bar lays the spans out
}
