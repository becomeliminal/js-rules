package generate_test

import (
	"reflect"
	"testing"

	"tools/please_js/generate"
	"tools/please_js/store"
)

// The shape found in a real wallet app: viem resolved twice (its optional zod
// peer filled with zod 3 in one subtree and zod 4 in another), and @wagmi/core
// split with it because each copy depends on a different viem.
func splitWagmi() *generate.Plan {
	return &generate.Plan{
		Entries: []generate.Entry{
			{Target: "app_wagmi_1", Package: "wagmi", Version: "2.19.5", Deps: map[string]string{"@wagmi/core": "wagmi_core_a"}},
			{Target: "connectkit_1", Package: "connectkit", Version: "1.9.1", Deps: map[string]string{"@wagmi/core": "wagmi_core_b"}},
			{Target: "wagmi_core_a", Package: "@wagmi/core", Version: "2.22.1", Deps: map[string]string{"viem": "viem_zod3"}},
			{Target: "wagmi_core_b", Package: "@wagmi/core", Version: "2.22.1", Deps: map[string]string{"viem": "viem_zod4"}},
			{Target: "viem_zod3", Package: "viem", Version: "2.57.1", Deps: map[string]string{"zod": "zod_3"}},
			{Target: "viem_zod4", Package: "viem", Version: "2.57.1", Deps: map[string]string{"zod": "zod_4"}},
			{Target: "zod_3", Package: "zod", Version: "3.25.76", Deps: map[string]string{}},
			{Target: "zod_4", Package: "zod", Version: "4.6.5", Deps: map[string]string{}},
		},
		Closure: map[string][]string{"../../app": {
			"app_wagmi_1", "connectkit_1", "viem_zod3", "viem_zod4",
			"wagmi_core_a", "wagmi_core_b", "zod_3", "zod_4",
		}},
		Direct: map[string]map[string]string{"../../app": {
			"wagmi": "app_wagmi_1", "connectkit": "connectkit_1",
		}},
	}
}

func TestProjectMergesPeerVariantsToOneCopyPerVersion(t *testing.T) {
	// GIVEN an app whose tree holds two copies each of @wagmi/core and viem
	plan := splitWagmi()

	// WHEN its tree is built with merging
	tree := plan.Project("../../app", true)

	// THEN each package@version is staged once, and what only a folded copy
	// reached (zod 4, via the folded viem) is gone
	want := []string{"app_wagmi_1", "connectkit_1", "viem_zod3", "wagmi_core_a", "zod_3"}
	if !reflect.DeepEqual(tree.Closure, want) {
		t.Errorf("closure = %v, want %v", tree.Closure, want)
	}
	// AND both dependents of @wagmi/core now reach the same copy
	if got := tree.Refs["connectkit_1"]; !reflect.DeepEqual(got, []store.Ref{{As: "@wagmi/core", Entry: "wagmi_core_a"}}) {
		t.Errorf("connectkit's @wagmi/core = %v, want wagmi_core_a", got)
	}
	// AND the merge is reported, kept copy -> folded copies
	wantMerged := map[string][]string{"wagmi_core_a": {"wagmi_core_b"}, "viem_zod3": {"viem_zod4"}}
	if !reflect.DeepEqual(tree.Merged, wantMerged) {
		t.Errorf("merged = %v, want %v", tree.Merged, wantMerged)
	}
}

func TestProjectKeepsTheCopyTheProjectImportsDirectly(t *testing.T) {
	// GIVEN the app imports viem itself, and its direct copy is the zod 4 one,
	// even though more of the tree reaches the zod 3 copy
	plan := splitWagmi()
	plan.Direct["../../app"]["viem"] = "viem_zod4"

	// WHEN its tree is built with merging
	tree := plan.Project("../../app", true)

	// THEN the directly imported copy is the one kept
	if got := tree.Merged["viem_zod4"]; !reflect.DeepEqual(got, []string{"viem_zod3"}) {
		t.Errorf("the direct import should win: merged = %v", tree.Merged)
	}
}

func TestProjectWithoutMergeIsExactlyWhatPnpmResolved(t *testing.T) {
	// GIVEN the same split tree
	plan := splitWagmi()

	// WHEN its tree is built without merging
	tree := plan.Project("../../app", false)

	// THEN every variant is staged, untouched
	if !reflect.DeepEqual(tree.Closure, plan.Closure["../../app"]) {
		t.Errorf("closure = %v, want pnpm's %v", tree.Closure, plan.Closure["../../app"])
	}
	if len(tree.Merged) != 0 {
		t.Errorf("nothing should be reported merged: %v", tree.Merged)
	}
}

func TestProjectWithNothingToMergeIsUnchanged(t *testing.T) {
	// GIVEN a tree where every package@version resolved once
	plan := &generate.Plan{
		Entries: []generate.Entry{
			{Target: "ms_2.1.3", Package: "ms", Version: "2.1.3", Deps: map[string]string{}},
		},
		Closure: map[string][]string{"../../app": {"ms_2.1.3"}},
		Direct:  map[string]map[string]string{"../../app": {"ms": "ms_2.1.3"}},
	}

	// WHEN its tree is built with merging
	tree := plan.Project("../../app", true)

	// THEN it is exactly pnpm's tree
	if !reflect.DeepEqual(tree.Closure, []string{"ms_2.1.3"}) || len(tree.Merged) != 0 {
		t.Errorf("closure = %v merged = %v, want pnpm's tree untouched", tree.Closure, tree.Merged)
	}
}
