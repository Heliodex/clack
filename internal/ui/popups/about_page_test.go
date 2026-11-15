package popups

// The About page (about_page.go) over fakeslack's Deploy Bot with the
// account's groups (agent_about_page on): what about_page_AboutPage
// draws for an internal app, the stacked layout at 544px, an open
// accordion, and AppNotFound when apps.about.info fails.

import (
	"image"
	"slices"
	"testing"

	"gioui.org/layout"
	"gioui.org/unit"

	"github.com/some-du6e/clack/internal/fakeslack"
)

func abPage(t *testing.T, e *srEnv, w, h int) (*AboutPage, *srDrv) {
	t.Helper()
	p := NewAboutPage(e.h, fakeslack.AppAppID, fakeslack.AppUserID)
	d := &srDrv{size: image.Pt(w, h), w: p}
	d.until(t, "apps.about.info and the app profile", func() bool { return p.info != nil && p.card != nil && !p.loading })
	return p, d
}

func TestAboutPageLikeSlack(t *testing.T) {
	e := srNewEnv(t, false)
	if !e.h.a.AgentAboutPage() {
		t.Fatal("the account's agent_about_page group doesn't give the About page")
	}
	_, d := abPage(t, e, 1280, 2400)
	x := d.scan()
	for _, want := range []string{
		// the description, headed by the app's name
		"Deploy Bot",
		// FeaturesSection
		"Features", "Slash commands", "Deploy Bot has 1 slash command:", "/deploy [branch]", "Deploy a branch",
		// AboutPagePermissions
		"Configurations", "Required authorizations", "Optional authorizations",
		"Deploy Bot will be able to view", "Deploy Bot will be able to do",
		"Perform actions in channels", "View messages and other content in public channels",
		// AboutPageSecurityCompliance
		"Security & compliance", "General", "Security", "Scopes",
		"App has 1 user token scopes", "App has 3 bot token scopes",
		// the sidebar
		"Add to VIP", "Configuration", "Mention", "@deploybot", "Learn more & support", "Get app support",
	} {
		if !srHas(x, want) {
			t.Errorf("not drawn: %q", want)
		}
	}
	for _, not := range []string{
		"Report this app",       // an internal app isn't in Slack Marketplace
		"Categories", "Pricing", // none
		"Supported languages", "Visit developer website", "Privacy policy",
		"This app has no description.", "Developer", // the accordions start closed
		"App not found", "Message shortcuts", "Shortcuts",
	} {
		if srHas(x, not) {
			t.Errorf("drawn: %q", not)
		}
	}
}

func TestAboutPageAccordions(t *testing.T) {
	e := srNewEnv(t, false)
	p, d := abPage(t, e, 1280, 2400)
	p.open["sec:General"] = true
	p.open["perm:req:actions:channelActions"] = true
	p.open["scopes:workspace"] = true
	x := d.scan()
	for _, want := range []string{"Developer", "Fake Club Tools", "Send messages as @deploybot", "•"} {
		if !srHas(x, want) {
			t.Errorf("open accordions: %q not drawn", want)
		}
	}
	if !srHasPrefix(x, "chat:write, reactions:write, channels:history") {
		t.Errorf("the bot scopes aren't listed: %q", x)
	}
	// the optional scope stays in its own (closed) block
	if srHas(x, "Add and edit emoji reactions") {
		t.Error("the optional block opened with the required one")
	}
}

func TestAboutPageColumns(t *testing.T) {
	gtx := layout.Context{Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}}
	for _, c := range []struct{ inner, main, side int }{
		{1200, 817, 351}, // 1168 split 7:3
		{2000, 968, 360}, // both at their max
		{600, 312, 256},  // the sidebar at its min
		{1400, 968, 360}, // the sidebar at its max, then the main column at its
	} {
		if m, s := aboutColumns(gtx, c.inner); m != c.main || s != c.side {
			t.Errorf("aboutColumns(%d) = %d, %d; want %d, %d", c.inner, m, s, c.main, c.side)
		}
	}
}

// TestAboutPageStacked: at 544px of content or less the sidebar comes
// first, then the rule, then the main column.
func TestAboutPageStacked(t *testing.T) {
	e := srNewEnv(t, false)
	_, d := abPage(t, e, 600, 3000)
	x := d.scan()
	vip, features := slices.Index(x, "Add to VIP"), slices.Index(x, "Features")
	if vip < 0 || features < 0 || vip > features {
		t.Errorf("stacked order: Add to VIP at %d, Features at %d", vip, features)
	}
	_, d = abPage(t, e, 1280, 2400)
	x = d.scan()
	if vip, features := slices.Index(x, "Add to VIP"), slices.Index(x, "Features"); vip < features {
		t.Errorf("wide order: Add to VIP at %d before Features at %d (the main column draws first)", vip, features)
	}
}

func TestAboutPageNotFound(t *testing.T) {
	e := srNewEnv(t, false)
	p := NewAboutPage(e.h, "A0NOSUCHAPP", "")
	d := &srDrv{size: image.Pt(1000, 700), w: p}
	d.until(t, "the failed fetch", func() bool { return p.asked && !p.loading })
	x := d.scan()
	if !srHas(x, "App not found") || !srHas(x, "The app may have been uninstalled or you do not have permission to view it.") {
		t.Errorf("AppNotFound: %q", x)
	}
}
