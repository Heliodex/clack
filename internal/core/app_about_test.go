package core

// Wave 8 (app_about.go): agent_about_page's gate, apps.about.info's
// store and the About page's model (2Fle ea over rmod wN). Fake Slack
// only.

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"

	"github.com/some-du6e/clack/internal/fakeslack"
)

func TestAgentAboutPageGate(t *testing.T) {
	a, _ := canvasApp(t)
	// the account's group (fakeslack serves it): on
	if g := a.Experiment(ExpAgentAboutPage); g != "on" || !a.AgentAboutPage() {
		t.Fatalf("agent_about_page %q: AgentAboutPage %v, want on and true", g, a.AgentAboutPage())
	}
	for _, g := range []string{"control", "off", ""} {
		a.SetExperimentGroup(ExpAgentAboutPage, g)
		if a.AgentAboutPage() {
			t.Errorf("agent_about_page %q: the About page", g)
		}
	}
	a.ClearExperimentGroup(ExpAgentAboutPage)
	if !a.AgentAboutPage() {
		t.Error("the server's group didn't come back")
	}
}

// TestAppAboutInfoStore: one apps.about.info per app for the session
// (reason fetch-unknown-apps-about-info), concurrent callers share it,
// and a failed fetch is remembered as non-existent.
func TestAppAboutInfoStore(t *testing.T) {
	a, fs := canvasApp(t)
	var calls atomic.Int32
	var reason atomic.Value
	fs.SetHook(func(method string, f url.Values) (any, bool) {
		if method == "apps.about.info" {
			calls.Add(1)
			reason.Store(f.Get("reason"))
		}
		return nil, false
	})
	ctx := context.Background()
	done := make(chan *AppAboutInfo, 3)
	for range 3 {
		go func() {
			info, err := a.AppAboutInfoOf(ctx, fakeslack.AppAppID)
			if err != nil {
				t.Error(err)
			}
			done <- info
		}()
	}
	for range 3 {
		if info := <-done; info == nil || info.Name != "Deploy Bot" {
			t.Fatalf("info %+v", info)
		}
	}
	if _, err := a.AppAboutInfoOf(ctx, fakeslack.AppAppID); err != nil {
		t.Fatal(err)
	}
	if n := calls.Load(); n != 1 {
		t.Errorf("%d apps.about.info calls, want 1", n)
	}
	if r, _ := reason.Load().(string); r != "fetch-unknown-apps-about-info" {
		t.Errorf("reason %q", r)
	}
	info, err := a.AppAboutInfoOf(ctx, "A0NOSUCHAPP")
	if err != nil || info != nil {
		t.Errorf("unknown app: %+v, %v (want non-existent)", info, err)
	}
	if info, ok := a.CachedAppAboutInfo("A0NOSUCHAPP"); !ok || info != nil {
		t.Errorf("unknown app not remembered: %+v %v", info, ok)
	}
	if n := calls.Load(); n != 2 {
		t.Errorf("%d calls after the unknown app, want 2", n)
	}
}

// TestAboutPageModel: fakeslack's Deploy Bot (an internal app) as the
// page's selector maps it with the account's groups.
func TestAboutPageModel(t *testing.T) {
	a, _ := canvasApp(t)
	ctx := context.Background()
	info, err := a.AppAboutInfoOf(ctx, fakeslack.AppAppID)
	if err != nil || info == nil {
		t.Fatal(info, err)
	}
	card, err := a.AppCardOf(ctx, fakeslack.AppUserID, "")
	if err != nil {
		t.Fatal(err)
	}
	m := a.AboutPageOf(info, false, &card)
	if !m.Loaded || m.AppName != "Deploy Bot" || m.Description != fakeslack.AppAboutDesc || !m.HasDescription || m.AIApp {
		t.Errorf("header: %+v", m)
	}
	if !m.InternalApp || m.ShowReportApp {
		t.Errorf("an internal app: internal %v, report %v", m.InternalApp, m.ShowReportApp)
	}
	if !m.HasFeatures || len(m.SlashCommands) != 1 || m.SlashCommands[0] != (AppSlashCommand{Name: "/deploy", Desc: "Deploy a branch", UsageHint: "[branch]"}) {
		t.Errorf("features: %v %+v", m.HasFeatures, m.SlashCommands)
	}
	// undistributed and not native: the permissions show
	if !m.ShowPermissions {
		t.Error("no permissions section for an undistributed app")
	}
	p := m.Permissions
	if p.RequiredContent == nil || p.RequiredActions == nil || p.OptionalActions == nil || p.OptionalContent != nil {
		t.Fatalf("permission blocks %+v", p)
	}
	if h := p.RequiredActions.Heading; h != "Perform actions in channels & conversations" {
		t.Errorf("heading not sanitized: %q", h)
	}
	keys := func(b *AboutPermissionBlock) []string {
		var out []string
		for _, g := range b.Groups {
			out = append(out, g.Key)
		}
		return out
	}
	if k := keys(p.RequiredContent); !slices.Equal(k, []string{"channelContent", "userContent"}) {
		t.Errorf("content groups %v (Slack's _h order)", k)
	}
	if d := p.OptionalActions.Groups[0].Descriptions; !slices.Equal(d, []string{"Add and edit emoji reactions"}) {
		t.Errorf("optional %v", d)
	}
	// security: General has the developer, Security the token rotation
	titles := []string{}
	for _, c := range m.Security {
		titles = append(titles, c.Title)
	}
	if !slices.Equal(titles, []string{"General", "Security"}) {
		t.Fatalf("security categories %v", titles)
	}
	if r := m.Security[0].Rows; len(r) != 1 || r[0] != (AboutDetailRow{Label: "Developer", Value: "Fake Club Tools", Type: "text"}) {
		t.Errorf("General %+v", r)
	}
	if r := m.Security[1].Rows; len(r) != 1 || r[0] != (AboutDetailRow{Label: "Uses token rotation", Value: "No", Type: "text"}) {
		t.Errorf("Security %+v", r)
	}
	if len(m.ScopeGroups) != 2 || m.ScopeGroups[0].Title != "App has 1 user token scopes" || m.ScopeGroups[0].Icon != "message" ||
		m.ScopeGroups[1].Title != "App has 3 bot token scopes" || m.ScopeGroups[1].Icon != "workspace" {
		t.Errorf("scope groups %+v", m.ScopeGroups)
	}
	if !slices.Equal(m.ScopeGroups[1].Scopes, []string{"chat:write", "reactions:write", "channels:history"}) {
		t.Errorf("bot scopes %v (actions first, then content)", m.ScopeGroups[1].Scopes)
	}
	// sidebar
	if m.AppUserID != fakeslack.AppUserID || m.Username != "deploybot" || m.SupportURL == "" || m.HomeURL != "" {
		t.Errorf("sidebar %+v", m)
	}
	if !m.ShowSettingsButton || m.SettingsURL != "https://fakeclub.slack.com/services/"+fakeslack.AppBotID+"?settings=1" {
		t.Errorf("settings %v %q", m.ShowSettingsButton, m.SettingsURL)
	}

	// control and unassigned: the selector drops what the experiment
	// gates (the page itself isn't drawn then: BaseAppSpace's old layout)
	for _, g := range []string{"control", ""} {
		a.SetExperimentGroup(ExpAgentAboutPage, g)
		if off := a.AboutPageOf(info, false, &card); len(off.Security) != 0 || off.ShowPermissions {
			t.Errorf("agent_about_page %q: security %d, permissions %v", g, len(off.Security), off.ShowPermissions)
		}
	}
	a.ClearExperimentGroup(ExpAgentAboutPage)

	// pending and non-existent
	if m := a.AboutPageOf(nil, true, nil); m.Loaded || !m.Loading {
		t.Errorf("pending %+v", m)
	}
	if m := a.AboutPageOf(nil, false, nil); m.Loaded || m.Loading {
		t.Errorf("non-existent %+v", m)
	}
}

// TestAboutPageDetailRows: buildDetailRow's types and the boolean
// localization, the developer's ?? fallback, AI apps' description
// rule, and the directory app's report link.
func TestAboutPageDetailRows(t *testing.T) {
	a, _ := canvasApp(t)
	raw := map[string]any{"app_id": "A0X", "name": "Helper &amp; Co", "is_ai_app": true,
		"long_description": "  ", "assistant_description": "Answers questions",
		"is_directory_approved": true, "is_slack_integration": false, "is_distributed": true,
		"supported_languages": []any{"en-US", "xx-YY"}, "pricing_info": "freemium",
		"categories": []any{map[string]any{"id": "Ln1", "name": "Productivity"}},
		"security_compliance": map[string]any{"company_developer_name": nil, "tos_url": "https://example.com/tos",
			"use_llm": "yes", "security_what_sso": []any{"Okta", "", nil, "Google"}, "company_location": "  "},
		"security_token_rotation_enabled": true}
	b, _ := json.Marshal(raw)
	info := ParseAppAboutInfo(b)
	m := a.AboutPageOf(&info, false, nil)
	if m.AppName != "Helper & Co" || m.Description != "" || !m.HasDescription || !m.AIApp || !m.HasFeatures {
		t.Errorf("AI app: %+v", m)
	}
	if !m.ShowReportApp || m.InternalApp {
		t.Errorf("directory app: report %v internal %v", m.ShowReportApp, m.InternalApp)
	}
	if !slices.Equal(m.Languages, []string{"English (U.S.)", "xx-YY"}) || m.PricingLabel != "Free and paid plans available" {
		t.Errorf("languages %v pricing %q", m.Languages, m.PricingLabel)
	}
	if m.DeveloperName != "Helper & Co" {
		t.Errorf("developer %q (null company_developer_name falls back to the name)", m.DeveloperName)
	}
	rows := map[string]AboutDetailRow{}
	for _, c := range m.Security {
		for _, r := range c.Rows {
			rows[c.Title+"/"+r.Label] = r
		}
	}
	want := map[string]AboutDetailRow{
		"General/Terms of service":              {Label: "Terms of service", Value: "https://example.com/tos", Type: "url"},
		"General/Developer":                     {Label: "Developer", Value: "Helper & Co", Type: "text"},
		"Privacy & data governance/Uses LLM/AI": {Label: "Uses LLM/AI", Value: "Yes", Type: "boolean"},
		"Security/Supports Single Sign On (SSO) with the following providers": {Label: "Supports Single Sign On (SSO) with the following providers", Value: "Okta, Google", Type: "text"},
		"Security/Uses token rotation":                                        {Label: "Uses token rotation", Value: "Yes", Type: "text"},
	}
	for k, w := range want {
		if rows[k] != w {
			t.Errorf("%s: %+v, want %+v", k, rows[k], w)
		}
	}
	if _, ok := rows["General/Company headquarters location"]; ok {
		t.Error("a blank value made a row")
	}
	if len(rows) != len(want) {
		t.Errorf("%d rows, want %d: %v", len(rows), len(want), rows)
	}
}
