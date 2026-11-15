package core

import (
	"testing"

	"github.com/some-du6e/clack/internal/emoji"
)

// The standard emoji set follows the account's emoji_v17 group: the
// 16.0 set without it (the defaults, Karim's account), the 17.0 set when
// experiments.getByUser or an override says "on", and back.
func TestEmojiSetFollowsEmojiV17(t *testing.T) {
	a, _ := testApp(t)
	t.Cleanup(func() { emoji.SetV17(false) })
	if emoji.V17() || a.Experiment(ExpEmojiV17) != "" {
		t.Fatalf("a new App reads emoji_v17 %q, set 17.0 %v", a.Experiment(ExpEmojiV17), emoji.V17())
	}
	if emoji.IsStandard("orca") || EmojiFrecencyID("orca") != "Eorca" || EmojiFrecencyName("E1facd") == "orca" {
		t.Error("16.0 set: orca is a standard emoji")
	}
	a.setExperiments(map[string]string{ExpEmojiV17: "on"})
	if !emoji.V17() || !emoji.IsStandard("orca") {
		t.Fatal("experiments.getByUser's emoji_v17 on didn't pick the 17.0 set")
	}
	if EmojiFrecencyID("orca") != "E1facd" || EmojiFrecencyName("E1facd") != "orca" {
		t.Errorf("17.0 set: orca's frecency id %q, E1facd's name %q", EmojiFrecencyID("orca"), EmojiFrecencyName("E1facd"))
	}
	found := false
	for _, e := range a.emojiSet().All() {
		found = found || e.Name == "hairy_creature"
	}
	if !found {
		t.Error("17.0 set: the frecency candidates lack hairy_creature")
	}
	a.SetExperimentGroup(ExpEmojiV17, "off")
	if emoji.V17() {
		t.Error("an override to off kept the 17.0 set")
	}
	for _, e := range a.emojiSet().All() {
		if e.Name == "hairy_creature" {
			t.Error("16.0 set: the frecency candidates have hairy_creature")
		}
	}
	a.ClearExperimentGroup(ExpEmojiV17)
	if !emoji.V17() {
		t.Error("clearing the override didn't go back to the server's on")
	}
	a.setExperiments(map[string]string{})
	if emoji.V17() {
		t.Error("an answer without emoji_v17 kept the 17.0 set")
	}
}
