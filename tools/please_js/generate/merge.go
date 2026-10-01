package generate

import (
	"sort"

	"tools/please_js/store"
)

// ProjectTree is what one workspace project's node_modules is built from: the
// entries to stage, what each one's dependencies resolve to, and what the
// project imports at its top level.
type ProjectTree struct {
	Closure []string               // targets to stage, sorted
	Refs    map[string][]store.Ref // target -> its dependencies
	Links   []store.Ref            // the project's own imports
	// Merged records, for each package@version that pnpm resolved more than
	// once, every copy that was folded into the one kept, keyed by the kept
	// copy's target. Empty when nothing was merged.
	Merged map[string][]string
}

// Project returns the tree a workspace project stages.
//
// pnpm resolves a package once per peer context: the same viem@2.57.1 becomes
// two packages when one subtree reaches it with zod 3 as its optional peer
// and another with zod 4, and every package above it splits with it. For a
// library holding module-level state -- a wallet connection, a React context,
// an error class something checks with instanceof -- two copies in one app is
// two of that state, and a bundler ships both. npm's layout never does this:
// one copy per version, whatever reaches it.
//
// With merge, each package@version in the project's tree is staged once.
// The copy kept is the one the project imports directly if there is one,
// otherwise the one most of the tree depends on, otherwise the first by key,
// so the choice is deterministic. Every dependency edge that pointed at a
// folded copy points at the kept one, and the tree is re-walked from the
// project's imports so nothing only a folded copy reached is staged.
//
// Without merge, the tree is exactly what pnpm resolved.
func (p *Plan) Project(project string, merge bool) ProjectTree {
	entries := make(map[string]Entry, len(p.Entries))
	for _, e := range p.Entries {
		entries[e.Target] = e
	}
	refs := p.Refs()
	links := p.Links(project)
	closure := p.Closure[project]

	if !merge {
		return ProjectTree{Closure: closure, Refs: refs, Links: links, Merged: map[string][]string{}}
	}

	// How often each copy is depended on inside this project's tree, and
	// which copies the project imports itself.
	direct := map[string]bool{}
	for _, l := range links {
		direct[l.Entry] = true
	}
	referrers := map[string]int{}
	for _, t := range closure {
		for _, r := range refs[t] {
			referrers[r.Entry]++
		}
	}

	groups := map[string][]string{} // package@version -> copies in this tree
	for _, t := range closure {
		e, ok := entries[t]
		if !ok {
			continue
		}
		pv := e.Package + "@" + e.Version
		groups[pv] = append(groups[pv], t)
	}

	keep := map[string]string{} // any copy -> the copy kept
	merged := map[string][]string{}
	for _, copies := range groups {
		sort.Slice(copies, func(i, j int) bool {
			a, b := copies[i], copies[j]
			if direct[a] != direct[b] {
				return direct[a]
			}
			if referrers[a] != referrers[b] {
				return referrers[a] > referrers[b]
			}
			return a < b
		})
		for _, c := range copies {
			keep[c] = copies[0]
		}
		if len(copies) > 1 {
			merged[copies[0]] = append([]string(nil), copies[1:]...)
		}
	}
	if len(merged) == 0 {
		// Nothing resolved twice: the tree is exactly what pnpm resolved.
		return ProjectTree{Closure: closure, Refs: refs, Links: links, Merged: merged}
	}
	canonical := func(t string) string {
		if k, ok := keep[t]; ok {
			return k
		}
		return t
	}

	mergedLinks := make([]store.Ref, len(links))
	for i, l := range links {
		mergedLinks[i] = store.Ref{As: l.As, Entry: canonical(l.Entry)}
	}
	mergedRefs := make(map[string][]store.Ref, len(refs))
	for t, rs := range refs {
		out := make([]store.Ref, len(rs))
		for i, r := range rs {
			out[i] = store.Ref{As: r.As, Entry: canonical(r.Entry)}
		}
		mergedRefs[t] = out
	}

	// Re-walk from the project's imports: a package only a folded copy
	// depended on is no longer part of this tree.
	seen := map[string]bool{}
	stack := make([]string, 0, len(mergedLinks))
	for _, l := range mergedLinks {
		stack = append(stack, l.Entry)
	}
	for len(stack) > 0 {
		t := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if seen[t] {
			continue
		}
		seen[t] = true
		for _, r := range mergedRefs[t] {
			stack = append(stack, r.Entry)
		}
	}
	walked := make([]string, 0, len(seen))
	for t := range seen {
		walked = append(walked, t)
	}
	sort.Strings(walked)

	return ProjectTree{Closure: walked, Refs: mergedRefs, Links: mergedLinks, Merged: merged}
}
