package fakeslack

import "strconv"

// AppAboutDesc is Deploy Bot's long_description in apps.about.info.
const AppAboutDesc = "Deploy Bot builds a branch when you ask it to and posts the release notes in the channel once the deploy is done."

// appAboutInfo is apps.about.info for Deploy Bot (AppAppID), in the shape
// the real method sends (keys checked against the live API, 2026-11-15;
// the content is synthetic): an internal app (not directory approved),
// one slash command, scope_info with required content and actions and an
// optional scope, security_compliance with nothing filled in but the
// developer, and token rotation off.
func (s *Server) appAboutInfo(app string) (map[string]any, bool) {
	if app != AppAppID {
		return nil, false
	}
	icons := map[string]any{}
	for _, px := range []int{32, 36, 48, 64, 72, 96, 128, 192, 512, 1024} {
		icons["image_"+strconv.Itoa(px)] = s.URL + "img/" + strconv.Itoa(px) + "x" + strconv.Itoa(px) + "/app.png"
	}
	scope := func(sc, desc, token string, optional bool) map[string]any {
		return map[string]any{"scopes": []any{sc}, "description": desc, "token_type": token, "is_optional": optional, "is_approved": false}
	}
	sec := map[string]any{}
	for _, k := range []string{"company_location", "tos_url", "data_retention_policy", "data_archival_removal_policy",
		"data_storage_policy", "data_how_host", "data_host_company", "cert_comply_sub_processors", "use_llm", "llm_desc",
		"security_email", "security_what_sso", "security_support_saml", "security_vulnerability_program"} {
		sec[k] = nil
	}
	sec["company_developer_name"] = "Fake Club Tools"
	return map[string]any{
		"ok": true, "app_id": AppAppID, "name": "Deploy Bot", "description": "Ships builds and posts release notes",
		"long_description": AppAboutDesc, "long_description_formatted": "<p>" + AppAboutDesc + "</p>",
		"assistant_description": nil, "icons": icons, "auth_full_name": nil, "is_ai_app": false, "is_agent_app": false,
		"agentforce": nil, "native_agent": nil, "categories": []any{}, "screenshots": []any{}, "youtube_url": nil,
		"support_url": "https://example.com/deploybot/help", "home_url": "", "privacy_link": nil,
		"supported_languages": nil, "pricing_info": nil, "support_email": nil, "is_directory_approved": false,
		"is_slack_integration": false, "is_custom_integration": false, "is_distributed": false, "is_published": false,
		"is_installed": true, "is_active": true, "home_workspace_id": TeamID, "home_enterprise_id": EnterpriseID,
		"app_type": nil, "service_id": AppBotID,
		"app_user": map[string]any{"id": AppUserID, "username": "deploybot", "memberships_count": 12},
		"scope_info": map[string]any{
			"actions": map[string]any{"heading": "Perform actions in channels &amp; conversations", "secondary_groups": map[string]any{
				"channel_actions": map[string]any{"heading": "Perform actions in channels", "scopes_info": []any{
					scope("chat:write", "Send messages as @deploybot", "bot", false),
					scope("reactions:write", "Add and edit emoji reactions", "bot", true)}},
			}},
			"content": map[string]any{"heading": "Content and info about channels &amp; conversations", "secondary_groups": map[string]any{
				"channel_content": map[string]any{"heading": "View messages and other content in public channels", "scopes_info": []any{
					scope("channels:history", "View messages and other content in public channels that Deploy Bot has been added to", "bot", false)}},
				"user_content": map[string]any{"heading": "Content and info about you", "scopes_info": []any{
					scope("users:read", "View people in a workspace", "user", false)}},
			}},
		},
		"has_user_consentable_scopes": false, "is_calls_app": false, "is_org_ready_app": false,
		"sales_home_workflow_app_type": 0, "is_workflow_app": false,
		"shortcuts":                       map[string]any{"message_shortcuts": []any{}, "global_shortcuts": []any{}},
		"mcp_servers":                     nil,
		"slash_commands":                  []any{map[string]any{"name": "/deploy", "description": "Deploy a branch", "usage_hint": "[branch]"}},
		"security_token_rotation_enabled": false, "security_compliance": sec, "is_shown_as_agent": false,
	}, true
}
