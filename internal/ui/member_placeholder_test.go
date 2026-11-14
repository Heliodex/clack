package ui

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/messages"
)

// Karim (2026-11-14, macOS): on a fresh launch a message's sender drew as
// its raw user id where the name belongs. Core loads messages without
// their people (core/lazy_users.go) and a row looks its sender and the
// members its text mentions up as it draws, so a member the store doesn't
// hold yet is getMemberById's unknown stub until users/info answers: the
// row drew m.User meanwhile, and a mention its parsed "@U…". Slack draws
// BaseMissingMember (MissingText's bar) for both, and the names once the
// fetch upserts the members (core's "names" event here).
//
// This channel ends with messages from an id users/info doesn't know, the
// viewer, a bot, a member of the generated history, and a member nothing
// else names who mentions another; users/info is held while those two are
// asked for. Held: the sender and the mention are bars, the viewer and
// the bot are named. Released: both names, drawn again on "names" with no
// other input. The unknown id ends as the non-existent bar. No frame
// draws an id. Synthetic people.
func TestUnknownMembersDrawPlaceholders(t *testing.T) {
	sz := fakeslack.Small
	sz.Authors = 20 // the generated messages' people: the first 20 users
	fs, err := fakeslack.Start(sz)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	ids := fs.UserIDs()
	sender, mentioned := ids[len(ids)-1], ids[len(ids)-2]
	const ghost = "U0GHOSTUSER1" // no such member: users/info leaves it out
	const botName = "Deploy Bot"
	known := ids[3]
	ch := fs.ChannelIDs()[0]
	tsGhost, _ := fs.PostQuiet(ch, ghost, "from an id nobody knows")
	tsSelf, _ := fs.PostQuiet(ch, fakeslack.SelfID, "the viewer's own")
	tsBot := fs.PostBotQuiet(ch, "B0FAKEBOT0001", botName, "a bot's message")
	tsKnown, _ := fs.PostQuiet(ch, known, "from the generated history's people")
	ts, _ := fs.PostQuiet(ch, sender, "hey <@"+mentioned+"> have a look")

	hold := make(chan struct{})
	var once sync.Once
	release := func() { once.Do(func() { close(hold) }) }
	var mu sync.Mutex
	asked := map[string]int{}
	fs.SetEdgeHook(func(method string, body map[string]json.RawMessage) (any, bool) {
		if method != "edge/users/info" {
			return nil, false
		}
		var up map[string]int
		_ = json.Unmarshal(body["updated_ids"], &up)
		mu.Lock()
		for id := range up {
			asked[id]++
		}
		mu.Unlock()
		_, s := up[sender]
		_, m := up[mentioned]
		if s || m {
			<-hold
		}
		return nil, false
	})
	t.Cleanup(release) // before fs.Close: a held request has to answer
	askedFor := func(id string) bool {
		mu.Lock()
		defer mu.Unlock()
		return asked[id] > 0
	}

	e := startEnv(t, fs, t.TempDir())
	e.waitReady()
	c := e.a.Core()
	e.openChannel(ch)
	e.until("history", 20*time.Second, func() bool { return e.loadedMessages(ch) > 0 })
	e.toLatest() // the channel opens at its first unread; the messages above are the newest
	senders := func() map[string]messages.SenderView {
		out := map[string]messages.SenderView{}
		cv := e.a.convos[ch+"/"]
		if cv == nil {
			return out
		}
		l, ok := cv.list.(channelList)
		if !ok {
			return out
		}
		for _, sv := range l.Senders() {
			out[sv.TS] = sv
		}
		return out
	}
	// no frame draws the people the store can't name by their ids: not
	// as a label (the sender), nor in a body's text (a mention)
	noIDs := func(when string) {
		t.Helper()
		drawn := frameStrings(e)
		for _, id := range []string{sender, mentioned, ghost} {
			if drawnContains(drawn, id) {
				t.Errorf("%s: %s drawn", when, id)
			}
			for _, sv := range senders() {
				if strings.Contains(sv.Name, id) || strings.Contains(sv.Text, id) {
					t.Errorf("%s: message %s draws %s (name %q, text %q)", when, sv.TS, id, sv.Name, sv.Text)
				}
			}
		}
	}
	nameOf := func(id string) string {
		u, ok := c.S.RawUser(id)
		if !ok {
			return ""
		}
		return store.DisplayName(u)
	}

	// the row draws both lookups in one frame: one users/info request has
	// them (held), or the other waits behind it
	e.until("the new sender's row, users/info held", 20*time.Second, func() bool {
		_, ok := senders()[ts]
		return ok && (askedFor(sender) || askedFor(mentioned))
	})
	e.settle(300 * time.Millisecond)
	if _, ok := c.S.RawUser(sender); ok {
		t.Fatal("the sender is in the store before users/info answered")
	}
	row := senders()[ts]
	if row.Missing != core.MemberUnknown || row.Name != "" {
		t.Errorf("held: sender %+v, want BaseMissingMember's loading bar and no name", row)
	}
	if len(row.Mentions) != 1 || row.Mentions[0] != mentioned {
		t.Errorf("held: mention bars %v, want %s's", row.Mentions, mentioned)
	}
	if !strings.Contains(row.Text, "hey") || !strings.Contains(row.Text, "have a look") || strings.Contains(row.Text, "@") {
		t.Errorf("held: body %q, want the text around the mention's bar", row.Text)
	}
	noIDs("held")
	// the viewer and a bot (senderOf's bot path) are named meanwhile
	if sv := senders()[tsSelf]; sv.Name == "" || sv.Name != nameOf(fakeslack.SelfID) || sv.Missing != core.MemberKnown {
		t.Errorf("held: the viewer's row %+v, want %q", sv, nameOf(fakeslack.SelfID))
	}
	if sv := senders()[tsBot]; sv.Name != botName || sv.Missing != core.MemberKnown {
		t.Errorf("held: the bot's row %+v, want %q", sv, botName)
	}

	// users/info answers: "names" draws the sender and the mention again
	release()
	e.until("the sender's and the mention's names", 20*time.Second, func() bool {
		sv := senders()[ts]
		n, m := nameOf(sender), nameOf(mentioned)
		return n != "" && m != "" && sv.Name == n && sv.Missing == core.MemberKnown && len(sv.Mentions) == 0 &&
			strings.Contains(sv.Text, "@"+m)
	})
	noIDs("after users/info")
	// an id users/info doesn't return: the non-existent member's bar
	e.until("the unknown id's non-existent bar", 20*time.Second, func() bool {
		sv := senders()[tsGhost]
		return sv.Missing == core.MemberNonExistent && sv.Name == "" && c.S.Nonexistent(ghost)
	})
	e.until("the generated member's name", 20*time.Second, func() bool {
		sv := senders()[tsKnown]
		return sv.Name != "" && sv.Name == nameOf(known)
	})
	noIDs("settled")
	if sv := senders()[tsSelf]; sv.Name != nameOf(fakeslack.SelfID) {
		t.Errorf("settled: the viewer's row %+v", sv)
	}
	if sv := senders()[tsBot]; sv.Name != botName {
		t.Errorf("settled: the bot's row %+v", sv)
	}
}
