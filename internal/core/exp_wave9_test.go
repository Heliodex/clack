package core

import (
	"context"
	"net/url"
	"slices"
	"sync"
	"testing"

	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
)

// flexpane_content_parity is HLQ9 X7: "on" only. The account is "on"
// (experiments.getByUser, 2026-11-15), so are core's defaults; control and
// an unassigned experiment are off.
func TestFlexpaneContentParityGate(t *testing.T) {
	a := New(slack.New(slack.Creds{}), store.New(""))
	if !a.FlexpaneContentParity() {
		t.Error("the account's group (on, the default) reads off")
	}
	for _, g := range []string{"control", "off", ""} {
		a.SetExperimentGroup(ExpFlexpaneContentParity, g)
		if a.FlexpaneContentParity() {
			t.Errorf("group %q reads on", g)
		}
	}
	a.SetExperimentGroup(ExpFlexpaneContentParity, "on")
	if !a.FlexpaneContentParity() {
		t.Error("on reads off")
	}
	i := slices.IndexFunc(ExperimentReads(), func(r ExperimentRead) bool { return r.Name == ExpFlexpaneContentParity })
	if i < 0 || !slices.Contains(ExperimentReads()[i].Groups, "on") {
		t.Errorf("flexpane_content_parity isn't in experimentReads with on")
	}
}

// bD9l canInviteAppToChannel over the viewer's conversations: public and
// private channels they're in and can post in, by name; not archived,
// read-only or thread-only ones, DMs or group DMs; a Slack Connect
// channel only for a bot that's neither Agentforce nor Sidekick.
func TestAppInvitableChannels(t *testing.T) {
	a := New(slack.New(slack.Creds{}), store.New(""))
	yes, no := true, false
	a.S.PutUsers([]slack.User{
		{ID: "UAPP", Name: "deploybot", IsBot: true},
		{ID: "UAF", Name: "agentforce", IsBot: true, IsAgentforceBot: true},
		{ID: "USK", Name: "sidekick", IsBot: true, IsSidekickBot: true},
	})
	for _, c := range []slack.Conversation{
		{ID: "C0ZED", Name: "zed", IsChannel: true, IsMember: true},
		{ID: "C0ALPHA", Name: "Alpha", IsChannel: true, IsMember: true},
		{ID: "G0PRIV", Name: "plans", IsGroup: true, IsPrivate: true, IsMember: true},
		{ID: "C0ARCH", Name: "old", IsChannel: true, IsMember: true, IsArchived: true},
		{ID: "C0RO", Name: "announce", IsChannel: true, IsMember: true, IsReadOnly: &yes, IsThreadOnly: &no},
		{ID: "C0TO", Name: "threads", IsChannel: true, IsMember: true, IsThreadOnly: &yes},
		{ID: "C0EXT", Name: "partners", IsChannel: true, IsMember: true, IsExtShared: true},
		{ID: "D0APP", IsIM: true, User: "UAPP"},
		{ID: "G0MPIM", Name: "mpdm-a--b-1", IsMPIM: true, IsMember: true},
	} {
		a.S.PutConv(c)
	}
	ids := func(cs []Conv) []string {
		var out []string
		for _, c := range cs {
			out = append(out, c.ID)
		}
		return out
	}
	if got, want := ids(a.AppInvitableChannels("UAPP")), []string{"C0ALPHA", "C0EXT", "G0PRIV", "C0ZED"}; !slices.Equal(got, want) {
		t.Errorf("app: %v, want %v", got, want)
	}
	for _, bot := range []string{"UAF", "USK"} {
		if got, want := ids(a.AppInvitableChannels(bot)), []string{"C0ALPHA", "G0PRIV", "C0ZED"}; !slices.Equal(got, want) {
			t.Errorf("%s: %v, want %v (no Slack Connect channel)", bot, got, want)
		}
	}
	if !a.HasAppInvitableChannel("UAPP") || !a.CanInviteAppTo("C0ZED", "UAPP") {
		t.Error("U3 / Vg false for a channel the app can join")
	}
	for _, ch := range []string{"C0ARCH", "C0RO", "C0TO", "D0APP", "G0MPIM", "C0NOWHERE"} {
		if a.CanInviteAppTo(ch, "UAPP") {
			t.Errorf("Vg true for %s", ch)
		}
	}
	b := New(slack.New(slack.Creds{}), store.New(""))
	b.S.PutConv(slack.Conversation{ID: "C0ARCH", Name: "old", IsChannel: true, IsMember: true, IsArchived: true})
	if b.HasAppInvitableChannel("UAPP") {
		t.Error("U3 true with only an archived channel")
	}
}

// AddAgentToChannelsModal's submit on the fake: one conversations.invite
// per channel (users = the bot user), then aggregateInviteResults: added,
// already a member (already_in_channel, cant_invite_self, read from
// errors[0].error first), failures by gWR0 E's reasons and "other".
func TestAddMemberToChannels(t *testing.T) {
	fs, err := fakeslack.Start(fakeslack.Small)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	var mu sync.Mutex
	var invites []url.Values
	fail := map[string]string{
		"C0ALREADY": "already_in_channel",
		"C0SELF":    "cant_invite_self",
		"C0NOTWS":   "org_user_not_in_team",
		"C0APP":     "cant_invite_app_user",
		"C0MANAGED": "cant_invite_to_managed_channel",
		"C0WEIRD":   "channel_not_found",
	}
	fs.SetHook(func(method string, f url.Values) (any, bool) {
		if method != "conversations.invite" {
			return nil, false
		}
		mu.Lock()
		invites = append(invites, f)
		mu.Unlock()
		ch := f.Get("channel")
		if code, ok := fail[ch]; ok {
			if ch == "C0APP" { // the per-user error wins over the top-level one
				return map[string]any{"ok": false, "error": "cant_invite", "errors": []map[string]any{{"user": f.Get("users"), "ok": false, "error": code}}}, true
			}
			return map[string]any{"ok": false, "error": code}, true
		}
		return map[string]any{"ok": true, "channel": map[string]any{"id": ch}}, true
	})
	a := New(slack.New(fs.Creds()), store.New(""))
	chans := []string{"C0NEW1", "C0ALREADY", "C0NOTWS", "C0SELF", "C0APP", "C0NEW2", "C0MANAGED", "C0WEIRD"}
	r := a.AddMemberToChannels(context.Background(), fakeslack.AppUserID, chans)
	if !slices.Equal(r.NewlyAdded, []string{"C0NEW1", "C0NEW2"}) {
		t.Errorf("newly added %v", r.NewlyAdded)
	}
	if !slices.Equal(r.AlreadyMember, []string{"C0ALREADY", "C0SELF"}) {
		t.Errorf("already %v", r.AlreadyMember)
	}
	want := []AddToChannelFailure{
		{"C0NOTWS", AddToChannelNotInWorkspace}, {"C0APP", AddToChannelNotAllowed},
		{"C0MANAGED", AddToChannelNoPermission}, {"C0WEIRD", AddToChannelOther},
	}
	if !slices.Equal(r.Failed, want) {
		t.Errorf("failed %v, want %v", r.Failed, want)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(invites) != len(chans) {
		t.Fatalf("%d invites for %d channels", len(invites), len(chans))
	}
	for _, f := range invites {
		if f.Get("users") != fakeslack.AppUserID {
			t.Errorf("invite users %q", f.Get("users"))
		}
	}
}
