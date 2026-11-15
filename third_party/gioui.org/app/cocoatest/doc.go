// SPDX-License-Identifier: Unlicense OR MIT

// Package cocoatest is a Clack patch (CLACK_PATCHES.md §24): AppKit
// helpers for the macOS window tests, Gio's (gioui.org/app) and Clack's.
// Test files can't use cgo, so what they need to do to a real NSWindow
// lives here: read its title bar, buttons and frames, hit-test it as
// AppKit routes a click, post synthesized clicks, press its buttons, call
// the app delegate's reopen, post the app's activation, send menu key
// equivalents, post key presses as a keyboard makes them (the current
// layout's characters) and trace what the view's key methods get.
//
// Only tests import it. Everything here runs on AppKit's main thread
// (OnMain), so the program must be running app.Main.
package cocoatest
