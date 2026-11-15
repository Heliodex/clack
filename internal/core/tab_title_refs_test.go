package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/some-du6e/clack/internal/richtext"
	"github.com/some-du6e/clack/internal/slack"
)

// A canvas tab's title mentions a channel: Slack labels an unlabeled
// canvas tab with the file's title_blocks (useTabLabel → Name →
// QuipTitleParsedId), never by parsing the title string's <#C…> token.
// Synthetic fixtures.
func TestTabsCanvasTitleRefs(t *testing.T) {
	a, _ := testApp(t)
	a.S.PutConv(slack.Conversation{ID: "C2", Name: "ship-guidelines", IsChannel: true})
	blocks := func(els string) string {
		return `[{"type":"rich_text","block_id":"t","elements":[{"type":"rich_text_section","elements":[` + els + `]}]}]`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		method := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		body := `{"ok":false,"error":"not_in_test"}`
		switch method {
		case "conversations.info":
			switch r.PostForm.Get("channel") {
			case "C1":
				body = `{"ok":true,"channel":{"id":"C1","properties":{"tabs":[
					{"id":"Ct1","type":"canvas","label":"","data":{"file_id":"FKNOWN"}},
					{"id":"Ct2","type":"canvas","label":"","data":{"file_id":"FGONE"}},
					{"id":"Ct3","type":"canvas","label":"","data":{"file_id":"FFETCH"}},
					{"id":"Ct4","type":"canvas","label":"","data":{"file_id":"FPLAIN"}},
					{"id":"Ct5","type":"canvas","label":"Own <#C2> label","data":{"file_id":"FKNOWN"}},
					{"id":"Ct6","type":"canvas","label":"","data":{"file_id":"FEMOJI"}}]}}}`
			case "C7":
				body = `{"ok":true,"channel":{"id":"C7","name":"design","is_channel":true}}`
			case "C404":
				body = `{"ok":false,"error":"channel_not_found"}`
			}
		case "files.info":
			switch r.PostForm.Get("file") {
			case "FKNOWN":
				body = `{"ok":true,"file":{"id":"FKNOWN","title":"<#C2> Guidelines","filetype":"quip","title_blocks":` +
					blocks(`{"type":"channel","channel_id":"C2"},{"type":"text","text":" Guidelines"}`) + `}}`
			case "FGONE":
				body = `{"ok":true,"file":{"id":"FGONE","title":"<#C404> notes","filetype":"quip","title_blocks":` +
					blocks(`{"type":"channel","channel_id":"C404"},{"type":"text","text":" notes"}`) + `}}`
			case "FFETCH":
				body = `{"ok":true,"file":{"id":"FFETCH","title":"<#C7|design> sync","filetype":"quip","title_blocks":` +
					blocks(`{"type":"channel","channel_id":"C7"},{"type":"text","text":" sync"}`) + `}}`
			case "FPLAIN":
				body = `{"ok":true,"file":{"id":"FPLAIN","title":"<#C2> no blocks","filetype":"quip"}}`
			case "FEMOJI":
				body = `{"ok":true,"file":{"id":"FEMOJI","title":"Plan","filetype":"quip","title_blocks":` +
					blocks(`{"type":"text","text":"Plan "},{"type":"user","user_id":"U1"}`) + `}}`
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	a.C = slack.New(slack.Creds{Token: "xoxc-test", UserID: "USELF", Team: "T1", TeamURL: srv.URL + "/"})
	a.C.EdgeURL = srv.URL + "/edge/"

	tabs, err := a.Tabs(context.Background(), "C1")
	if err != nil {
		t.Fatal(err)
	}
	if len(tabs) != 6 {
		t.Fatalf("tabs: %+v", tabs)
	}
	want := []struct{ title, spans string }{
		{"#ship-guidelines Guidelines", "#ship-guidelines Guidelines"},                     // a known channel
		{richtext.PrivateChannelLabel + " notes", richtext.PrivateChannelLabel + " notes"}, // channel_not_found: BaseMissingChannel
		{"#design sync", "#design sync"},                                                   // looked up (conversations.info) before the label is built
		{"<#C2> no blocks", ""},                                                            // no title_blocks: the title string, as Slack draws it
		{"Own <#C2> label", ""},                                                            // the tab's own label wins, unparsed (FormatTextWithEmoji)
		{"Plan @Alice", "Plan @Alice"},                                                     // a user element
	}
	for i, w := range want {
		tb := tabs[i]
		if tb.Title != w.title || richtext.PlainSpans(tb.TitleSpans) != w.spans {
			t.Errorf("%s: title %q spans %q, want %q / %q", tb.ID, tb.Title, richtext.PlainSpans(tb.TitleSpans), w.title, w.spans)
		}
	}
	if sp := tabs[0].TitleSpans; len(sp) == 0 || sp[0].Kind != richtext.Channel || sp[0].ID != "C2" {
		t.Errorf("channel span: %+v", sp)
	}
	if sp := tabs[1].TitleSpans; len(sp) == 0 || !sp[0].Lock {
		t.Errorf("missing channel span: %+v", sp)
	}
}
