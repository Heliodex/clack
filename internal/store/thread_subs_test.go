package store

import (
	"encoding/json"
	"testing"

	"github.com/some-du6e/clack/internal/slack"
)

func decodeMsg(t *testing.T, s string) slack.Message {
	t.Helper()
	var m slack.Message
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatal(err)
	}
	return m
}

// A copy says whether it had a "subscribed" of its own: what Slack's
// slurpThreadSubscriptions (hasOwnProperty) goes by.
func TestMessageSubscribedSet(t *testing.T) {
	for _, c := range []struct {
		json     string
		set, sub bool
	}{
		{`{"ts":"1.000001"}`, false, false},
		{`{"ts":"1.000001","subscribed":false}`, true, false},
		{`{"ts":"1.000001","subscribed":true,"last_read":"1.000002"}`, true, true},
		{`{"ts":"1.000001","subscribed":null}`, false, false},
	} {
		m := decodeMsg(t, c.json)
		if m.SubscribedSet != c.set || m.Subscribed != c.sub {
			t.Errorf("%s: set %v subscribed %v, want %v %v", c.json, m.SubscribedSet, m.Subscribed, c.set, c.sub)
		}
	}
}

// A thread's subscription, as Slack's threadSub store keeps it apart from
// the messages: a copy with its own "subscribed" sets it, a copy without
// one (the socket's message_changed) leaves it, every cached copy of the
// root reads it, and a fetch sent before a subscription event doesn't
// undo the event.
func TestThreadSubsLikeSlack(t *testing.T) {
	s := New("")
	const ch, root = "C1", "1700000000.000100"
	const r1, r2, r3 = "1700000060.000100", "1700000120.000100", "1700000180.000100"

	// nothing known: the copy's own fields
	own := slack.Message{TS: root, ThreadTS: root, Subscribed: true, LastRead: r1}
	if got := s.ThreadSubOf(ch, &own); !got.Subscribed || got.LastRead != r1 {
		t.Errorf("no subscription known: %+v, want the copy's own", got)
	}

	// the socket's message_changed for the root has no "subscribed"
	edited := decodeMsg(t, `{"type":"message","ts":"`+root+`","thread_ts":"`+root+`","text":"edited","reply_count":2}`)
	s.SlurpSub(ch, &edited)
	if sub, ok := s.ThreadSub(ch, root); ok {
		t.Errorf("a copy without subscribed set the subscription %+v", sub)
	}

	// a replies answer's root has one: the subscription
	since := s.SubSeq()
	fetched := []slack.Message{decodeMsg(t, `{"type":"message","ts":"`+root+`","thread_ts":"`+root+`","subscribed":true,"last_read":"`+r1+`"}`)}
	s.SyncSubs(ch, since, fetched)
	if sub, ok := s.ThreadSub(ch, root); !ok || !sub.Subscribed || sub.LastRead != r1 {
		t.Fatalf("after the replies answer: %+v %v, want followed, read to r1", sub, ok)
	}
	// and every copy reads it, the edited one included
	if got := s.ThreadSubOf(ch, &edited); !got.Subscribed || got.LastRead != r1 {
		t.Errorf("the edited copy reads %+v, want the thread's subscription", got)
	}
	s.SlurpSub(ch, &edited)
	if sub, _ := s.ThreadSub(ch, root); !sub.Subscribed || sub.LastRead != r1 {
		t.Errorf("a later message_changed without subscribed changed it to %+v", sub)
	}

	// thread_marked while a fetch is out: the answer, older, keeps the mark
	since = s.SubSeq()
	s.SetThreadSub(ch, root, func(x *ThreadSub) { x.LastRead = r2 })
	s.SyncSubs(ch, since, fetched) // answered read to r1
	if sub, _ := s.ThreadSub(ch, root); !sub.Subscribed || sub.LastRead != r2 {
		t.Errorf("a fetch sent before thread_marked undid it: %+v", sub)
	}
	// a fetch sent after it stands, unfollowed included
	s.SyncSubs(ch, s.SubSeq(), []slack.Message{decodeMsg(t, `{"ts":"`+root+`","subscribed":false}`)})
	if sub, _ := s.ThreadSub(ch, root); sub.Subscribed || sub.LastRead != "" {
		t.Errorf("after a later answer with subscribed false: %+v", sub)
	}

	// an event for a thread with no subscription yet starts from the
	// cached root (the thread's copy, from conversations.replies) and
	// writes every copy
	const other = "1700000300.000100"
	s.SetThread(ch, other, []slack.Message{{TS: other, ThreadTS: other, Subscribed: true, LastRead: r1}})
	s.SetLatest(ch, []slack.Message{{TS: other, ThreadTS: other}}, false)
	s.SetThreadSub(ch, other, func(x *ThreadSub) { x.LastRead = r3 })
	if sub, _ := s.ThreadSub(ch, other); !sub.Subscribed || sub.LastRead != r3 {
		t.Errorf("thread_marked on a thread nothing fetched yet: %+v, want followed, read to r3", sub)
	}
	if m, _ := s.Find(ch, other); !m.Subscribed || m.LastRead != r3 {
		t.Errorf("the channel's copy reads subscribed %v, last read %q", m.Subscribed, m.LastRead)
	}
}
