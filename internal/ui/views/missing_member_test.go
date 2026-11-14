package views

// The views drew a member the store couldn't name yet by their raw U… id
// (views.userName's fallback): Activity's senders, the Threads view's
// people and DM titles; and kept it, since Activity described its rows
// again only on channel lookups and the Threads view never. Slack draws
// BaseMissingMember's bar for an Activity sender and a message's, leaves
// an unnamed participant out of a thread's people and a group DM's
// title, names an IM "" until its member loads, and draws them all again
// once the fetch upserts the member (missing_member.go). Synthetic people
// and ids over fakeslack; users/info is held while the rows ask.

import (
	"context"
	"encoding/json"
	"image"
	"net/url"
	"strings"
	"sync"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/richtext"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/theme"
)

const (
	mmKnown = "U0KNOWN0001" // in the store: Ada Example
	mmLate1 = "U0LATE00001" // users/info has them only after release
	mmLate2 = "U0LATE00002"
	mmLate3 = "U0LATE00003"
	mmLate4 = "U0LATE00004"
	mmGhost = "U0GHOST0001" // users/info never returns it: non-existent
	mmMPDM  = "G0LATEMPDM1" // the viewer, Ada and mmLate4
	mmIM    = "D0LATEIM001" // the viewer and mmLate3
)

var mmIDs = []string{mmLate1, mmLate2, mmLate3, mmLate4, mmGhost}

// mmHold is users/info for the synthetic members: a request with one of
// them waits until release and then has the late ones (never mmGhost),
// except a first ask for an id in first, which answers at once without
// it (the Activity load's EnsureUsers, as if users/info had nothing yet,
// so the rows draw it unknown and ask again). Other requests are
// fakeslack's.
type mmHold struct {
	mu    sync.Mutex
	asked map[string]int
	first map[string]bool
	users map[string]slack.User
	hold  chan struct{}
	once  sync.Once
}

func newMMHold(first ...string) *mmHold {
	h := &mmHold{asked: map[string]int{}, first: map[string]bool{}, hold: make(chan struct{}), users: map[string]slack.User{
		mmLate1: {ID: mmLate1, Name: "lee", RealName: "Lee Late"},
		mmLate2: {ID: mmLate2, Name: "rae", RealName: "Rae React"},
		mmLate3: {ID: mmLate3, Name: "tom", RealName: "Tom Thread"},
		mmLate4: {ID: mmLate4, Name: "gia", RealName: "Gia Group"},
	}}
	for _, id := range first {
		h.first[id] = true
	}
	return h
}

func (h *mmHold) release() { h.once.Do(func() { close(h.hold) }) }

// askedN is how many users/info requests had id.
func (h *mmHold) askedN(id string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.asked[id]
}

func (h *mmHold) hook(method string, body map[string]json.RawMessage) (any, bool) {
	if method != "edge/users/info" {
		return nil, false
	}
	var up map[string]int
	_ = json.Unmarshal(body["updated_ids"], &up)
	ours, firstAsk := false, false
	h.mu.Lock()
	for id := range up {
		h.asked[id]++
		if _, late := h.users[id]; late || id == mmGhost {
			ours = true
		}
		if h.first[id] && h.asked[id] == 1 {
			firstAsk = true
		}
	}
	h.mu.Unlock()
	if !ours {
		return nil, false
	}
	res := []slack.User{}
	if !firstAsk {
		<-h.hold
		for id := range up {
			if u, ok := h.users[id]; ok {
				res = append(res, u)
			}
		}
	}
	return map[string]any{"ok": true, "results": res, "failed_ids": []string{}}, true
}

// mmFake boots a core on fakeslack's views fixtures (the member fetcher
// runs, as after any boot) with hook answering Web API methods first and
// h answering users/info; Ada is in the store, the rest aren't.
func mmFake(t *testing.T, h *mmHold, hook func(method string, f url.Values) (any, bool)) *core.App {
	t.Helper()
	sz := fakeslack.Small
	sz.Views = true
	fs, err := fakeslack.Start(sz)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fs.Close() })
	fs.SetHook(hook)
	fs.SetEdgeHook(h.hook)
	app := core.New(slack.New(fs.Creds()), store.New(t.TempDir()))
	t.Cleanup(app.Shutdown)
	t.Cleanup(h.release) // first: a held request has to answer
	if err := app.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	app.S.PutUsers([]slack.User{{ID: mmKnown, Name: "ada", RealName: "Ada Example"}})
	app.S.PutConv(slack.Conversation{ID: fakeslack.ViewsChannel, Name: "views-fixtures", IsChannel: true, IsMember: true, NumMembers: 3})
	app.S.PutConv(slack.Conversation{ID: mmMPDM, IsMPIM: true, Name: "mpdm-self--ada--gia-1", IsMember: true,
		Members: []string{fakeslack.SelfID, mmKnown, mmLate4}})
	app.S.PutConv(slack.Conversation{ID: mmIM, IsIM: true, User: mmLate3, IsMember: true})
	return app
}

func mmMsg(ch, ts, user, text string) slack.Message {
	return slack.Message{Type: "message", Channel: ch, TS: ts, User: user, Text: text}
}

// drawnStrings runs a frame and returns every string it drew.
func drawnStrings(e *vaEnv) []string {
	var mu sync.Mutex
	var out []string
	prev := theme.DrawHook
	theme.DrawHook = func(s string) {
		mu.Lock()
		out = append(out, s)
		mu.Unlock()
	}
	e.frame()
	theme.DrawHook = prev
	return out
}

// mmNoIDs fails for any of the synthetic members' ids in strs.
func mmNoIDs(t *testing.T, when string, strs ...string) {
	t.Helper()
	for _, s := range strs {
		for _, id := range mmIDs {
			if strings.Contains(s, id) {
				t.Errorf("%s: %q shows %s", when, s, id)
			}
		}
	}
}

// mmActivityHook is the feed: mentions from a late member, the ghost and
// Ada; a reaction from a late member; a thread whose other participant
// is late; a group DM with a late member.
func mmActivityHook(method string, _ url.Values) (any, bool) {
	if method != "activity.feed" {
		return nil, false
	}
	ch := fakeslack.ViewsChannel
	msg := func(ts, user string) map[string]any {
		return map[string]any{"ts": ts, "channel": ch, "author_user_id": user}
	}
	e := func(key, ts string, item map[string]any) map[string]any {
		return map[string]any{"key": key, "feed_ts": ts, "is_unread": true, "is_bot": false, "item": item}
	}
	return map[string]any{"ok": true, "response_metadata": map[string]any{"next_cursor": ""}, "items": []any{
		e("mm-late", "1762800600.000100", map[string]any{"type": "at_user", "message": msg("1762800600.000100", mmLate1)}),
		e("mm-ghost", "1762800500.000100", map[string]any{"type": "at_user", "message": msg("1762800500.000100", mmGhost)}),
		e("mm-known", "1762800400.000100", map[string]any{"type": "at_user", "message": msg("1762800400.000100", mmKnown)}),
		e("mm-react", "1762800350.000100", map[string]any{"type": "message_reaction", "message": msg("1762800300.000100", fakeslack.SelfID),
			"reaction": map[string]any{"user": mmLate2, "name": "eyes"}}),
		e("mm-thread", "1762800250.000100", map[string]any{"type": "thread_v2", "bundle_info": map[string]any{"payload": map[string]any{
			"thread_entry": map[string]any{"channel_id": ch, "latest_ts": "1762800250.000100", "thread_ts": "1762800200.000100", "unread_msg_count": 1}}}}),
		e("mm-mpdm", "1762800150.000100", map[string]any{"type": "mpdm", "bundle_info": map[string]any{"unread_count": 1, "payload": map[string]any{
			"dm_entry": map[string]any{"latest_message": map[string]any{"ts": "1762800150.000100", "channel": mmMPDM}}}}}),
	}}, true
}

// mmActivityStore caches the feed's messages (the load hydrates from the
// store first).
func mmActivityStore(s *store.Store) {
	ch := fakeslack.ViewsChannel
	root := mmMsg(ch, "1762800200.000100", fakeslack.SelfID, "a synthetic plan")
	root.ThreadTS, root.ReplyCount, root.ReplyUsers = root.TS, 2, []string{mmKnown, mmLate3}
	reply := mmMsg(ch, "1762800250.000100", mmKnown, "a synthetic reply")
	reply.ThreadTS = root.TS
	reacted := mmMsg(ch, "1762800300.000100", fakeslack.SelfID, "a synthetic note")
	reacted.Reactions = []slack.Reaction{{Name: "eyes", Count: 1, Users: []string{mmLate2}}}
	s.SetLatest(ch, []slack.Message{root, reply, reacted,
		mmMsg(ch, "1762800400.000100", mmKnown, "a synthetic question"),
		mmMsg(ch, "1762800500.000100", mmGhost, "a synthetic note from nobody"),
		mmMsg(ch, "1762800600.000100", mmLate1, "a synthetic note from later"),
	}, false)
	s.SetThread(ch, root.TS, []slack.Message{root, reply})
	s.SetLatest(mmMPDM, []slack.Message{mmMsg(mmMPDM, "1762800150.000100", mmKnown, "a synthetic group hello")}, false)
}

// mmRowH draws the row key as the list does (Detailed or Dense) and
// returns its height.
func mmRowH(t *testing.T, v *Activity, key string) int {
	t.Helper()
	page := actPage(v)
	for i := range page.Items {
		if actKey(&page.Items[i]) != key {
			continue
		}
		var ops op.Ops
		gtx := actFrame(&ops, image.Pt(1000, 400))
		gtx.Constraints = layout.Constraints{Min: image.Pt(1000, 0), Max: image.Pt(1000, 400)}
		return v.row(gtx, &page.Items[i], i).Size.Y
	}
	t.Fatalf("%s isn't in the feed", key)
	return 0
}

// Activity: while users/info is held, the late author's, the reactor's
// and the thread's rows are BaseMissingMember's bar (no name, no "+N"),
// as is the ghost's; the group DM's footer names Ada alone. The rows keep
// the height a name gives them. The "names" event that follows the
// answer describes them again; the ghost ends non-existent, still the
// bar (Activity draws BaseMissingMember's default type). No row, and no
// string a frame draws, shows an id.
func TestActivityUnknownMembersNeverIDs(t *testing.T) {
	for _, dark := range []bool{false, true} {
		h := newMMHold(mmLate1, mmLate2, mmGhost)
		app := mmFake(t, h, mmActivityHook)
		mmActivityStore(app.S)
		v := NewActivity(&slickHost{app: app, th: theme.New(dark)})
		e := &vaEnv{size: image.Pt(1200, 1100)}
		e.draw = func(gtx layout.Context) { v.Layout(gtx) }
		e.until(t, "the feed", func() bool { return len(actPage(v).Items) == 6 })
		row := func(key string) actDesc { return v.rowFor(key).d }
		noIDs := func(when string) {
			t.Helper()
			mmNoIDs(t, when+": drawn", drawnStrings(e)...)
			page := actPage(v)
			for i := range page.Items {
				d := row(actKey(&page.Items[i]))
				mmNoIDs(t, when+": "+page.Items[i].Key, d.name, d.nameDense, d.title, d.foot, d.loc, d.body, d.prev, d.subtext, d.hay)
			}
		}

		// the rows ask for the members they draw: the feed load's
		// EnsureUsers had none of them, the fetcher's request is held
		// (one at a time: what the rows asked later waits behind it)
		e.until(t, "the rows' lookups, users/info held", func() bool {
			return h.askedN(mmLate1) > 1 || h.askedN(mmLate2) > 1 || h.askedN(mmGhost) > 1 || h.askedN(mmLate3) > 0 || h.askedN(mmLate4) > 0
		})
		e.frame()
		for _, key := range []string{"mm-late", "mm-ghost", "mm-react", "mm-thread"} {
			if d := row(key); d.name != "" || d.nameDense != "" || d.nameBlank {
				t.Errorf("held: %s's sender %q (%q), want BaseMissingMember's bar", key, d.name, d.nameDense)
			}
		}
		for key, id := range map[string]string{"mm-late": mmLate1, "mm-react": mmLate2, "mm-thread": mmLate3, "mm-mpdm": mmLate4} {
			if d := row(key); !namesAny(d.waits, []string{id}) {
				t.Errorf("held: %s waits for %v, want %s", key, d.waits, id)
			}
		}
		if d := row("mm-known"); d.name != "Ada Example" || len(d.waits) != 0 {
			t.Errorf("held: the known sender %q (waits %v)", d.name, d.waits)
		}
		if d := row("mm-mpdm"); d.name != "Ada Example" || d.foot != "Ada Example" {
			t.Errorf("held: the group DM's sender %q, footer %q, want Ada and getMpimDisplayNameAlphabetical without the late member", d.name, d.foot)
		}
		noIDs("held")
		// the bar is the name's line box: the row is as tall as one with a name
		detailed := mmRowH(t, v, "mm-known")
		if got := mmRowH(t, v, "mm-late"); got != detailed {
			t.Errorf("held: the bar's row is %dpx, a named one %dpx", got, detailed)
		}
		vaShot(t, e, "activity-members-held", dark)
		v.dense, v.dirty = true, true
		e.frame()
		dense := mmRowH(t, v, "mm-known")
		if got := mmRowH(t, v, "mm-late"); got != dense {
			t.Errorf("held, Dense: the bar's row is %dpx, a named one %dpx", got, dense)
		}
		noIDs("held, Dense")
		vaShot(t, e, "activity-members-held-dense", dark)
		v.dense, v.dirty = false, true

		// users/info answers: "names" brings every name in
		h.release()
		e.until(t, "the late members' names", func() bool {
			return row("mm-late").name == "Lee Late" && row("mm-react").name == "Rae React" &&
				row("mm-thread").name == "Ada Example +1" && row("mm-mpdm").foot == "Ada Example, Gia Group"
		})
		e.until(t, "the ghost non-existent", func() bool { return app.S.Nonexistent(mmGhost) && len(row("mm-ghost").waits) == 0 })
		if d := row("mm-ghost"); d.name != "" {
			t.Errorf("settled: the ghost's sender %q, want the bar", d.name)
		}
		for _, key := range []string{"mm-late", "mm-react", "mm-thread", "mm-mpdm", "mm-known"} {
			if d := row(key); len(d.waits) != 0 {
				t.Errorf("settled: %s still waits for %v", key, d.waits)
			}
		}
		if got := mmRowH(t, v, "mm-late"); got != detailed {
			t.Errorf("named: the row is %dpx, it was %dpx with the bar", got, detailed)
		}
		noIDs("settled")
		vaShot(t, e, "activity-members-named", dark)
	}
}

// mmThreadsHook is the Threads view: a channel thread with a late
// replier; a DM with a late member; a group DM thread with a late
// replier; a thread with the ghost.
func mmThreadsHook(method string, _ url.Values) (any, bool) {
	if method != "subscriptions.thread.getView" {
		return nil, false
	}
	ch, self := fakeslack.ViewsChannel, fakeslack.SelfID
	thread := func(root slack.Message, users []string, replies ...slack.Message) map[string]any {
		root.ThreadTS, root.ReplyCount, root.ReplyUsers = root.TS, len(replies), users
		root.Subscribed = true
		root.LatestReply = replies[len(replies)-1].TS
		for i := range replies {
			replies[i].ThreadTS = root.TS
		}
		return map[string]any{"root_msg": root, "latest_replies": replies, "unread_replies": []slack.Message{}}
	}
	return map[string]any{"ok": true, "has_more": false, "max_ts": "", "total_unread_replies": 0,
		"threads_state": map[string]any{"has_unreads": false, "mention_count": 0},
		"threads": []any{
			thread(mmMsg(ch, "1762900000.000100", mmKnown, "a synthetic question"), []string{self, mmLate1},
				mmMsg(ch, "1762900100.000100", self, "a synthetic answer"), mmMsg(ch, "1762900200.000100", mmLate1, "a synthetic follow-up")),
			thread(mmMsg(mmIM, "1762890000.000100", mmLate3, "a synthetic direct question"), []string{self},
				mmMsg(mmIM, "1762890100.000100", self, "a synthetic direct answer")),
			thread(mmMsg(mmMPDM, "1762880000.000100", mmKnown, "a synthetic group plan"), []string{mmLate4},
				mmMsg(mmMPDM, "1762880100.000100", mmLate4, "a synthetic group reply")),
			thread(mmMsg(ch, "1762870000.000100", self, "a synthetic note"), []string{mmGhost},
				mmMsg(ch, "1762870100.000100", mmGhost, "a synthetic reply from nobody")),
		}}, true
}

// The Threads view, users/info held: the channel thread's people leave
// the late replier out ("Ada Example and you": Slack counts the named),
// the DM's title is "" (getDisplayNameForChannel's IM name) and its
// people "Just you", the group DM's title and people name Ada alone, and
// every message from a member the store can't name has BaseMessageSender's
// bar. On "names" the cards make their headers again and the senders
// read their names; the ghost's reply ends with the non-existent
// (outlined) bar. No frame draws an id.
func TestThreadsUnknownMembersNeverIDs(t *testing.T) {
	for _, dark := range []bool{false, true} {
		h := newMMHold()
		app := mmFake(t, h, mmThreadsHook)
		v := NewThreads(&slickHost{app: app, th: theme.New(dark)})
		e := &vaEnv{size: image.Pt(1000, 2600)}
		e.draw = func(gtx layout.Context) { v.Layout(gtx) }
		e.until(t, "the threads", func() bool { p, ok, _ := v.data.get(); return ok && len(p.items) == 4 })
		card := func(ch, ts string) *thrCard {
			if c := v.cards[ch+"/"+ts]; c != nil {
				return c
			}
			return &thrCard{msgs: map[string]*thrMsg{}}
		}
		sender := func(ch, root, ts string) (string, core.MemberState) {
			s := card(ch, root).msgs[ts]
			if s == nil || s.m == nil {
				return "?", core.MemberKnown
			}
			return s.name, v.memberState(s.m.User)
		}
		e.until(t, "the cards' lookups, users/info held", func() bool {
			return h.askedN(mmLate1) > 0 || h.askedN(mmLate3) > 0 || h.askedN(mmLate4) > 0 || h.askedN(mmGhost) > 0
		})
		e.frame()
		chT, imT := card(fakeslack.ViewsChannel, "1762900000.000100"), card(mmIM, "1762890000.000100")
		mpT, ghT := card(mmMPDM, "1762880000.000100"), card(fakeslack.ViewsChannel, "1762870000.000100")
		if chT.name != "#views-fixtures" || chT.people != "Ada Example and you" {
			t.Errorf("held: the channel thread %q / %q, want #views-fixtures / Ada Example and you", chT.name, chT.people)
		}
		if imT.name != "" || imT.people != "Just you" {
			t.Errorf("held: the DM %q / %q, want an empty title (getDisplayNameForChannel) and Just you", imT.name, imT.people)
		}
		if mpT.name != "Ada Example" || mpT.people != "Ada Example" {
			t.Errorf("held: the group DM %q / %q, want Ada alone", mpT.name, mpT.people)
		}
		if ghT.people != "Just you" {
			t.Errorf("held: the ghost's thread's people %q, want Just you", ghT.people)
		}
		for _, w := range []struct{ ch, root, ts string }{
			{fakeslack.ViewsChannel, "1762900000.000100", "1762900200.000100"},
			{mmIM, "1762890000.000100", "1762890000.000100"},
			{mmMPDM, "1762880000.000100", "1762880100.000100"},
			{fakeslack.ViewsChannel, "1762870000.000100", "1762870100.000100"},
		} {
			if name, miss := sender(w.ch, w.root, w.ts); name != "" || miss != core.MemberUnknown {
				t.Errorf("held: %s's sender %q (%v), want the unknown bar", w.ts, name, miss)
			}
		}
		if name, _ := sender(fakeslack.ViewsChannel, "1762900000.000100", "1762900000.000100"); name != "Ada Example" {
			t.Errorf("held: Ada's root sender %q", name)
		}
		mmNoIDs(t, "held: drawn", drawnStrings(e)...)
		for _, c := range []*thrCard{chT, imT, mpT, ghT} {
			mmNoIDs(t, "held: a card", c.name, c.people, c.also)
		}
		vaShot(t, e, "threads-members-held", dark)

		h.release()
		e.until(t, "the late members' names", func() bool {
			chT, imT = card(fakeslack.ViewsChannel, "1762900000.000100"), card(mmIM, "1762890000.000100")
			mpT = card(mmMPDM, "1762880000.000100")
			return chT.people == "Ada Example, Lee Late, and you" && imT.name == "Tom Thread" && imT.people == "Tom Thread and you" &&
				mpT.name == "Ada Example, Gia Group" && mpT.people == "Ada Example and Gia Group"
		})
		e.until(t, "the senders' names", func() bool {
			a, _ := sender(fakeslack.ViewsChannel, "1762900000.000100", "1762900200.000100")
			b, _ := sender(mmIM, "1762890000.000100", "1762890000.000100")
			c, _ := sender(mmMPDM, "1762880000.000100", "1762880100.000100")
			return a == "Lee Late" && b == "Tom Thread" && c == "Gia Group"
		})
		e.until(t, "the ghost's non-existent bar", func() bool {
			name, miss := sender(fakeslack.ViewsChannel, "1762870000.000100", "1762870100.000100")
			return name == "" && miss == core.MemberNonExistent
		})
		ghT = card(fakeslack.ViewsChannel, "1762870000.000100")
		if ghT.people != "Just you" {
			t.Errorf("settled: the ghost's thread's people %q, want Just you", ghT.people)
		}
		mmNoIDs(t, "settled: drawn", drawnStrings(e)...)
		vaShot(t, e, "threads-members-named", dark)
	}
}

// ThreadParticipantList (oYYP) over the named participants: the simple
// list for three people or fewer, else the overflow, both counting only
// the named (Slack's output, worked from mapStateToProps by hand).
func TestThreadParticipantsCountTheNamed(t *testing.T) {
	cases := []struct {
		names   []string
		hasSelf bool
		all     int
		want    string
	}{
		{nil, true, 1, "Just you"},
		{[]string{"Ada"}, true, 2, "Ada and you"},
		{[]string{"Ada", "Bea"}, true, 3, "Ada, Bea, and you"},
		{[]string{"Ada"}, false, 1, "Ada"},
		{[]string{"Ada", "Bea"}, false, 2, "Ada and Bea"},
		{[]string{"Ada", "Bea", "Cy"}, false, 3, "Ada, Bea, and Cy"},
		{[]string{"Ada", "Bea", "Cy"}, true, 4, "Ada, Bea, and 2 others"},
		{[]string{"Ada", "Bea", "Cy", "Di"}, false, 4, "Ada, Bea, and 2 others"},
		// someone left out: three people, two named
		{[]string{"Ada"}, true, 3, "Ada and you"},
		{nil, true, 2, "Just you"},
		{[]string{"Ada"}, false, 2, "Ada"},
		{nil, false, 1, ""},
		// past three: the overflow over the named ("you" second)
		{[]string{"Ada"}, true, 4, "Ada, you, and 0 others"},
		{[]string{"Ada", "Bea"}, true, 5, "Ada, Bea, and 1 other"},
		// past three with one named: the simple list (Slack's text has
		// no second name there)
		{nil, true, 4, "Just you"},
		{[]string{"Ada"}, false, 5, "Ada"},
	}
	for _, c := range cases {
		if got := thrParticipants(c.names, c.hasSelf, c.all); got != c.want {
			t.Errorf("thrParticipants(%q, self %v, %d) = %q, want %q", c.names, c.hasSelf, c.all, got, c.want)
		}
	}
}

// groupNames is getMpimDisplayNameAlphabetical: the others QJub-sorted
// (Intl.Collator("en-US", {numeric: true}): case aside, 9 before 10),
// the viewer and anyone the store can't name left out; userName is ""
// for them, never the id; the DM title too.
func TestGroupNamesAlphabeticalWithoutUnknown(t *testing.T) {
	app, _, release := actNamesApp(t)
	release()
	app.S.PutUsers([]slack.User{
		{ID: "U0ZED00001", Name: "zed", RealName: "Zed Zulu"},
		{ID: "U0BEA00001", Name: "bea", RealName: "bea Bloom"},
		{ID: "U0ANN00010", Name: "ann10", RealName: "Ann 10"},
		{ID: "U0ANN00009", Name: "ann9", RealName: "Ann 9"},
	})
	b := NewActivity(&slickHost{app: app, th: theme.New(false)})
	got := b.groupNames([]string{"USELF", "U0ZED00001", "U0NOTLOADED", "U0BEA00001", "U0ANN00010", "U0ANN00009"})
	if want := "Ann 9, Ann 10, bea Bloom, Zed Zulu"; got != want {
		t.Errorf("groupNames = %q, want %q", got, want)
	}
	if n := b.userName("U0NOTLOADED"); n != "" {
		t.Errorf("userName of a member the store can't name = %q, want \"\"", n)
	}
	app.S.PutConv(slack.Conversation{ID: "D0NOTLOADED", IsIM: true, User: "U0NOTLOADED", IsMember: true})
	if n := b.convTitle("D0NOTLOADED", nil); n != "" {
		t.Errorf("the DM's title = %q, want \"\" until its member loads", n)
	}
	app.S.PutUsers([]slack.User{{ID: "U0NOTLOADED", Name: "nia", RealName: "Nia Now"}})
	if n := b.convTitle("D0NOTLOADED", nil); n != "Nia Now" {
		t.Errorf("the DM's title once its member loaded = %q, want Nia Now", n)
	}
}

// membersIn, on a "names" event's ids: the rows and cards that drew one
// of them without a name are described again, the rest kept.
func TestMembersInRedescribesWaiters(t *testing.T) {
	app, _, release := actNamesApp(t)
	release()
	v := NewActivity(&slickHost{app: app, th: theme.New(false)})
	v.rows["a"] = &actRow{built: true, d: actDesc{waits: []string{mmLate1}}}
	v.rows["b"] = &actRow{built: true, d: actDesc{waits: []string{mmLate2}}}
	v.rows["c"] = &actRow{built: true}
	v.membersIn([]string{mmLate1, "C0SOMECHAN"})
	if v.rows["a"].built || !v.rows["b"].built || !v.rows["c"].built || !v.dirty {
		t.Errorf("after names [%s]: built a %v b %v c %v, dirty %v; want only a described again", mmLate1,
			v.rows["a"].built, v.rows["b"].built, v.rows["c"].built, v.dirty)
	}
	th := NewThreads(&slickHost{app: app, th: theme.New(false)})
	th.cards["x"] = &thrCard{built: true, waits: []string{mmLate3}}
	th.cards["y"] = &thrCard{built: true}
	th.membersIn([]string{mmLate3})
	if th.cards["x"].built || !th.cards["y"].built {
		t.Errorf("Threads after names [%s]: x %v y %v, want only x made again", mmLate3, th.cards["x"].built, th.cards["y"].built)
	}
}

// missingBar is MissingText on the name's line box: the box an empty
// name gets (so a row keeps its height), 120px wide or what fits, with
// the line's baseline.
func TestMissingBarKeepsTheLineBox(t *testing.T) {
	app, _, release := actNamesApp(t)
	release()
	v := NewActivity(&slickHost{app: app, th: theme.New(false)})
	v.frame()
	var ops op.Ops
	gtx := actFrame(&ops, image.Pt(400, 100))
	gtx.Constraints.Min = image.Point{}
	for _, lh := range []unit.Sp{22, 0} {
		line := func(gtx C) D { return v.para(gtx, 15, lh, wBold, v.p.Text, 1, "") }
		want := line(gtx)
		named := v.para(gtx, 15, lh, wBold, v.p.Text, 1, "Ada Example")
		got := v.missingBar(gtx, false, nil, line)
		if got.Size != image.Pt(120, want.Size.Y) || got.Baseline != want.Baseline || named.Size.Y != want.Size.Y {
			t.Errorf("line height %v: bar %v (baseline %d), the empty line %v (%d), a name %v", lh, got.Size, got.Baseline, want.Size, want.Baseline, named.Size)
		}
		g := gtx
		g.Constraints.Max.X = 50
		if got := v.missingBar(g, true, nil, line); got.Size.X != 50 {
			t.Errorf("line height %v: the bar in 50px is %dpx wide", lh, got.Size.X)
		}
	}
}

// A preview reads its user mentions as it draws: the member's name, no
// text for one the store can't name yet (BaseMissingMember's bar adds
// none), never the "@U…" the doc was built with; once the member loads,
// their name.
func TestPreviewMentionsReadWhenDrawn(t *testing.T) {
	app, _, release := actNamesApp(t)
	release()
	b := NewActivity(&slickHost{app: app, th: theme.New(false)})
	doc := richtext.Doc{Blocks: []richtext.Block{{Kind: richtext.Paragraph, Spans: []richtext.Span{
		{Kind: richtext.Text, Text: "ask "},
		{Kind: richtext.User, Text: "@U0NOTLOADED", ID: "U0NOTLOADED"},
		{Kind: richtext.Text, Text: " and "},
		{Kind: richtext.User, Text: "@U1", ID: "U1"},
	}}}}
	m := &core.Message{Channel: "C1", TS: "1700000001.000100", Doc: doc}
	if got, want := b.preview(m), "ask  and @Alice Example"; got != want {
		t.Errorf("preview %q, want %q", got, want)
	}
	app.S.PutUsers([]slack.User{{ID: "U0NOTLOADED", Name: "nia", RealName: "Nia Now"}})
	if got, want := b.preview(m), "ask @Nia Now and @Alice Example"; got != want {
		t.Errorf("preview once the member loaded %q, want %q", got, want)
	}
}
