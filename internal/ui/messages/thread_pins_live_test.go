package messages

// Karim's two reports of 2026-11-14, testing on the Slack Extra app's
// messages, on internal/fakeslack (synthetic, nothing talks to Slack):
//
//   - "latest read in thread doesnt seem like its working": Slack Extra's
//     anchor deletes the replies its message gets (Slack then sends the
//     root a message_changed, with no "subscribed") and posts an
//     ephemeral "please don't reply" in the thread. Clack kept the
//     thread's subscription on the root's copy, so the message_changed
//     made the pane take the followed thread for an unfollowed one: no
//     "New" marker, never marked read. And it took the ephemeral for the
//     thread's newest message, the read point it would mark. Slack keeps
//     the subscription apart (threadSub, store/thread_subs.go) and marks
//     to maxRealTs, which skips ephemerals (8GqV isRealMsg).
//   - "the pinned thing is not updating right": a pin from another client
//     (pin_added, pin_removed) changes the row live; the viewer's own pin
//     from the menu shows at once, before pins.add answers, as Slack's
//     pinMessage does (Clack used to wait for the socket's pin_added).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/theme"
)

// liveCore turns app's realtime socket on (the booted envs start with it
// off) and waits until fakeslack holds the socket, so Push reaches core.
func liveCore(t *testing.T, app *core.App, fs *fakeslack.Server) {
	t.Helper()
	evs, stop := app.Subscribe()
	defer stop()
	app.SetRealtime(true)
	dl := time.After(15 * time.Second)
	for connected := false; !connected; {
		select {
		case ev := <-evs:
			connected = ev.Type == "connection" && ev.Connected
		case <-dl:
			t.Fatal("the realtime socket never connected")
		}
	}
	for d := time.Now().Add(5 * time.Second); fs.Sockets() == 0; {
		if time.Now().After(d) {
			t.Fatal("fakeslack holds no socket")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestThreadsAuditRootChangedStaysRead(t *testing.T) {
	app, fs := auditEnv(t)
	liveCore(t, app, fs)
	root := fakeslack.ThreadsAuditUnread
	r2, r5 := fakeslack.ThreadsAuditReply(root, 2), fakeslack.ThreadsAuditReply(root, 5)
	tv, host, frame := auditPane(t, app, root, false, 700)
	const want = "root sep r0 r1 r2 new+unread r3 r4 r5"
	if got := auditRows(tv, root); got != want {
		t.Fatalf("rows %q, want %q", got, want)
	}
	rm, _ := tv.s.threadRoot()
	edited := rm.Text + " (edited)"
	// the root's message_changed, as Slack sends the parent's when a reply
	// under it is deleted (or the root is edited): no "subscribed" of its own
	fs.Push(map[string]any{"type": "message", "subtype": "message_changed", "hidden": true, "channel": fakeslack.ThreadsAuditChannel,
		"ts": "1762710000.000200", "event_ts": "1762710000.000200",
		"message": map[string]any{"type": "message", "ts": root, "thread_ts": root, "user": rm.User, "text": edited,
			"reply_count": rm.ReplyCount, "reply_users": rm.ReplyUsers, "reply_users_count": rm.ReplyUsersN, "latest_reply": rm.LatestReply,
			"edited": map[string]any{"user": rm.User, "ts": "1762710000.000200"}}})
	until(t, 10*time.Second, "the edited root in the pane", frame, func() bool { r, ok := tv.s.threadRoot(); return ok && r.Text == edited })
	if sub, lr := tv.s.threadSub(); !sub || lr != r2 {
		t.Fatalf("after the root's message_changed the pane reads followed %v, last read %q; Slack: followed, read to r2 %s", sub, lr, r2)
	}
	frame()
	if got := auditRows(tv, root); got != want {
		t.Errorf("after the root's message_changed: rows %q, want %q", got, want)
	}

	// an app's ephemeral in the thread: shown, never the read point
	eph := fmt.Sprintf("%d.000300", time.Now().Unix())
	fs.Push(map[string]any{"type": "message", "channel": fakeslack.ThreadsAuditChannel, "ts": eph, "thread_ts": root,
		"user": fakeslack.AppUserID, "bot_id": fakeslack.AppBotID, "is_ephemeral": true,
		"text": "Please start a new thread instead of replying to this one."})
	until(t, 10*time.Second, "the ephemeral reply in the pane", frame, func() bool {
		n := len(tv.s.snap.msgs)
		return n > 0 && tv.s.snap.msgs[n-1].m.TS == eph
	})
	if mx := tv.s.maxMarkable(); mx != r5 {
		t.Errorf("maxMarkable %q, want the newest real reply %q (maxRealTs skips ephemerals)", mx, r5)
	}

	// the user's attention: marked read to r5, once; the marker turns
	// into the ghost
	host.attention.Store(true)
	until(t, 10*time.Second, "subscriptions.thread.mark", frame, func() bool { return len(fs.ThreadsAuditMarks()) > 0 })
	if m := fs.ThreadsAuditMarks()[0]; m.TS != r5 || m.ThreadTS != root {
		t.Errorf("marked %+v, want the thread read to r5 %s", m, r5)
	}
	until(t, 10*time.Second, "the ghost", frame, func() bool { return strings.Contains(auditRows(tv, root), "new+ghost") })
	for i := 0; i < 25; i++ {
		frame()
		time.Sleep(20 * time.Millisecond)
	}
	if n := len(fs.ThreadsAuditMarks()); n != 1 {
		t.Errorf("marked %d times, want once: %+v", n, fs.ThreadsAuditMarks())
	}
}

// pinsCore is a booted core on fakeslack's #pins-gallery, realtime on or
// off.
func pinsCore(t *testing.T, live bool) (*core.App, *fakeslack.Server) {
	t.Helper()
	SetTaut(nil)
	sz := fakeslack.Small
	sz.PinsFolders = true
	fs, err := fakeslack.Start(sz)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	app := core.New(slack.New(fs.Creds()), store.New(""))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := make(chan error, 1)
	go func() { done <- app.Boot(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("boot: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("boot timed out")
	}
	if live {
		liveCore(t, app, fs)
	}
	return app, fs
}

// pinRow waits for ts's row in v and returns a reader of it.
func pinRow(t *testing.T, v *ChannelView, f func(), ts string) func() *row {
	t.Helper()
	get := func() *row { return v.s.rows[ts] }
	until(t, 15*time.Second, "the row of "+ts, f, func() bool { return get() != nil })
	return get
}

func TestPinsLiveInChannel(t *testing.T) {
	app, fs := pinsCore(t, true)
	v := NewChannelView(coreHost{app: app, th: theme.New(false)}, fakeslack.PinsChannelID)
	v.Preview = true // no conversations.mark
	f := frameFn(v)
	ts := fakeslack.PinsTS(1) // not pinned
	row := pinRow(t, v, f, ts)
	if r := row(); r.m.Pinned {
		t.Fatalf("%s starts pinned", ts)
	}
	ev, code := fs.Pin(fakeslack.SelfID, fakeslack.PinsChannelID, ts, true)
	if code != "" {
		t.Fatal(code)
	}
	fs.Push(ev)
	until(t, 10*time.Second, "pin_added on the row", f, func() bool {
		r := row()
		return r != nil && r.m.Pinned && r.m.PinnedBy == fakeslack.SelfID
	})
	if ls := paneLabels(f); !drewLabel(ls, "Pinned by you") {
		t.Errorf("the pinned row drew no \"Pinned by you\": %q", ls)
	}
	ev, _ = fs.Pin(fakeslack.SelfID, fakeslack.PinsChannelID, ts, false)
	fs.Push(ev)
	until(t, 10*time.Second, "pin_removed on the row", f, func() bool { r := row(); return r != nil && !r.m.Pinned })
	if ls := paneLabels(f); drewLabel(ls, "Pinned by you") {
		t.Error("the unpinned row still drew \"Pinned by you\"")
	}
}

// The menu's pin with no socket (so no pin_added comes) and pins.add slow
// to answer: the row shows "Pinned by you" first; the unpin likewise.
func TestPinShowsAtOnce(t *testing.T) {
	app, fs := pinsCore(t, false)
	v := NewChannelView(coreHost{app: app, th: theme.New(false)}, fakeslack.PinsChannelID)
	v.Preview = true // no conversations.mark
	f := frameFn(v)
	ts := fakeslack.PinsTS(3) // not pinned
	row := pinRow(t, v, f, ts)
	for _, on := range []bool{true, false} {
		method := map[bool]string{true: "pins.add", false: "pins.remove"}[on]
		fs.SetFault(method, fakeslack.Fault{ReplyDelay: 4 * time.Second, Times: 1})
		start := time.Now()
		done := make(chan error, 1)
		go func() {
			if on {
				done <- app.Pin(context.Background(), fakeslack.PinsChannelID, ts)
			} else {
				done <- app.Unpin(context.Background(), fakeslack.PinsChannelID, ts)
			}
		}()
		until(t, 3*time.Second, method+"'s change before it answers", f, func() bool {
			r := row()
			return r != nil && r.m.Pinned == on && (!on || r.m.PinnedBy == fakeslack.SelfID)
		})
		if d := time.Since(start); d >= 4*time.Second {
			t.Errorf("%s: the row changed only after %v, when the answer came", method, d)
		}
		if on {
			if ls := paneLabels(f); !drewLabel(ls, "Pinned by you") {
				t.Errorf("the row pinned at once drew no \"Pinned by you\": %q", ls)
			}
		}
		if err := <-done; err != nil {
			t.Fatalf("%s: %v", method, err)
		}
		f()
		if r := row(); r.m.Pinned != on {
			t.Errorf("after %s answered the row reads pinned %v", method, r.m.Pinned)
		}
	}
}
