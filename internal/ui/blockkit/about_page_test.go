package blockkit

// An app DM's About tab (apphome.go): BaseAppSpace (2Fle) draws Slack's
// About page with agent_about_page on (the account's group) and the
// older layout in control or unassigned. Fake Slack only (Size.Gallery's
// Deploy Bot DM).

import (
	"context"
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/some-du6e/clack/internal/core"
	"github.com/some-du6e/clack/internal/fakeslack"
	"github.com/some-du6e/clack/internal/imgcache"
	"github.com/some-du6e/clack/internal/slack"
	"github.com/some-du6e/clack/internal/store"
	"github.com/some-du6e/clack/internal/ui/theme"
)

func TestAppDMAboutPageGate(t *testing.T) {
	sz := fakeslack.Small
	sz.Gallery = true
	fs, err := fakeslack.Start(sz)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fs.Close() })
	a := core.New(slack.New(fs.Creds()), store.New(t.TempDir()))
	t.Cleanup(a.Shutdown)
	if err := a.Boot(context.Background()); err != nil {
		t.Fatal(err)
	}
	h := &bkHost{a: a, th: theme.New(false)}
	h.img = imgcache.New(func(context.Context, string) ([]byte, error) { return nil, context.Canceled }, "", nil)
	e := &wfEnv{t: t, h: h, fs: fs, a: a}

	var dm *AppDM
	e.until("the app DM", func() bool { dm = AppDMFor(h, fakeslack.AppDMID); return dm != nil })
	dm.SetTab("about")
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Now: time.Now(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(1200, 800))}
		dm.LayoutBody(gtx)
	}

	// the account's group: the About page, for the DM's app
	if g := a.Experiment(core.ExpAgentAboutPage); g != "on" {
		t.Fatalf("agent_about_page %q, want the account's on", g)
	}
	p := dm.AboutPage()
	if p == nil || p.AppID() != fakeslack.AppAppID {
		t.Fatalf("About page %v", p)
	}
	e.until("apps.about.info", func() bool { frame(); return p.Model().Loaded })
	if m := p.Model(); m.AppName != "Deploy Bot" || len(m.Security) == 0 {
		t.Errorf("model %+v", m)
	}
	if dm.about != nil {
		t.Error("the older layout's apps.profile.get ran for the About page")
	}

	// control and unassigned: BaseAppSpace's older layout
	for _, g := range []string{"control", ""} {
		a.SetExperimentGroup(core.ExpAgentAboutPage, g)
		if dm.AboutPage() != nil {
			t.Errorf("agent_about_page %q: the About page", g)
		}
		e.until("the older About", func() bool { frame(); return dm.about != nil })
	}
	a.ClearExperimentGroup(core.ExpAgentAboutPage)
	if dm.AboutPage() != p {
		t.Error("the page wasn't kept across the experiment's change")
	}
}
