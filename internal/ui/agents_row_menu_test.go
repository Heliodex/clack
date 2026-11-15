package ui

import (
	"image"
	"net/url"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/ui/popups"
	"github.com/some-du6e/clack/internal/ui/theme/pixeltest"
	"github.com/some-du6e/clack/internal/ui/views"
)

// The reworked Agents & apps roster's "…" (views/details_agents_menu.go,
// core/exp_wave10.go) on fakeslack's #agents-lab, where the viewer holds
// REMOVE_FROM_CHANNEL: under agent_channel_header the agents and apps
// get "Remove from channel" (canUserRemoveAppFromChannel), the button
// shows on the row's hover at its right edge, a click on it opens the
// menu below it (bottom-right) without opening the row's card, and the
// item opens RemoveMemberDialog, whose Remove kicks the app out
// (conversations.kick) and takes its row away.
func TestAgentsRowMenuRemove(t *testing.T) {
	e, fs := agentsRowMenuEnv(t, "on", true)
	ch := fakeslack.ChannelAgentsID
	d := agentsRowMenuTab(t, e, ch)
	for _, id := range []string{fakeslack.ChannelAgentID, fakeslack.ChannelAppID, fakeslack.AppUserID} {
		if got := d.AgentRowMenuItems(id); !slices.Equal(got, []string{"Remove from channel"}) {
			t.Errorf("%s's menu %q", id, got)
		}
	}

	// hover Build Status's row, then its button
	row, btn := d.AgentRowClickables(fakeslack.ChannelAppID)
	mid := e.size.X / 2
	at, ok := e.scanHover(row, image.Pt(mid, 0), image.Pt(mid, e.size.Y-1))
	if !ok {
		t.Fatal("Build Status's row never took the hover")
	}
	e.frame()
	y := at.Y + 28 // the 60px row's middle (the scan stops within 4px of its top)
	bp, ok := e.scanHover(btn, image.Pt(at.X, y), image.Pt(e.size.X-1, y))
	if !ok {
		t.Fatal("no \"…\" on the hovered row")
	}
	e.darkShot("agents-row-menu-hover")
	cards := e.profileCards()
	e.click(bp)
	e.frame()
	e.frame()
	m := e.topMenu()
	if m == nil {
		t.Fatal("the \"…\" opened no menu")
	}
	if got := m.Labels(); !slices.Equal(got, []string{"Remove from channel"}) {
		t.Errorf("menu %q", got)
	}
	if e.profileCards() != cards {
		t.Error("the button's click also opened the row's card")
	}
	anchor := d.AgentRowMenuAnchor(fakeslack.ChannelAppID)
	if !bp.In(anchor) || anchor.Dx() != anchor.Dy() {
		t.Errorf("the menu's anchor %v doesn't hold the click %v", anchor, bp)
	}
	if r := m.Rect(); r.Min.Y != anchor.Max.Y || r.Max.X != anchor.Max.X {
		t.Errorf("menu at %v, want bottom-right of %v", r, anchor)
	}
	e.darkShot("agents-row-menu-open")

	// Remove from channel: the dialog, then the kick
	rr := m.RowRect("Remove from channel")
	e.click(rr.Min.Add(rr.Size().Div(2)))
	e.frame()
	e.frame()
	dlg, ok := topOverlay[*popups.RemoveMemberDialog](e.a)
	if !ok {
		t.Fatalf("no RemoveMemberDialog; overlays %v", e.overlayTypes())
	}
	if c := dlg.Copy; c.Title != "Remove Build Status from #agents-lab?" ||
		c.Para != "They’ll still be able to rejoin, or be added to the conversation." || c.Go != "Remove" {
		t.Errorf("dialog %+v", c)
	}
	e.darkShot("agents-row-remove-dialog")
	if !dlg.Press("Remove") {
		t.Fatal("no Remove button")
	}
	e.until("conversations.kick", 10*time.Second, func() bool {
		e.frame()
		return slices.Contains(fs.Kicked(ch), fakeslack.ChannelAppID)
	})
	e.until("the dialog closes", 5*time.Second, func() bool { e.frame(); return dlg.Dismissed() })
	e.until("Build Status leaves the roster", 10*time.Second, func() bool {
		e.frame()
		return d.AgentsRosterLoaded() && !slices.Contains(d.AgentRosterIDs(), fakeslack.ChannelAppID)
	})
	if got := d.AgentRosterIDs(); !slices.Contains(got, fakeslack.ChannelAgentID) {
		t.Errorf("roster after the kick %v", got)
	}
}

// A kick the server refuses keeps the dialog open with renderError's
// alert, and Remove stays disabled.
func TestAgentsRowMenuRemoveFails(t *testing.T) {
	e, fs := agentsRowMenuEnv(t, "on", true)
	ch := fakeslack.ChannelAgentsID
	fs.SetHook(func(method string, _ url.Values) (any, bool) {
		if method == "conversations.kick" {
			return map[string]any{"ok": false, "error": "restricted_action"}, true
		}
		return nil, false
	})
	d := agentsRowMenuTab(t, e, ch)
	if d.OpenAgentRowMenu(fakeslack.ChannelAgentID) == nil {
		t.Fatal("no menu for Research Agent")
	}
	e.frame()
	m := e.topMenu()
	rr := m.RowRect("Remove from channel")
	e.click(rr.Min.Add(rr.Size().Div(2)))
	e.frame()
	e.frame()
	dlg, ok := topOverlay[*popups.RemoveMemberDialog](e.a)
	if !ok {
		t.Fatal("no RemoveMemberDialog")
	}
	dlg.Press("Remove")
	want := "Hmm, looks like you don’t have permission to kick from channels. Please contact a Workspace Admin if Research Agent should no longer have access to your workspace."
	e.until("the error alert", 10*time.Second, func() bool { e.frame(); return dlg.Error() != "" })
	if dlg.Error() != want || dlg.Dismissed() {
		t.Errorf("error %q (dismissed %v), want %q", dlg.Error(), dlg.Dismissed(), want)
	}
	e.darkShot("agents-row-remove-error")
	if dlg.Press("Remove") && dlg.Pending() {
		t.Error("Remove ran again after the error")
	}
}

// agent_channel_header off (or unassigned): the agents and apps go
// through canUserKickMemberFromChannel, which the grant passes too, so
// the rows keep "Remove from channel" (Slack's off group has the menu;
// only its checks and the dialog's private-channel wording differ).
// Without REMOVE_FROM_CHANNEL no row has a menu in either group.
func TestAgentsRowMenuGroups(t *testing.T) {
	for _, g := range []string{"off", ""} {
		t.Run("grant/"+g, func(t *testing.T) {
			e, _ := agentsRowMenuEnvOpts(t, g, true, false)
			d := agentsRowMenuTab(t, e, fakeslack.ChannelAgentsID)
			if got := d.AgentRowMenuItems(fakeslack.ChannelAppID); !slices.Equal(got, []string{"Remove from channel"}) {
				t.Errorf("group %q: menu %q", g, got)
			}
		})
	}
	for _, g := range []string{"on", "off"} {
		t.Run("nogrant/"+g, func(t *testing.T) {
			e, _ := agentsRowMenuEnvOpts(t, g, false, false)
			d := agentsRowMenuTab(t, e, fakeslack.ChannelAgentsID)
			ids := d.AgentRosterIDs()
			if len(ids) == 0 {
				t.Fatal("an empty roster")
			}
			for _, id := range ids {
				if got := d.AgentRowMenuItems(id); len(got) != 0 {
					t.Errorf("group %q, no grant: %s has %q", g, id, got)
				}
			}
		})
	}
}

// agentsRowMenuEnv is channelAgentsEnv under agents_in_channels_improvements
// with agent_channel_header in group, the viewer holding
// REMOVE_FROM_CHANNEL in #agents-lab when grant is set.
func agentsRowMenuEnv(t *testing.T, group string, grant bool) (*perfEnv, *fakeslack.Server) {
	t.Helper()
	return agentsRowMenuEnvOpts(t, group, grant, true)
}

// agentsRowMenuEnvOpts is agentsRowMenuEnv; shots gives it the headless
// window PREVIEW_SHOTS_DIR asks for (one per test: two EGL windows in a
// process take each other down).
func agentsRowMenuEnvOpts(t *testing.T, group string, grant, shots bool) (*perfEnv, *fakeslack.Server) {
	t.Helper()
	sz := fakeslack.Small
	sz.ChannelAgents = true
	fs, err := fakeslack.Start(sz)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	if grant {
		fs.Permissions = map[string][]string{
			"":                        {"CAN_AT_CHANNEL", "CAN_AT_EVERYONE", "UPDATE_NAME", "UPDATE_DISPLAY_NAME", "EDIT_PROFILE_PHOTO"},
			fakeslack.ChannelAgentsID: {"CAN_AT_CHANNEL", "CAN_AT_EVERYONE", "MANAGE_CHANNEL_TABS", core.PermRemoveFromChannel},
		}
	}
	e := startEnv(t, fs, t.TempDir())
	if dir := os.Getenv("PREVIEW_SHOTS_DIR"); shots && dir != "" && e.gpu == nil {
		w, err := pixeltest.NewWindow(e.size.X, e.size.Y)
		if err != nil {
			t.Fatalf("headless window: %v", err)
		}
		e.gpu = w
	}
	e.waitReady()
	e.a.Core().SetExperimentGroup(core.ExpAgentsInChannelsImprovements, "on")
	e.a.Core().SetExperimentGroup(core.ExpAgentChannelHeader, group)
	return e, fs
}

// agentsRowMenuTab opens ch's details on the reworked Agents & apps tab
// and waits for its roster and, with the grant, the rows' menus.
func agentsRowMenuTab(t *testing.T, e *perfEnv, ch string) *views.ChannelDetails {
	t.Helper()
	d := openAgentsDetails(t, e, ch)
	d.ShowTab("Agents & apps")
	e.until("the agents roster", 10*time.Second, func() bool { e.frame(); return d.AgentsRosterLoaded() })
	// the grants answer after the roster (no grant: the wait runs out)
	e.untilSoft(5*time.Second, func() bool {
		e.frame()
		return e.a.Core().ChannelGrant(ch, core.PermRemoveFromChannel)
	})
	e.settle(300 * time.Millisecond)
	return d
}
