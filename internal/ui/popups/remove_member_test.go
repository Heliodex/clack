package popups

import (
	"slices"
	"testing"

	"github.com/some-du6e/clack/internal/core"
)

// RemoveMemberDialog's text (remove-member-dialog LUCM render): the
// private channel lines say "They’ll" under agent_channel_header and
// "This person will" without it; a public channel's line is the same in
// both; RemoveMemberDialogContent for an external member under
// sc_remove_full_access_user.
func TestRemoveMemberText(t *testing.T) {
	const m, ch = "Build Status", "#agents-lab"
	title := "Remove Build Status from #agents-lab?"
	for _, tc := range []struct {
		name string
		f    core.RemoveMemberFacts
		para string
	}{
		{"public on", core.RemoveMemberFacts{AgentHeader: true}, "They’ll still be able to rejoin, or be added to the conversation."},
		{"public off", core.RemoveMemberFacts{}, "They’ll still be able to rejoin, or be added to the conversation."},
		{"private on", core.RemoveMemberFacts{Private: true, AgentHeader: true}, "They’ll no longer have access to the channel and can only rejoin by invitation."},
		{"private off", core.RemoveMemberFacts{Private: true}, "This person will no longer have access to the channel and can only rejoin by invitation."},
		{"record on", core.RemoveMemberFacts{Private: true, SfChannel: true, AgentHeader: true}, "They’ll no longer have access to the channel and may need to request to rejoin later."},
		{"record off", core.RemoveMemberFacts{Private: true, SfChannel: true}, "This person will no longer have access to the channel and may need to request to rejoin later."},
		{"public record", core.RemoveMemberFacts{SfChannel: true}, "They’ll still be able to rejoin, or be added to the conversation."},
		// external without sc_remove_full_access_user: the channel's line
		{"external, sc off", core.RemoveMemberFacts{External: true, Private: true, AgentHeader: true}, "They’ll no longer have access to the channel and can only rejoin by invitation."},
	} {
		c := RemoveMemberText(tc.f, m, ch)
		if c.Title != title || c.Go != "Remove" {
			t.Errorf("%s: title %q, go %q", tc.name, c.Title, c.Go)
		}
		if c.Para != tc.para || c.Header != "" {
			t.Errorf("%s: %q (header %q), want %q", tc.name, c.Para, c.Header, tc.para)
		}
	}
	c := RemoveMemberText(core.RemoveMemberFacts{External: true, ScRemoveFull: true, Private: true, AgentHeader: true}, m, ch)
	if c.Para != "" || c.Header != "Build Status will be removed from #agents-lab." || c.NoteHeader != "Note that:" ||
		!slices.Equal(c.Notes, []string{"They can be re-invited to this channel.",
			"If this channel is public for Build Status's org, and they are still connected, they can rejoin."}) {
		t.Errorf("external member: %+v", c)
	}
}

// renderError's three texts.
func TestRemoveMemberErrors(t *testing.T) {
	for code, want := range map[string]string{
		"restricted_action": "Hmm, looks like you don’t have permission to kick from channels. Please contact a Workspace Admin if Build Status should no longer have access to your workspace.",
		"not_in_channel":    "Build Status is not a member of #lab.",
		"unknown_error":     "Something’s gone wrong, and we couldn’t remove Build Status from #lab. We suspect this is only temporary. Try again in a bit?",
	} {
		if got := removeMemberError(code, "Build Status", "#lab"); got != want {
			t.Errorf("%s: %q, want %q", code, got, want)
		}
	}
}
