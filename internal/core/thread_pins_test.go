package core

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/slack"
)

// Karim, 2026-11-14 (testing on Slack Extra's messages): "latest read in
// thread doesnt seem like its working". Slack keeps a thread's
// subscription apart from its messages (store/thread_subs.go); Clack kept
// it on each copy of the root, so the root's message_changed (Slack Extra's
// anchor deletes the replies its message gets, and Slack sends the parent
// a message_changed with no "subscribed") made a followed thread read as
// unfollowed, and thread_marked's update carried the channel's copy of
// the root, which never had the subscription.
func TestThreadSubscriptionLikeSlack(t *testing.T) {
	a, f := testApp(t)
	ctx := context.Background()
	const root, r1, r2 = "1700000000.000100", "1700000060.000100", "1700000120.000100"
	// the channel's copy of the root, which came without a subscription
	// (a live message: socket copies carry none)
	a.S.SetLatest("C1", []slack.Message{{Type: "message", TS: root, ThreadTS: root, User: "U1", Text: "a synthetic root", ReplyCount: 2, LatestReply: r2}}, false)
	// conversations.replies: the root with the viewer's subscription
	f.reply["conversations.replies"] = `{"ok":true,"has_more":false,"messages":[` +
		`{"type":"message","ts":"` + root + `","thread_ts":"` + root + `","user":"U1","text":"a synthetic root","reply_count":2,"latest_reply":"` + r2 + `","subscribed":true,"last_read":"` + r1 + `"},` +
		`{"type":"message","ts":"` + r1 + `","thread_ts":"` + root + `","user":"U2","text":"one"},` +
		`{"type":"message","ts":"` + r2 + `","thread_ts":"` + root + `","user":"U2","text":"two"}]}`
	msgs, err := a.Replies(ctx, "C1", root)
	if err != nil || len(msgs) != 3 {
		t.Fatalf("replies: %d %v", len(msgs), err)
	}
	if m := msgs[0]; !m.FollowingThread || m.ThreadLastRead != r1 {
		t.Fatalf("the replies' root: following %v, last read %q; want following, r1", m.FollowingThread, m.ThreadLastRead)
	}
	// every copy reads the thread's subscription, the channel's too
	if v := a.message("C1", &a.S.Timeline("C1").Msgs[0]); !v.FollowingThread || v.ThreadLastRead != r1 {
		t.Errorf("the channel's copy: following %v, last read %q; want the thread's subscription", v.FollowingThread, v.ThreadLastRead)
	}

	evs, stop := a.Subscribe()
	defer stop()
	// another client read the thread: thread_marked's update (Mutate hands
	// back the channel's copy) still says followed
	a.handle(rtm("thread_marked", map[string]any{"subscription": map[string]any{"type": "thread", "channel": "C1", "thread_ts": root, "last_read": r2}}))
	if e := next(t, evs, "update"); e.Msg.TS != root || !e.Msg.FollowingThread || e.Msg.ThreadLastRead != r2 {
		t.Errorf("thread_marked: update %s following %v, last read %q; want following, r2", e.Msg.TS, e.Msg.FollowingThread, e.Msg.ThreadLastRead)
	}

	// the root's message_changed (an edit; the same as the parent's when a
	// reply under it is deleted): no "subscribed", so the subscription stays
	a.handle(rtm("message", map[string]any{"subtype": "message_changed", "hidden": true, "channel": "C1", "ts": "1700000200.000100",
		"message": map[string]any{"type": "message", "ts": root, "thread_ts": root, "user": "U1", "text": "a synthetic root, edited",
			"reply_count": 2, "latest_reply": r2, "edited": map[string]any{"user": "U1", "ts": "1700000200.000100"}}}))
	e := next(t, evs, "update")
	if e.Msg.TS != root || e.Msg.Text != "a synthetic root, edited" || !e.Msg.FollowingThread || e.Msg.ThreadLastRead != r2 {
		t.Errorf("message_changed: update %q following %v, last read %q; want the edit, following, r2", e.Msg.Text, e.Msg.FollowingThread, e.Msg.ThreadLastRead)
	}
	if msgs, _ = a.Replies(ctx, "C1", root); !msgs[0].FollowingThread || msgs[0].ThreadLastRead != r2 {
		t.Errorf("the thread after message_changed: following %v, last read %q", msgs[0].FollowingThread, msgs[0].ThreadLastRead)
	}
	for _, m := range []slack.Message{a.S.Timeline("C1").Msgs[0], func() slack.Message { th, _ := a.S.Thread("C1", root); return th[0] }()} {
		if !m.Subscribed || m.LastRead != r2 || m.Text != "a synthetic root, edited" {
			t.Errorf("a cached copy after message_changed: subscribed %v, last read %q, %q", m.Subscribed, m.LastRead, m.Text)
		}
	}

	// a copy with a "subscribed" of its own is the subscription: unfollowed
	// elsewhere, as a message_changed that says so
	a.handle(rtm("message", map[string]any{"subtype": "message_changed", "hidden": true, "channel": "C1", "ts": "1700000201.000100",
		"message": map[string]any{"type": "message", "ts": root, "thread_ts": root, "user": "U1", "text": "a synthetic root, edited",
			"reply_count": 2, "latest_reply": r2, "subscribed": false}}))
	if e := next(t, evs, "update"); e.Msg.FollowingThread {
		t.Error(`a copy with "subscribed": false left the thread followed`)
	}
}

// Mark unread in a thread (setThreadLastRead read false, v4te): the read
// point goes just before the reply at once (moveTimestampBackInTime),
// then subscriptions.thread.mark {ts: the reply, read 0}; a failure puts
// it back.
func TestMarkThreadUnreadLikeSlack(t *testing.T) {
	a, f := testApp(t)
	ctx := context.Background()
	const root, r1, r2 = "1700000000.000100", "1700000060.000100", "1700000120.000100"
	f.reply["conversations.replies"] = `{"ok":true,"has_more":false,"messages":[` +
		`{"type":"message","ts":"` + root + `","thread_ts":"` + root + `","user":"U1","text":"root","reply_count":2,"subscribed":true,"last_read":"` + r2 + `"},` +
		`{"type":"message","ts":"` + r1 + `","thread_ts":"` + root + `","user":"U2","text":"one"},` +
		`{"type":"message","ts":"` + r2 + `","thread_ts":"` + root + `","user":"U2","text":"two"}]}`
	if _, err := a.Replies(ctx, "C1", root); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var atSend string
	f.hooks["subscriptions.thread.mark"] = func(url.Values) {
		sub, _ := a.S.ThreadSub("C1", root)
		mu.Lock()
		atSend = sub.LastRead
		mu.Unlock()
	}
	f.reply["subscriptions.thread.mark"] = `{"ok":true}`
	if err := a.MarkThreadUnread(ctx, "C1", root, r1); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	got := atSend
	mu.Unlock()
	if want := "1700000060.000099"; got != want {
		t.Errorf("the read point as the mark went out: %q, want just before the reply %q", got, want)
	}
	if p := f.last("subscriptions.thread.mark"); p.Get("ts") != r1 || p.Get("read") != "0" || p.Get("thread_ts") != root {
		t.Errorf("subscriptions.thread.mark %v", p)
	}
	// a failed mark puts the read point back
	f.reply["subscriptions.thread.mark"] = `{"ok":false,"error":"internal_error"}`
	if err := a.MarkThreadUnread(ctx, "C1", root, root); err == nil {
		t.Fatal("the failed mark returned no error")
	}
	if sub, _ := a.S.ThreadSub("C1", root); sub.LastRead != "1700000060.000099" {
		t.Errorf("after a failed mark the read point is %q, want it back at %q", sub.LastRead, "1700000060.000099")
	}
}

// The message menu's pin (Slack's pinMessage, fBOF) and unpin
// (unpinMessage, 4mNC): the message and the Pins tab's rows change before
// pins.add / pins.remove go out (with Slack's reason), and a failure
// takes both back. Clack used to wait for the socket's pin_added, so a
// pin showed only once the event came round.
func TestPinLikeSlack(t *testing.T) {
	a, f := testApp(t)
	ctx := context.Background()
	const t1, t2 = "1700000000.000100", "1700000060.000100"
	a.S.SetLatest("C1", []slack.Message{
		{Type: "message", TS: t1, User: "U1", Text: "a synthetic message"},
		{Type: "message", TS: t2, User: "U2", Text: "another synthetic message"},
	}, false)
	// the channel's Pins tab, read once: nothing pinned
	f.reply["pins.list"] = `{"ok":true,"items":[]}`
	a.PinsTab("C1")
	dl := time.Now().Add(5 * time.Second)
	for {
		if _, loaded := a.PinsTab("C1"); loaded {
			break
		}
		if time.Now().After(dl) {
			t.Fatal("the Pins tab never loaded")
		}
		time.Sleep(5 * time.Millisecond)
	}
	type seen struct {
		pinned bool
		by     string
		rows   []string
	}
	var mu sync.Mutex
	var at seen
	look := func(ts string) func(url.Values) {
		return func(url.Values) {
			m, _ := a.S.Find("C1", ts)
			v := a.message("C1", &m)
			items, _ := a.PinsTab("C1")
			mu.Lock()
			at = seen{v.Pinned, v.PinnedBy, pinKeys(items)}
			mu.Unlock()
		}
	}
	sent := func() seen {
		mu.Lock()
		defer mu.Unlock()
		return at
	}
	state := func(ts string) seen {
		m, _ := a.S.Find("C1", ts)
		v := a.message("C1", &m)
		items, _ := a.PinsTab("C1")
		return seen{v.Pinned, v.PinnedBy, pinKeys(items)}
	}
	evs, stop := a.Subscribe()
	defer stop()

	f.hooks["pins.add"] = look(t1)
	f.reply["pins.add"] = `{"ok":true}`
	if err := a.Pin(ctx, "C1", t1); err != nil {
		t.Fatal(err)
	}
	if s := sent(); !s.pinned || s.by != "USELF" || strings.Join(s.rows, ",") != t1 {
		t.Errorf("as pins.add went out: pinned %v by %q, rows %v; want pinned by you, the row first", s.pinned, s.by, s.rows)
	}
	if p := f.last("pins.add"); p.Get("channel") != "C1" || p.Get("timestamp") != t1 || p.Get("reason") != "pin-message" {
		t.Errorf("pins.add %v", p)
	}
	if e := next(t, evs, "update"); e.Msg.TS != t1 || !e.Msg.Pinned || e.Msg.PinnedBy != "USELF" {
		t.Errorf("the update: %s pinned %v by %q", e.Msg.TS, e.Msg.Pinned, e.Msg.PinnedBy)
	}

	// a channel at its 100 pins: both go back, and the caller can tell
	f.hooks["pins.add"] = look(t2)
	f.reply["pins.add"] = `{"ok":false,"error":"too_many_pins"}`
	if err := a.Pin(ctx, "C1", t2); !IsTooManyPins(err) {
		t.Errorf("Pin's error %v, want too_many_pins", err)
	}
	if s := sent(); !s.pinned || strings.Join(s.rows, ",") != t2+","+t1 {
		t.Errorf("as the refused pins.add went out: pinned %v, rows %v", s.pinned, s.rows)
	}
	if s := state(t2); s.pinned || strings.Join(s.rows, ",") != t1 {
		t.Errorf("after too_many_pins: pinned %v, rows %v; want neither", s.pinned, s.rows)
	}

	// unpin: gone before pins.remove goes out
	f.hooks["pins.remove"] = look(t1)
	f.reply["pins.remove"] = `{"ok":true}`
	if err := a.Unpin(ctx, "C1", t1); err != nil {
		t.Fatal(err)
	}
	if s := sent(); s.pinned || len(s.rows) != 0 {
		t.Errorf("as pins.remove went out: pinned %v, rows %v; want neither", s.pinned, s.rows)
	}
	if p := f.last("pins.remove"); p.Get("channel") != "C1" || p.Get("timestamp") != t1 || p.Get("reason") != "unpin-message" {
		t.Errorf("pins.remove %v", p)
	}

	// a failed unpin puts both back
	f.reply["pins.add"] = `{"ok":true}`
	f.hooks["pins.add"] = nil
	if err := a.Pin(ctx, "C1", t1); err != nil {
		t.Fatal(err)
	}
	f.reply["pins.remove"] = `{"ok":false,"error":"internal_error"}`
	if err := a.Unpin(ctx, "C1", t1); err == nil {
		t.Fatal("the failed unpin returned no error")
	}
	if s := state(t1); !s.pinned || strings.Join(s.rows, ",") != t1 {
		t.Errorf("after a failed pins.remove: pinned %v, rows %v; want both back", s.pinned, s.rows)
	}
}
