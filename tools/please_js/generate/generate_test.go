
// A package declaring install scripts is not a reason to run them. Someone
// naming it is, and only the packages named are marked.
func TestAllowHooksMarksOnlyWhatWasNamed(t *testing.T) {
	plan := &generate.Plan{Entries: []generate.Entry{
		{Target: "a_1", Package: "a"},
		{Target: "b_1", Package: "b"},
		{Target: "b_1_peer", Package: "b"},
	}}
	if err := plan.AllowHooks([]string{"b"}); err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, e := range plan.Entries {
		got[e.Target] = e.RunHooks
	}
	// Both resolutions of b, because one package under two peer resolutions is
	// one decision, not two.
	if got["a_1"] || !got["b_1"] || !got["b_1_peer"] {
		t.Errorf("marked %v", got)
	}
}

// A name that matches nothing is a typo or a package that moved, and the thing
// it silently fails to do is run a build step something needs.
func TestAllowHooksRefusesANameItCannotResolve(t *testing.T) {
	plan := &generate.Plan{Entries: []generate.Entry{{Target: "a_1", Package: "a"}}}
	err := plan.AllowHooks([]string{"a", "typo"})
	if err == nil {
		t.Fatal("expected an error naming the unresolved package")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error should name what was not found, got: %v", err)
	}
}
