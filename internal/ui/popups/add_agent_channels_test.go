package popups

import (
	"encoding/json"
	"fmt"
	"image"
	"net/url"
	"slices"
	"strings"
	"testing"

	"gioui.org/layout"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/slack"
)

// aacEnv is pfEnv with the viewer in three channels (#launches, where
// the app isn't, #builds and the private #plans) and the app's DM, the
// fake answering channels/membership and conversations.invite (invite
// errors from fail), and CurrentView on #launches.
func aacEnv(t *testing.T, fail map[string]string) (*tzHost, *pfCalls) {
	t.Helper()
	h, fs := pfEnv(t)
	for _, c := range []slack.Conversation{
		{ID: "C0LAUNCH", Name: "launches", IsChannel: true, IsMember: true},
		{ID: "C0BUILDS", Name: "builds", IsChannel: true, IsMember: true},
		{ID: "G0PLANS", Name: "plans", IsGroup: true, IsPrivate: true, IsMember: true},
		{ID: "D0APPDM", IsIM: true, User: fakeslack.AppUserID},
	} {
		h.a.S.PutConv(c)
	}
	calls := &pfCalls{}
	fs.SetEdgeHook(func(method string, body map[string]json.RawMessage) (any, bool) {
		if !strings.HasSuffix(method, "channels/membership") {
			return nil, false
		}
		var ch string
		_ = json.Unmarshal(body["channel"], &ch)
		calls.add("channels/membership", url.Values{"channel": {ch}})
		if ch == "C0BUILDS" {
			return map[string]any{"ok": true, "members": []string{fakeslack.AppUserID}}, true
		}
		return map[string]any{"ok": true, "non_members": []string{fakeslack.AppUserID}}, true
	})
	fs.SetHook(func(method string, f url.Values) (any, bool) {
		if method != "conversations.invite" {
			return nil, false
		}
		calls.add(method, f)
		if code, ok := fail[f.Get("channel")]; ok {
			return map[string]any{"ok": false, "error": code}, true
		}
		return map[string]any{"ok": true, "channel": map[string]any{"id": f.Get("channel")}}, true
	})
	aacView(t, "C0LAUNCH", "")
	return h, calls
}

// aacView points CurrentView at ch (an app DM's tab).
func aacView(t *testing.T, ch, tab string) {
	prev := CurrentView
	t.Cleanup(func() { CurrentView = prev })
	CurrentView = func() (string, string) { return ch, tab }
}

// aacDetails opens App details for the fake's app and waits for its card
// and the current channel's membership.
func aacDetails(t *testing.T, h *tzHost) (*AppDetailsView, core.AppCard) {
	t.Helper()
	v := NewAppDetailsView(h, fakeslack.AppUserID, "")
	draw := func() []string {
		return pfFrame(t, h.th, image.Pt(420, 760), "", func(gtx layout.Context) { v.Layout(gtx) })
	}
	draw()
	pfUntil(t, "the app card", func() bool { _, ok := v.Card(); return ok })
	draw()
	c, _ := v.Card()
	pfUntil(t, "the membership", func() bool { return v.facts(c).membership != adMemberUnknown })
	return v, c
}

func aacLabels(items []MenuItem) []string {
	var out []string
	for _, it := range items {
		if !it.Separator {
			out = append(out, it.Label)
		}
	}
	return out
}

func aacName(h *tzHost) string {
	u, _ := lookupUser(h, fakeslack.AppUserID)
	return pfMemberName(u)
}

// flexpane_content_parity on (the account's group): App details opened
// from a channel the app isn't in has getAppPrimaryActions' Add to
// Channel as its main button, a skeleton while the membership is asked;
// the overflow has Message and no Add to channel; the button opens
// AddAgentToChannelsModal with the channel picked. From a channel the app
// is in, the button is Message and the overflow's Add to channel opens
// the modal with nothing picked.
func TestAppDetailsAddToChannelLikeSlack(t *testing.T) {
	h, calls := aacEnv(t, nil)
	v := NewAppDetailsView(h, fakeslack.AppUserID, "")
	draw := func(shot string) []string {
		return pfFrame(t, h.th, image.Pt(420, 760), shot, func(gtx layout.Context) { v.Layout(gtx) })
	}
	draw("")
	pfUntil(t, "the app card", func() bool { _, ok := v.Card(); return ok })
	x := draw("")
	if c, _ := v.Card(); v.memCh == "" && adPrimary(v.facts(c)) != adMainLoading {
		t.Errorf("membership unanswered: main %d, want the skeleton", adPrimary(v.facts(c)))
	}
	if v.memCh == "" && (hasText(x, "Message") || hasText(x, "Add to Channel")) {
		t.Errorf("a label over the skeleton: %q", x)
	}
	pfUntil(t, "the membership", func() bool { return v.memCh != "" })
	x = draw("app-details-add-to-channel")
	if !hasText(x, "Add to Channel") || hasText(x, "Message") {
		t.Fatalf("main button: %q", x)
	}
	if got := calls.get("channels/membership"); len(got) != 1 || got[0].Get("channel") != "C0LAUNCH" {
		t.Errorf("membership asks: %v", got)
	}
	c, _ := v.Card()
	f := v.facts(c)
	if !f.parity || !f.canAdd || !f.canInviteCurrent || f.membership != adMemberNonMember || f.prefill() != "C0LAUNCH" {
		t.Errorf("facts %+v", f)
	}
	labels := aacLabels(v.overflow(c))
	if !slices.Contains(labels, "Message") || slices.Contains(labels, "Add to channel") {
		t.Errorf("overflow %q: want Message, no Add to channel", labels)
	}
	n := h.shownCount()
	v.mainAction(c)
	if h.shownCount() != n+1 {
		t.Fatal("Add to Channel opened nothing")
	}
	h.mu.Lock()
	m, ok := h.shown[len(h.shown)-1].(*Modal)
	h.mu.Unlock()
	if want := "Add " + aacName(h) + " to channels"; !ok || m.Title() != want {
		t.Fatalf("opened %T, want the %q modal", h.shown[len(h.shown)-1], want)
	}

	// a channel the app is in: Message; the overflow's item has no prefill
	aacView(t, "C0BUILDS", "")
	v2, c2 := aacDetails(t, h)
	if main := adPrimary(v2.facts(c2)); main != adMainMessage {
		t.Errorf("app in the channel: main %d, want Message", main)
	}
	labels = aacLabels(v2.overflow(c2))
	if !slices.Contains(labels, "Add to channel") || slices.Contains(labels, "Message") {
		t.Errorf("overflow %q: want Add to channel, no Message", labels)
	}
	if p := v2.facts(c2).prefill(); p != "" {
		t.Errorf("prefill %q for a channel the app is in", p)
	}

	// the app's own DM on its Messages tab: offersMessage drops, so the
	// button is Add to Channel; on Home it's Message again
	aacView(t, "D0APPDM", "messages")
	v3, c3 := aacDetails(t, h)
	if f := v3.facts(c3); !f.ownDM || f.offersMessage || adPrimary(f) != adMainAddToChannel {
		t.Errorf("own DM, Messages tab: %+v main %d", f, adPrimary(f))
	}
	aacView(t, "D0APPDM", "home")
	if f := v3.facts(c3); f.offersAppHome || adPrimary(f) != adMainMessage {
		t.Errorf("own DM, Home tab: %+v main %d", f, adPrimary(f))
	}
}

// AddAgentToChannelsModal: the options (bD9l dX, by name), the prefill,
// Add's invites, the toast and the close; a failure keeps the modal with
// buildFailureMessage and only the failed channel picked.
func TestAddAgentToChannelsModal(t *testing.T) {
	h, calls := aacEnv(t, map[string]string{"C0BUILDS": "already_in_channel", "G0PLANS": "cant_invite_app_user"})
	name := aacName(h)
	d := OpenAddAgentToChannels(h, fakeslack.AppUserID, "C0LAUNCH")
	var opts []string
	for _, o := range d.Options() {
		opts = append(opts, o.ID)
	}
	if !slices.Equal(opts, []string{"C0BUILDS", "C0LAUNCH", "G0PLANS"}) {
		t.Errorf("options %v", opts)
	}
	if !slices.Equal(d.Picked(), []string{"C0LAUNCH"}) {
		t.Errorf("prefill %v", d.Picked())
	}
	x := pfFrame(t, h.th, image.Pt(1000, 700), "add-agent-to-channels", func(gtx layout.Context) { d.Modal().Layout(gtx) })
	for _, s := range []string{"Add " + name + " to channels", "Cancel", "Add", "launches"} {
		if !hasText(x, s) {
			t.Errorf("modal: %q not drawn in %q", s, x)
		}
	}
	d.Pick("C0BUILDS")
	d.submit()
	pfUntil(t, "the invites", func() bool { return !d.busy })
	if got := calls.get("conversations.invite"); len(got) != 2 {
		t.Fatalf("invites %v", got)
	}
	if ts := h.toastList(); len(ts) != 1 || ts[0] != "Added "+name+" to 1 channel." {
		t.Errorf("toasts %q", ts)
	}
	if !d.Modal().Dismissed() {
		t.Error("no failures, the modal stayed")
	}

	// a failure: the error, the failed channel left picked
	d = OpenAddAgentToChannels(h, fakeslack.AppUserID, "")
	d.Pick("G0PLANS")
	d.Pick("C0BUILDS")
	d.submit()
	pfUntil(t, "the invites", func() bool { return !d.busy })
	if d.Modal().Dismissed() {
		t.Fatal("a failure closed the modal")
	}
	if want := "Can’t add " + name + " to #plans. " + name + " isn’t allowed in that channel."; d.Error() != want {
		t.Errorf("error %q, want %q", d.Error(), want)
	}
	if !slices.Equal(d.Picked(), []string{"G0PLANS"}) {
		t.Errorf("picked after the failure %v", d.Picked())
	}
	if ts := h.toastList(); len(ts) != 2 || ts[1] != name+" is already in that channel." {
		t.Errorf("toasts %q", ts)
	}
	x = pfFrame(t, h.th, image.Pt(1000, 700), "add-agent-to-channels-error", func(gtx layout.Context) { d.Modal().Layout(gtx) })
	if !hasText(x, d.Error()) && !slices.ContainsFunc(x, func(s string) bool { return strings.Contains(s, "allowed") }) {
		t.Errorf("the error isn't drawn: %q", x)
	}

	// several reasons read as "other"; two channels, "those channels"
	two := []core.AddToChannelFailure{{Channel: "C1", Reason: core.AddToChannelNotInWorkspace}, {Channel: "C2", Reason: core.AddToChannelNotInWorkspace}}
	if got, want := aacFailureMessage(name, two, []string{"#a", "#b"}),
		"Can’t add "+name+" to #a and #b. A Slack admin hasn’t added "+name+" to the workspace for those channels yet."; got != want {
		t.Errorf("not in workspace: %q, want %q", got, want)
	}
	mixed := []core.AddToChannelFailure{{Channel: "C1", Reason: core.AddToChannelNoPermission}, {Channel: "C2", Reason: core.AddToChannelNotAllowed}, {Channel: "C3", Reason: core.AddToChannelNotAllowed}}
	if got, want := aacFailureMessage(name, mixed, []string{"#a", "#b", "#c"}),
		"Can’t add "+name+" to #a, #b, and #c. Try again, or contact a Slack admin."; got != want {
		t.Errorf("mixed: %q, want %q", got, want)
	}
	if got := aacResultLabel(name, core.AddToChannelsResult{NewlyAdded: []string{"C1", "C2", "C3"}}); got != "Added "+name+" to 3 channels." {
		t.Errorf("result %q", got)
	}
}

// Ten channels at a time: the eleventh pick is refused and the hint shows.
func TestAddAgentToChannelsCap(t *testing.T) {
	h, _ := aacEnv(t, nil)
	for i := 0; i < 11; i++ {
		h.a.S.PutConv(slack.Conversation{ID: fmt.Sprintf("C0MORE%04d", i), Name: fmt.Sprintf("more-%02d", i), IsChannel: true, IsMember: true})
	}
	d := OpenAddAgentToChannels(h, fakeslack.AppUserID, "")
	hint := "You can add " + aacName(h) + " to up to 10 channels at a time."
	for i := 0; i < 10; i++ {
		if !d.Pick(fmt.Sprintf("C0MORE%04d", i)) {
			t.Fatalf("pick %d refused", i+1)
		}
		if i == 8 {
			x := pfFrame(t, h.th, image.Pt(1000, 900), "", func(gtx layout.Context) { d.Modal().Layout(gtx) })
			if hasText(x, hint) {
				t.Error("hint at nine")
			}
		}
	}
	if d.Pick("C0MORE0010") || len(d.Picked()) != 10 {
		t.Errorf("eleventh pick taken: %v", d.Picked())
	}
	x := pfFrame(t, h.th, image.Pt(1000, 900), "add-agent-to-channels-cap", func(gtx layout.Context) { d.Modal().Layout(gtx) })
	if !hasText(x, hint) {
		t.Errorf("no hint at ten: %q", x)
	}
}

// Control and unassigned (Slack's null): no primary actions, so the
// button is Message, never Add to Channel, and the overflow's Add to
// channel is the single-channel dialog (openInviteMemberToChannelDialog).
func TestAppDetailsAddToChannelOff(t *testing.T) {
	for _, g := range []string{"control", ""} {
		t.Run("group="+g, func(t *testing.T) {
			h, _ := aacEnv(t, nil)
			h.a.SetExperimentGroup(core.ExpFlexpaneContentParity, g)
			v, c := aacDetails(t, h)
			x := pfFrame(t, h.th, image.Pt(420, 760), "", func(gtx layout.Context) { v.Layout(gtx) })
			if !hasText(x, "Message") || hasText(x, "Add to Channel") {
				t.Errorf("main button: %q", x)
			}
			if f := v.facts(c); f.parity || adPrimary(f) != adMainMessage {
				t.Errorf("facts %+v main %d", f, adPrimary(f))
			}
			var add *MenuItem
			items := v.overflow(c)
			for i := range items {
				if items[i].Label == "Add to channel" {
					add = &items[i]
				}
			}
			if add == nil {
				t.Fatalf("no Add to channel in %q", aacLabels(items))
			}
			add.Do()
			h.mu.Lock()
			m, ok := h.shown[len(h.shown)-1].(*Modal)
			h.mu.Unlock()
			if want := "Add " + aacName(h) + " to a channel"; !ok || m.Title() != want {
				t.Errorf("Add to channel opened %T, want %q", h.shown[len(h.shown)-1], want)
			}
		})
	}
}
