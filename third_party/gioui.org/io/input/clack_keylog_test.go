// SPDX-License-Identifier: Unlicense OR MIT

package input

import (
	"fmt"
	"reflect"
	"testing"

	"gioui.org/io/event"
	"gioui.org/io/key"
	"gioui.org/op"
)

func keyPress(n key.Name) key.Event   { return key.Event{Name: n, State: key.Press} }
func keyRelease(n key.Name) key.Event { return key.Event{Name: n, State: key.Release} }

// readingRouter is a router with an editor that reads ed's filters and a
// KeyPresses reader, both every frame, after two frames.
func readingRouter(ed *int, filters ...event.Filter) (*Router, func()) {
	r := new(Router)
	fs := append([]event.Filter{key.FocusFilter{Target: ed}}, filters...)
	frame := func() {
		r.Source().KeyPresses()
		events(r, -1, fs...)
		var ops op.Ops
		event.Op(&ops, ed)
		r.Frame(&ops)
	}
	frame()
	frame()
	return r, frame
}

func keyEventsNamed(evs []event.Event, n key.Name) int {
	c := 0
	for _, e := range evs {
		if ke, ok := e.(key.Event); ok && ke.Name == n {
			c++
		}
	}
	return c
}

// Clack patch 28: KeyPresses has every press, the ones a filter took and
// the ones nothing matched, once each, and not the modifier keys' own;
// the widgets get their keys as before.
func TestKeyPressesSeesTakenKeys(t *testing.T) {
	ed := new(int)
	up := key.Filter{Focus: ed, Name: key.NameUpArrow}
	r, frame := readingRouter(ed, up)
	r.Source().Execute(key.FocusCmd{Tag: ed})
	frame()
	frame()
	if r.KeyLogged() {
		t.Error("KeyLogged before any key")
	}
	r.Queue(keyPress(key.NameShift), keyPress(key.NameUpArrow), keyRelease(key.NameUpArrow), keyPress("B"))
	if !r.KeyLogged() {
		t.Error("KeyLogged false after two presses were kept: no frame would read them")
	}
	// the focused editor takes ↑ (nothing matches B)
	assertEventSequence(t, events(r, -1, key.FocusFilter{Target: ed}, up), keyPress(key.NameUpArrow), keyRelease(key.NameUpArrow))
	want := []key.Event{keyPress(key.NameUpArrow), keyPress("B")}
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, want) {
		t.Errorf("KeyPresses = %v, want %v", got, want)
	}
	if again := r.Source().KeyPresses(); len(again) != 0 {
		t.Errorf("KeyPresses again = %v, want none", again)
	}
}

// A press nothing matches is kept and asks for a frame (KeyLogged), but
// still isn't handled: WakeupTime says so, and the window still moves
// the focus for an unhandled Tab and gives macOS an unhandled key's
// command.
func TestKeyPressesUnmatchedStaysUnhandled(t *testing.T) {
	r, _ := readingRouter(new(int))
	r.WakeupTime()
	r.Queue(keyPress("B"))
	if _, handled := r.WakeupTime(); handled {
		t.Error("a press no filter matched counts as handled")
	}
	if !r.KeyLogged() {
		t.Error("KeyLogged false: no frame would read the press")
	}
	if r.KeyLogged() {
		t.Error("KeyLogged true twice for one press")
	}
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, []key.Event{keyPress("B")}) {
		t.Errorf("KeyPresses = %v, want [B]", got)
	}
}

// The router keeps presses only from the frame after a KeyPresses call,
// and stops after a frame without one.
func TestKeyPressesOnlyWhileRead(t *testing.T) {
	var r Router
	r.Frame(new(op.Ops))
	r.Queue(keyPress("B"))
	if r.KeyLogged() {
		t.Error("a press kept with no reader")
	}
	if got := r.Source().KeyPresses(); len(got) != 0 {
		t.Errorf("first KeyPresses = %v, want none (no reader before it)", got)
	}
	r.Frame(new(op.Ops))
	r.Queue(keyPress("A"))
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, []key.Event{keyPress("A")}) {
		t.Errorf("KeyPresses = %v, want [A]", got)
	}
	r.Frame(new(op.Ops)) // read this frame: still on
	r.Frame(new(op.Ops)) // not read: off
	r.Queue(keyPress("C"))
	if r.KeyLogged() {
		t.Error("a press kept after a frame without a reader")
	}
	if got := r.Source().KeyPresses(); len(got) != 0 {
		t.Errorf("KeyPresses = %v after a frame without a reader, want none", got)
	}
}

// A command that replays the queue doesn't log its presses again: the
// immediate replay (Router.execute) and the next frame's (Frame).
func TestKeyPressesReplayedOnce(t *testing.T) {
	ed := new(int)
	b := key.Filter{Name: "B"}
	r, _ := readingRouter(ed, b)
	r.Queue(keyPress("B"), keyPress("A"))
	r.Source().Execute(key.FocusCmd{Tag: ed}) // immediate: the queue is replayed
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, []key.Event{keyPress("B"), keyPress("A")}) {
		t.Errorf("after an immediate replay KeyPresses = %v, want [B A]", got)
	}
	if n := keyEventsNamed(events(r, -1, key.FocusFilter{Target: ed}, b), "B"); n != 1 {
		t.Errorf("the B filter got %d B presses, want 1", n)
	}

	ed2 := new(int)
	r, _ = readingRouter(ed2, b)
	r.Queue(keyPress("B"))
	// the editor has read its events: a focus change now waits for the
	// next frame, and the events after it with it
	events(r, -1, key.FocusFilter{Target: ed2})
	r.Source().Execute(key.FocusCmd{Tag: ed2})
	if n := keyEventsNamed(events(r, -1, b), "B"); n != 0 {
		t.Fatalf("B delivered before the deferred command ran (%d)", n)
	}
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, []key.Event{keyPress("B")}) {
		t.Errorf("KeyPresses = %v, want [B]", got)
	}
	var ops op.Ops
	event.Op(&ops, ed2)
	r.Frame(&ops) // replays B
	if got := r.Source().KeyPresses(); len(got) != 0 {
		t.Errorf("the next frame's replay logged %v again", got)
	}
	if n := keyEventsNamed(events(r, -1, key.FocusFilter{Target: ed2}, b), "B"); n != 1 {
		t.Errorf("after the replay the B filter got %d B presses, want 1", n)
	}
}

// A HoverOnly or disabled Source reads none and leaves them; the log
// keeps the last 32.
func TestKeyPressesSourcesAndBound(t *testing.T) {
	r, _ := readingRouter(new(int))
	r.Queue(keyPress("B"))
	if got := r.Source().HoverOnly().KeyPresses(); got != nil {
		t.Errorf("HoverOnly KeyPresses = %v", got)
	}
	if got := r.Source().Disabled().KeyPresses(); got != nil {
		t.Errorf("disabled KeyPresses = %v", got)
	}
	if got := r.Source().KeyPresses(); !reflect.DeepEqual(got, []key.Event{keyPress("B")}) {
		t.Errorf("KeyPresses = %v, want [B] (left by the other Sources)", got)
	}
	for i := range 40 {
		r.Queue(keyPress(key.Name(fmt.Sprint(i))))
	}
	got := r.Source().KeyPresses()
	if len(got) != keyLogMax || got[0].Name != "8" || got[len(got)-1].Name != "39" {
		t.Errorf("after 40 presses KeyPresses has %d from %v to %v, want the last %d (8 to 39)", len(got), got[0].Name, got[len(got)-1].Name, keyLogMax)
	}
}
