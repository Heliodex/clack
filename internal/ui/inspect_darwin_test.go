//go:build darwin && cgo

package ui

import (
	"testing"
	"time"

	"gioui.org/app/cocoatest"
)

// TestMacInspectKeys is the inspect tool's keys on App.Run's own window
// on a real macOS desktop (CLACK_MACOS_WINDOW_TESTS=1, ci's macos job;
// TestMain runs app.Main), signed in to internal/fakeslack. The keys are
// posted as a keyboard makes them (cocoatest.PostKey: the modifiers'
// flagsChanged, the key's down and up with the current layout's
// characters) and AppKit delivers them: NSApp sendEvent:, the menu bar's
// key equivalents, then Gio's view, whose interpretKeyEvents: hands them
// to Gio. The log shows what the view got of each (cocoatest.TraceKeys).
//
// Reproduction: with CLACK_INSPECT=1, ⌘⌥⇧D through AppKit starts a
// session and again ends it.
func TestMacInspectKeys(t *testing.T) {
	if !macWindowTests {
		t.Skip("CLACK_MACOS_WINDOW_TESTS not set")
	}
	t.Setenv("CLACK_INSPECT", "1")
	ma := runMacBoundsApp(t, nil)
	defer ma.quit(t, true)
	var layout string
	onMain(func() {
		cocoatest.OrderFront(ma.view)
		cocoatest.TraceKeys(ma.view)
		layout = cocoatest.KeyboardLayout()
	})
	t.Logf("keyboard layout %s", layout)
	ma.until(t, "the key window", func() bool { return ma.state().Key })
	var hasTool bool
	ma.onUI(t, func() { hasTool = ma.a.insp != nil })
	if !hasTool {
		t.Fatal("CLACK_INSPECT=1 but the window has no inspect tool")
	}
	active := func() (on bool) {
		ma.onUI(t, func() { on = ma.a.insp != nil && ma.a.insp.Active() })
		return on
	}
	sidebarHidden := func() (h bool) {
		ma.onUI(t, func() { h = ma.a.sidebar.hidden })
		return h
	}
	trace := func() (s string) {
		onMain(func() { s = cocoatest.KeyTrace() })
		return s
	}
	press := func(name string, code, mods int, direct bool) {
		t.Helper()
		var c, ign string
		onMain(func() {
			if direct {
				c, ign = cocoatest.SendKey(ma.view, code, mods)
			} else {
				c, ign = cocoatest.PostKey(ma.view, code, mods)
			}
		})
		time.Sleep(300 * time.Millisecond) // the run loop delivers, a frame reads
		t.Logf("%s (characters %+q, ignoring modifiers %+q); the view got:\n%s", name, c, ign, trace())
	}
	waitActive := func(want bool) bool {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if active() == want {
				return true
			}
			time.Sleep(50 * time.Millisecond)
		}
		return false
	}
	const cmd, opt, shift, ctrl = cocoatest.Cmd, cocoatest.Opt, cocoatest.Shift, cocoatest.Ctrl

	// 1. ⌘⌥⇧D, as Karim pressed it
	sb := sidebarHidden()
	press("⌘⌥⇧D through NSApp", cocoatest.KeyD, cmd|opt|shift, false)
	started := waitActive(true)
	if sbNow := sidebarHidden(); sbNow != sb {
		t.Errorf("⌘⌥⇧D toggled the sidebar (hidden %v → %v): the menu's Hide Sidebar ⌘⇧D took it", sb, sbNow)
	}
	if !started {
		var took bool
		onMain(func() { took = cocoatest.KeyEquivalent("d", cmd|opt|shift) })
		t.Logf("the main menu's performKeyEquivalent: for ⌘⌥⇧D: took %v", took)
		press("⌘⌥⇧D straight to the view", cocoatest.KeyD, cmd|opt|shift, true)
		t.Errorf("⌘⌥⇧D through AppKit started no session (straight to the view: session %v)", waitActive(true))
	} else {
		// 2. again ends it
		press("⌘⌥⇧D again", cocoatest.KeyD, cmd|opt|shift, false)
		if !waitActive(false) {
			t.Error("⌘⌥⇧D again didn't end the session")
		}
	}
	if active() {
		press("Esc", cocoatest.KeyEscape, 0, false)
		waitActive(false)
	}

	// 3. what AppKit makes of Hyper+D and the Konami code's keys (logged)
	press("⌃⌥⇧⌘D through NSApp", cocoatest.KeyD, cmd|opt|shift|ctrl, false)
	t.Logf("⌃⌥⇧⌘D: session %v", active())
	if active() {
		press("Esc", cocoatest.KeyEscape, 0, false)
	}
	for _, k := range []struct {
		name string
		code int
	}{
		{"↑", cocoatest.KeyUp}, {"↓", cocoatest.KeyDown}, {"←", cocoatest.KeyLeft}, {"→", cocoatest.KeyRight},
		{"B", cocoatest.KeyB}, {"A", cocoatest.KeyA},
	} {
		press(k.name, k.code, 0, false)
	}
}
