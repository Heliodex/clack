package core

import "testing"

// team_joiner_invite never counts toward the All tab (3x6A, build
// c07ab895506f: TeamJoinerInvite sits with the skipped types and nothing
// reads inviter_joiner_first_dm_v2), whatever that experiment's group: the
// test account's control, a forced treatment, or none.
func TestActivityBadgeSkipsTeamJoinerInvite(t *testing.T) {
	a, _ := testApp(t)
	a.setActivityBadges(map[string]int{"at_user": 2, "team_joiner_invite": 5, "message_reaction": 7, "quietly_added_to_channel": 1})
	for _, group := range []string{"control", "treatment", ""} {
		a.SetExperimentGroup("inviter_joiner_first_dm_v2", group)
		c, ok := a.ActivityTabCounts()
		if !ok || c[0] != 3 {
			t.Errorf("inviter_joiner_first_dm_v2 %q: All %d (ok %v), want 3 (at_user + quietly_added, no joiner or reaction)", group, c[0], ok)
		}
		if a.ActivityBadgeCount() != c[0] {
			t.Errorf("inviter_joiner_first_dm_v2 %q: the rail badge %d differs from All %d", group, a.ActivityBadgeCount(), c[0])
		}
	}
	// the gate next to it is unchanged: code_review_requested_in_activity
	a.setActivityBadges(map[string]int{"code_review_requested": 4, "team_joiner_invite": 5})
	a.SetExperimentGroup("code_review_requested_in_activity", "on")
	if c, _ := a.ActivityTabCounts(); c[0] != 4 || c[2] != 4 {
		t.Errorf("code reviews: All %d Mentions %d, want 4 and 4", c[0], c[2])
	}
}
