package messages

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/taut"
	"github.com/some-du6e/clack/internal/ui/slackc"
	"github.com/some-du6e/clack/internal/ui/theme"
)

// Plugins that credit a bot's post to the person behind it (Taut and
// Slick's ShowRealUser) hand MessageWrapper a msg whose user is that
// person and whose bot fields are gone (messages.modifyMessageObject
// with sentBy). Slack's sender is then that member's: their name and
// picture, never the relay's username or icons. Slack Extra's spoilers
// are such posts: bot B09QQ24JRL1 posts with the poster's name and a copy
// of their picture as username and icons, and names them in
// metadata.event_payload.poster (live shape, 2026-11-14). Synthetic ids,
// text and URLs.

const (
	relayBotID = "B09QQ24JRL1" // Slack Extra's bot, in ShowRealUser's relay list
	relayGhost = "U0POSTER0001"
	relayCh    = "C0RELAYED1"
	tsRelayed  = "1700000600.000100"
	tsGhostly  = "1700001200.000100" // ten minutes on: its own group
	ghostPic   = "https://avatars.invalid/ghosty-72.png"
)

// realUserPlugin is ShowRealUser's MessageWrapper patch for Slack
// Extra's bot alone (its other parts and bots don't matter here).
const realUserPlugin = `
class RealUserTest extends TautPlugin {
  static id = 'RealUserTest'
  static pluginName = 'Real user (test)'
  static description = 'test: credits relay posts to the person their metadata names, as ShowRealUser does'
  static authors = ['clack']
  static category = 'messages'
  static defaultConfig = { enabled: true }
  start() {
    const fixed = (msg) => {
      const poster = msg?.bot_id === '` + relayBotID + `' ? msg.metadata?.event_payload?.poster : undefined
      return typeof poster === 'string' ? this.api.messages.modifyMessageObject(msg, { sentBy: poster }) : msg
    }
    this.api.patchComponent('MessageWrapper', (Original) => (props) => {
      const msg = React.useMemo(() => fixed(props.msg), [props.msg])
      return React.createElement(Original, { ...props, msg })
    })
  }
}
return RealUserTest`

// relayedPost is a Slack Extra spoiler as history returns it.
func relayedPost(ts, poster, username, icon string) slack.Message {
	meta, _ := json.Marshal(map[string]any{"event_type": "spoiler", "event_payload": map[string]any{"text": "synthetic", "poster": poster}})
	return slack.Message{Type: "message", TS: ts, Subtype: "bot_message", BotID: relayBotID, Username: username,
		Icons: &slack.Icons{Image48: icon}, Metadata: meta, Text: "a relayed synthetic post"}
}

// The inline patch (ci): the known poster's picture at once, the
// no-image square for one core hasn't loaded, then their picture.
func TestPluginRealUserAvatar(t *testing.T) {
	app, _, tt, _, h := pluginEnv(t)
	checkRealUserAvatar(t, app, tt, h, pluginJS(realUserPlugin))
}

// TestPluginShowRealUser is the same with Taut's own ShowRealUser (the
// messages-taut workflow builds it): its relay list, its isHackClub
// gate (the test core is Hack Club's team) and its messages slice patch
// on top of the component's.
func TestPluginShowRealUser(t *testing.T) {
	code := pluginFile(t, "ShowRealUser")
	app, _, tt, _, h := pluginEnv(t)
	checkRealUserAvatar(t, app, tt, h, code)
}

func checkRealUserAvatar(t *testing.T, app *core.App, tt *taut.Taut, h coreHost, code []byte) {
	t.Helper()
	app.S.PutConv(slack.Conversation{ID: relayCh, Name: "relayed", IsChannel: true, IsMember: true, NumMembers: 3})
	app.S.SetLatest(relayCh, []slack.Message{
		relayedPost(tsRelayed, "UALICE", "alice", "https://bot-icons.invalid/alice-48.png"),
		relayedPost(tsGhostly, relayGhost, "ghosty", "https://bot-icons.invalid/ghosty-48.png"),
	}, false)
	install(t, tt, code)
	until(t, 5*time.Second, "MessageWrapper patched", func() {}, func() bool { return slackc.Patched("MessageWrapper") })
	v := NewChannelView(h, relayCh)
	v.Preview = true
	f := frameFn(v)
	until(t, 10*time.Second, "both posts credited to their posters", f, func() bool {
		a, g := v.s.rows[tsRelayed], v.s.rows[tsGhostly]
		return a != nil && g != nil && a.m.User == "UALICE" && g.m.User == relayGhost
	})
	for i := 0; i < 5; i++ {
		f()
	}
	alice, _ := app.User("UALICE")
	a := v.s.rows[tsRelayed]
	if a.avDrawn != alice.Avatar || a.nameShown != "alice" || a.badge != "" {
		t.Errorf("known poster: avatar %q, name %q, badge %q; want alice's picture %q, her name and no APP badge",
			a.avDrawn, a.nameShown, a.badge, alice.Avatar)
	}
	g := v.s.rows[tsGhostly]
	if g.avDrawn != "" || g.senderMiss() != core.MemberUnknown {
		t.Errorf("poster not loaded yet: avatar %q, sender %v; want no picture (Slack's no-image square, not the relay's icon) and the loading bar",
			g.avDrawn, g.senderMiss())
	}
	if c := colorsFor(h.th); avatarFill(g.avDrawn, c) != c.NoImage {
		t.Errorf("poster not loaded yet: the avatar is filled %v, want .c-avatar--no_image's %v", avatarFill(g.avDrawn, c), c.NoImage)
	}
	// users/info answers: the row resolves again on "names"
	app.S.PutUsers([]slack.User{{ID: relayGhost, Name: "ghosty", RealName: "Ghost Example",
		Profile: slack.Profile{DisplayName: "ghosty", Image72: ghostPic}}})
	ApplyEvent(app, core.Event{Type: "names", Users: []string{relayGhost}})
	until(t, 5*time.Second, "the poster's picture after names", f, func() bool {
		g := v.s.rows[tsGhostly]
		return g.avDrawn == ghostPic && g.nameShown == "ghosty" && g.senderMiss() == core.MemberKnown
	})
}

// A picture that is still loading shows the loading tile; no picture at
// all is Slack's .c-avatar--no_image, rgba(var(--sk_foreground_low_solid)).
func TestAvatarNoImageFill(t *testing.T) {
	for _, dark := range []bool{false, true} {
		c := colorsFor(theme.New(dark))
		want := map[bool]uint32{false: 0xdddddd, true: 0x35373b}[dark]
		if got := avatarFill("", c); got != rgb(want) {
			t.Errorf("dark=%v: no picture filled %v, want sk_foreground_low_solid %06x", dark, got, want)
		}
		if got := avatarFill("https://avatars.invalid/x-72.png", c); got != c.Skeleton {
			t.Errorf("dark=%v: a loading picture filled %v, want the loading tile %v", dark, got, c.Skeleton)
		}
	}
}

// Karim's "no pfp on a message sent by Slack Extra" (2026-11-14) was
// Slack Extra's anchor: it re-posts a pinned message with its author's
// own token about half a second after every new message in the channel,
// so the post has the person's user plus the app's bot_id, bot_profile
// and metadata event_type "anchor" (live shape). Slack's sender is the
// member (getMsgSenderEntity: the user wins), and isAdjacentMessage
// (boot-shared Ag38) compares only the user when there is one, so right
// after that person's own message the anchor is grouped under it: no
// avatar and no name, "Pinned by you" over its text. ShowRealUser leaves
// it alone (its relay list has Slack Extra's bot-token bot, not this
// one, and the post already credits the person). After someone else's
// message it starts a group with the member's picture, not the app's
// icon, and no APP badge.
func TestAppAsUserAnchorGroupsLikeSlack(t *testing.T) {
	const ch = "C0ANCHORS1"
	const tsOwn, tsAnchor1, tsOther, tsAnchor2 = "1700000500.000100", "1700000500.700100", "1700001100.000100", "1700001100.600100"
	anchor := func(ts string) slack.Message {
		meta, _ := json.Marshal(map[string]any{"event_type": "anchor", "event_payload": map[string]any{"channel": ch}})
		return slack.Message{Type: "message", TS: ts, User: tSelf, BotID: "B0EXTRA0001", Text: "anchored synthetic text",
			BotProfile: &slack.BotProfile{ID: "B0EXTRA0001", Name: "Slack Extra", AppID: "A0EXTRA0001",
				Icons: slack.Icons{Image48: "https://avatars.invalid/extra-48.png"}},
			Metadata: meta, PinnedTo: []string{ch}, PinnedInfo: &slack.PinnedInfo{Channel: ch, PinnedBy: tSelf, PinnedTS: 1700000000}}
	}
	e := newCtxEnvSetup(t, ch, []slack.Message{
		{Type: "message", TS: tsOwn, User: tSelf, Text: "my own synthetic message"},
		anchor(tsAnchor1),
		{Type: "message", TS: tsOther, User: "UBOB", Text: "someone else's synthetic message"},
		anchor(tsAnchor2),
	}, nil)
	header := map[string]bool{}
	for _, it := range e.v.s.items {
		if it.kind == itMsg {
			header[it.key] = it.header
		}
	}
	if header[tsAnchor1] {
		t.Error("the anchor right after its author's own message starts a group; Slack groups it under that message")
	}
	if !header[tsAnchor2] {
		t.Error("the anchor after someone else's message is grouped; Slack starts a group")
	}
	self, _ := e.h.app.User(tSelf)
	r := e.row(tsAnchor2)
	if r == nil {
		t.Fatal("no row for the second anchor")
	}
	if r.avDrawn != self.Avatar || r.badge != "" || r.nameShown != "self" {
		t.Errorf("anchor after someone else: avatar %q, badge %q, name %q; want the member's picture %q, no badge, their name",
			r.avDrawn, r.badge, r.nameShown, self.Avatar)
	}
	if g := e.row(tsAnchor1); g == nil || !g.m.Pinned || g.m.User != tSelf {
		t.Error("the grouped anchor isn't drawn as its author's pinned message")
	}
}
