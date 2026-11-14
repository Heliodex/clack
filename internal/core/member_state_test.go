package core

import (
	"context"
	"slices"
	"testing"
	"time"
)

// A view reading a member core doesn't hold gets getMemberById's unknown
// stub while users/info runs (MemberUnknown: views draw BaseMissingMember,
// never the id); once it answers, the member, or for an id it didn't
// return a non-existent member, and "names" names both so the views draw
// them again. Synthetic ids (usersEdge returns only "UK…").
func TestMemberStateOf(t *testing.T) {
	a, _ := testApp(t)
	e := &usersEdge{}
	e.serve(t, a)
	evs, stop := a.Subscribe()
	defer stop()
	const known, nobody = "UK0STATE01", "UNOSTATE01"
	for range 3 {
		for _, id := range []string{known, nobody} {
			if st := a.MemberStateOf(id); st != MemberUnknown {
				t.Fatalf("%s before users/info: state %v, want the unknown stub", id, st)
			}
			if st := a.MissingMember(id); st != MemberUnknown {
				t.Fatalf("%s before users/info: MissingMember %v", id, st)
			}
		}
	}
	if st := a.MemberStateOf("USELF"); st != MemberKnown {
		t.Errorf("the viewer: state %v, want known", st)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.memberLoop(ctx) // the misses above are asked for in one request
	ev := next(t, evs, "names")
	if !slices.Contains(ev.Users, known) || !slices.Contains(ev.Users, nobody) {
		t.Errorf("names %v, want both", ev.Users)
	}
	if got := e.requests(); len(got) != 1 {
		t.Errorf("users/info requests %v, want one for both", got)
	}
	if st := a.MemberStateOf(known); st != MemberKnown {
		t.Errorf("%s after users/info: %v, want known", known, st)
	}
	if st := a.MemberStateOf(nobody); st != MemberNonExistent {
		t.Errorf("%s after users/info: %v, want non-existent", nobody, st)
	}
	if st := a.MissingMember(nobody); st != MemberNonExistent {
		t.Errorf("MissingMember(%s) = %v, want non-existent", nobody, st)
	}
	time.Sleep(300 * time.Millisecond)
	if got := e.requests(); len(got) != 1 {
		t.Errorf("read again after the answer: %d requests", len(got))
	}
}

// looksLikeMemberId (5oKS): which ids getMemberById fetches, and which
// MemberSlug draws as "@id" once they're non-existent.
func TestLooksLikeMemberID(t *testing.T) {
	for id, want := range map[string]bool{
		"U0CFFEW8VMK": true, "W012ABCDEF": true, "@U0CFFEW8VMK": true, "U01234567": true,
		"USLACKBOT": true, "USEARCH": true, "UCHANNELEXPERT": true, "USLACK": true, "UTBD": true,
		"U0123456": false, "U1": false, "UBOB": false, "B0123ABCDE": false, "C0123ABCDE": false,
		"u0cffew8vmk": false, "U0123abcd9": false, "": false, "@": false,
	} {
		if got := LooksLikeMemberID(id); got != want {
			t.Errorf("LooksLikeMemberID(%q) = %v, want %v", id, got, want)
		}
	}
}
