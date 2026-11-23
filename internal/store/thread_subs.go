package store

import (
	"github.com/some-du6e/clack/internal/slack"
)

// Thread subscriptions, kept apart from the messages as Slack keeps them
// (build c07ab895506f, read from the public CDN without a cookie):
//
//   - The threadSub objects (client-boot 8l6l getSubscription, by
//     channel/thread ts) hold {subscribed, lastRead}. preprocessImsgs'
//     slurpThreadSubscriptions (udbq) upserts one from every message that
//     has a "subscribed" field of its own, as conversations.history and
//     conversations.replies roots do ({subscribed, lastRead: last_read},
//     bulkSetSubscriptions 049b: the copy's last_read, absent or not). A
//     copy without the field leaves the subscription alone, though
//     messageChanged's replaceMessages (boot-render GG) swaps the message
//     itself wholesale.
//   - thread_subscribed / thread_unsubscribed set subscribed (and
//     lastRead), thread_marked and the viewer's own marks lastRead alone
//     (setSubscription, AoK2).
//   - Readers take the subscription before the root's own copy (Prtm:
//     `u?.subscribed ?? s?._subscribed`), so every copy of a root reads
//     the same.
//
// Clack used to keep the state on each cached copy of the root only: a
// message_changed for the root (an edit, an unfurl, or a reply deleted
// under it, as Slack Extra's anchor deletes replies to its message)
// wiped it, and the thread pane took a followed thread for one nobody
// follows: no "New" marker, never marked read. And Mutate hands back the
// timeline's copy, which could disagree with the thread's.
//
// A fetch answers what was true when it was sent: a subscription event
// that came meanwhile is newer and stays (SyncSubs), as pin events do
// (pins.go).

// ThreadSub is a thread's subscription: whether the viewer follows it and
// the last reply they read.
type ThreadSub struct {
	Subscribed bool
	LastRead   string
}

type subRec struct {
	ThreadSub
	seq uint64 // the event that last set it (0: a fetched or socket copy)
}

// maxSubRecs bounds the remembered subscriptions (oldest first out; a
// later fetch brings any it forgot back).
const maxSubRecs = 4096

// SubSeq is the number of subscription events so far; a fetch reads it
// before its request and passes it to SyncSubs.
func (s *Store) SubSeq() uint64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.subSeq
}

// ThreadSub is ch/ts's subscription when one is known.
func (s *Store) ThreadSub(ch, ts string) (ThreadSub, bool) {
	if ch == "" || ts == "" {
		return ThreadSub{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.subs[ch+"/"+ts]
	return r.ThreadSub, ok
}

// ThreadSubOf is what a root reads: ch/ts's subscription, else the copy
// m's own fields.
func (s *Store) ThreadSubOf(ch string, m *slack.Message) ThreadSub {
	if sub, ok := s.ThreadSub(ch, m.TS); ok {
		return sub
	}
	return ThreadSub{Subscribed: m.Subscribed, LastRead: m.LastRead}
}

// SetThreadSub applies a subscription event (or the viewer's own write)
// to ch/ts: f changes the subscription, which starts from the cached
// root's own fields when none is known yet. The cached copies of the
// root take it too, for the readers of a raw copy.
func (s *Store) SetThreadSub(ch, ts string, f func(*ThreadSub)) ThreadSub {
	if ch == "" || ts == "" {
		return ThreadSub{}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := ch + "/" + ts
	r, ok := s.subs[k]
	if !ok {
		if m, found := s.findLocked(ch, ts, true); found {
			r.ThreadSub = ThreadSub{Subscribed: m.Subscribed, LastRead: m.LastRead}
		}
	}
	f(&r.ThreadSub)
	s.subSeq++
	r.seq = s.subSeq
	s.putSubLocked(k, r)
	sub := r.ThreadSub
	s.mutateLocked(ch, ts, func(m *slack.Message) { m.Subscribed, m.LastRead = sub.Subscribed, sub.LastRead })
	return sub
}

// SlurpSub is slurpThreadSubscriptions for a copy of a message in ch that
// just came over the socket: one with its own "subscribed" field sets the
// thread's subscription; any other leaves it.
func (s *Store) SlurpSub(ch string, m *slack.Message) {
	if !m.SubscribedSet || ch == "" || m.TS == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.putSubLocked(ch+"/"+m.TS, subRec{ThreadSub: ThreadSub{Subscribed: m.Subscribed, LastRead: m.LastRead}})
}

// SyncSubs runs after a history or replies fetch of ch whose request
// began at SubSeq since: each answered copy with a "subscribed" field
// sets its thread's subscription, unless an event changed that
// subscription after the request went out (it is newer than the answer).
func (s *Store) SyncSubs(ch string, since uint64, fetched []slack.Message) {
	if ch == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range fetched {
		m := &fetched[i]
		if !m.SubscribedSet || m.TS == "" {
			continue
		}
		k := ch + "/" + m.TS
		if r, ok := s.subs[k]; ok && r.seq > since {
			continue
		}
		s.putSubLocked(k, subRec{ThreadSub: ThreadSub{Subscribed: m.Subscribed, LastRead: m.LastRead}})
	}
}

// putSubLocked stores r under k, forgetting the oldest past maxSubRecs.
// s.mu is held.
func (s *Store) putSubLocked(k string, r subRec) {
	if s.subs == nil {
		s.subs = map[string]subRec{}
	}
	if _, ok := s.subs[k]; !ok {
		s.subQ = append(s.subQ, k)
		for len(s.subQ) > maxSubRecs {
			delete(s.subs, s.subQ[0])
			s.subQ = s.subQ[1:]
		}
	}
	s.subs[k] = r
}

// findLocked is Find under s.mu; threadFirst looks in the threads before
// the timeline (a thread's root from conversations.replies carries the
// subscription).
func (s *Store) findLocked(ch, ts string, threadFirst bool) (slack.Message, bool) {
	inThreads := func() (slack.Message, bool) {
		if th, ok := s.threads[ch+"/"+ts]; ok {
			for i := range th {
				if th[i].TS == ts {
					return th[i], true
				}
			}
		}
		return slack.Message{}, false
	}
	if threadFirst {
		if m, ok := inThreads(); ok {
			return m, true
		}
	}
	if t := s.timelines[ch]; t != nil {
		for i := range t.Msgs {
			if t.Msgs[i].TS == ts {
				return t.Msgs[i], true
			}
		}
	}
	if !threadFirst {
		return inThreads()
	}
	return slack.Message{}, false
}
