package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

func main() {
	// Create temp entry file with problematic imports that triggered the bugs.
	tmpDir, err := os.MkdirTemp("", "prebundle-test")
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: %v\n", err)
		os.Exit(1)
	}
	defer os.RemoveAll(tmpDir)

	// --- Test 1: Unknown externals + node builtin subpaths ---

	entry1 := filepath.Join(tmpDir, "entry_unknown.js")
	err = os.WriteFile(entry1, []byte(`
// Node builtin subpaths — NodeBuiltinEmptyPlugin must handle these
import "node:fs/promises";
import "fs/promises";
import "stream/web";
import "util/types";
import "node:path/posix";
import "node:stream/consumers";

// Unknown bare imports — UnknownExternalPlugin must catch these
import "vue";
import "react-native";
import "@remix-run/react";
import "next/navigation";
import "expo-crypto";
import "@vercel/analytics/react";

console.log("ok");
`), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: write entry: %v\n", err)
		os.Exit(1)
	}

	moduleMap1 := map[string]string{}

	result1 := api.Build(api.BuildOptions{
		EntryPoints: []string{entry1},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatESModule,
		Platform:    api.PlatformBrowser,
		Target:      api.ESNext,
		LogLevel:    api.LogLevelSilent,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap1, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(moduleMap1),
		},
	})

	if len(result1.Errors) > 0 {
		for _, e := range result1.Errors {
			fmt.Fprintf(os.Stderr, "  error: %s\n", e.Text)
		}
		fmt.Fprintln(os.Stderr, "FAIL: test 1 — build had errors (unknown externals + builtin subpaths)")
		os.Exit(1)
	}

	if len(result1.OutputFiles) == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: test 1 — no output files")
		os.Exit(1)
	}

	output1 := string(result1.OutputFiles[0].Contents)

	// Unknown imports must be preserved as external imports in the output
	for _, pkg := range []string{"vue", "react-native", "@remix-run/react", "next/navigation", "expo-crypto", "@vercel/analytics/react"} {
		if !strings.Contains(output1, fmt.Sprintf("%q", pkg)) {
			fmt.Fprintf(os.Stderr, "FAIL: test 1 — expected external import %q in output\n", pkg)
			os.Exit(1)
		}
	}

	// Node builtin subpaths must NOT appear as imports (they're empty-stubbed)
	for _, builtin := range []string{"node:fs/promises", "fs/promises", "stream/web", "util/types", "node:path/posix", "node:stream/consumers"} {
		if strings.Contains(output1, fmt.Sprintf("from %q", builtin)) || strings.Contains(output1, fmt.Sprintf("import %q", builtin)) {
			fmt.Fprintf(os.Stderr, "FAIL: test 1 — node builtin %q should be empty-stubbed, not imported\n", builtin)
			os.Exit(1)
		}
	}
	fmt.Println("  PASS: test 1 — unknown externals + node builtin subpaths")

	// --- Test 2: Known packages are NOT externalized ---

	// Resolve the known-pkg fixture to an absolute path.
	// The test binary runs from the repo root, so the fixture is at a known relative path.
	knownPkgDir := "test/prebundle_plugins/fixtures/known-pkg"
	absKnownPkg, _ := filepath.Abs(knownPkgDir)

	entry2 := filepath.Join(tmpDir, "entry_known.js")
	err = os.WriteFile(entry2, []byte(`
import { known } from "known-pkg";
console.log(known);
`), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: write entry2: %v\n", err)
		os.Exit(1)
	}

	moduleMap2 := map[string]string{
		"known-pkg": absKnownPkg,
	}

	result2 := api.Build(api.BuildOptions{
		EntryPoints: []string{entry2},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatESModule,
		Platform:    api.PlatformBrowser,
		Target:      api.ESNext,
		LogLevel:    api.LogLevelSilent,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap2, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(moduleMap2),
		},
	})

	if len(result2.Errors) > 0 {
		for _, e := range result2.Errors {
			fmt.Fprintf(os.Stderr, "  error: %s\n", e.Text)
		}
		fmt.Fprintln(os.Stderr, "FAIL: test 2 — build had errors (known package)")
		os.Exit(1)
	}

	if len(result2.OutputFiles) == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: test 2 — no output files")
		os.Exit(1)
	}

	output2 := string(result2.OutputFiles[0].Contents)

	// "known-pkg" must be bundled, NOT preserved as an external import
	if strings.Contains(output2, `"known-pkg"`) {
		fmt.Fprintln(os.Stderr, "FAIL: test 2 — known-pkg should be bundled, not external")
		os.Exit(1)
	}
	// The bundled output should contain the actual export value
	if !strings.Contains(output2, "known") {
		fmt.Fprintln(os.Stderr, "FAIL: test 2 — expected bundled content from known-pkg")
		os.Exit(1)
	}
	fmt.Println("  PASS: test 2 — known packages are bundled, not externalized")

	// --- Test 3: Scoped package name extraction ---
	// @scope/pkg/subpath where @scope/pkg IS known must NOT be externalized

	entry3 := filepath.Join(tmpDir, "entry_scoped.js")
	err = os.WriteFile(entry3, []byte(`
// @remix-run/react is unknown — should be external
import "@remix-run/react";

// @known-scope/known-pkg is known — should be bundled (even with subpath)
import "@known-scope/known-pkg/subpath";

console.log("ok");
`), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FAIL: write entry3: %v\n", err)
		os.Exit(1)
	}

	moduleMap3 := map[string]string{
		"@known-scope/known-pkg": absKnownPkg,
	}

	result3 := api.Build(api.BuildOptions{
		EntryPoints: []string{entry3},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatESModule,
		Platform:    api.PlatformBrowser,
		Target:      api.ESNext,
		LogLevel:    api.LogLevelSilent,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap3, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(moduleMap3),
		},
	})

	if len(result3.Errors) > 0 {
		for _, e := range result3.Errors {
			fmt.Fprintf(os.Stderr, "  error: %s\n", e.Text)
		}
		fmt.Fprintln(os.Stderr, "FAIL: test 3 — build had errors (scoped packages)")
		os.Exit(1)
	}

	if len(result3.OutputFiles) == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: test 3 — no output files")
		os.Exit(1)
	}

	output3 := string(result3.OutputFiles[0].Contents)

	// @remix-run/react is unknown — must be external
	if !strings.Contains(output3, `"@remix-run/react"`) {
		fmt.Fprintln(os.Stderr, "FAIL: test 3 — @remix-run/react should be external")
		os.Exit(1)
	}
	// @known-scope/known-pkg is known — must NOT appear as external import
	if strings.Contains(output3, `"@known-scope/known-pkg/subpath"`) {
		fmt.Fprintln(os.Stderr, "FAIL: test 3 — @known-scope/known-pkg/subpath should not be external")
		os.Exit(1)
	}
	fmt.Println("  PASS: test 3 — scoped package name extraction")

	// --- Test 4: Partial failure + retry (simulates @scure/bip32 scenario) ---
	// bad-pkg imports { nonExistent } from ./lib.js (which doesn't export it).
	// Building both good-pkg and bad-pkg together should produce errors.
	// We identify bad-pkg from error locations, retry without it, and verify
	// that good-pkg is successfully bundled.

	badPkgDir := "test/prebundle_plugins/fixtures/bad-pkg"
	absBadPkg, _ := filepath.Abs(badPkgDir)

	moduleMap4 := map[string]string{
		"good-pkg": absKnownPkg,
		"bad-pkg":  absBadPkg,
	}

	entryPoints4 := []api.EntryPoint{
		{InputPath: filepath.Join(absKnownPkg, "index.js"), OutputPath: "good-pkg"},
		{InputPath: filepath.Join(absBadPkg, "index.js"), OutputPath: "bad-pkg"},
	}

	outdir4, _ := filepath.Abs(filepath.Join(tmpDir, "out4"))
	result4 := api.Build(api.BuildOptions{
		EntryPointsAdvanced: entryPoints4,
		Bundle:              true,
		Write:               false,
		Format:              api.FormatESModule,
		Platform:            api.PlatformBrowser,
		Target:              api.ESNext,
		Splitting:           true,
		ChunkNames:          "chunk-[hash]",
		Outdir:              outdir4,
		LogLevel:            api.LogLevelSilent,
		IgnoreAnnotations:   true,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap4, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(moduleMap4),
		},
	})

	if len(result4.Errors) == 0 {
		fmt.Fprintln(os.Stderr, "FAIL: test 4 — expected errors from bad-pkg, got none")
		os.Exit(1)
	}

	// Identify failing packages by matching error locations to package dirs
	dirToName := make(map[string]string)
	for name, pkgDir := range moduleMap4 {
		absDir, _ := filepath.Abs(pkgDir)
		dirToName[absDir] = name
	}
	failing := make(map[string]bool)
	for _, e := range result4.Errors {
		if e.Location == nil || e.Location.File == "" {
			continue
		}
		absFile, _ := filepath.Abs(e.Location.File)
		for dir, name := range dirToName {
			if strings.HasPrefix(absFile, dir+"/") {
				failing[name] = true
				break
			}
		}
	}

	if !failing["bad-pkg"] {
		fmt.Fprintln(os.Stderr, "FAIL: test 4 — expected bad-pkg to be identified as failing")
		os.Exit(1)
	}
	if failing["good-pkg"] {
		fmt.Fprintln(os.Stderr, "FAIL: test 4 — good-pkg should NOT be identified as failing")
		os.Exit(1)
	}

	// Retry without bad-pkg — only good-pkg entry point
	entryPoints4retry := []api.EntryPoint{
		{InputPath: filepath.Join(absKnownPkg, "index.js"), OutputPath: "good-pkg"},
	}

	result4retry := api.Build(api.BuildOptions{
		EntryPointsAdvanced: entryPoints4retry,
		Bundle:              true,
		Write:               false,
		Format:              api.FormatESModule,
		Platform:            api.PlatformBrowser,
		Target:              api.ESNext,
		Splitting:           true,
		ChunkNames:          "chunk-[hash]",
		Outdir:              outdir4,
		LogLevel:            api.LogLevelSilent,
		IgnoreAnnotations:   true,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap4, "browser"),
			common.NodeBuiltinEmptyPlugin(),
			common.UnknownExternalPlugin(moduleMap4),
		},
	})

	if len(result4retry.Errors) > 0 {
		for _, e := range result4retry.Errors {
			fmt.Fprintf(os.Stderr, "  error: %s\n", e.Text)
		}
		fmt.Fprintln(os.Stderr, "FAIL: test 4 — retry without bad-pkg should succeed")
		os.Exit(1)
	}

	// Verify good-pkg is in the output
	foundGood := false
	for _, f := range result4retry.OutputFiles {
		rel, _ := filepath.Rel(outdir4, f.Path)
		if strings.Contains(rel, "good-pkg") {
			foundGood = true
			break
		}
	}
	if !foundGood {
		fmt.Fprintln(os.Stderr, "FAIL: test 4 — good-pkg should be in retry output")
		os.Exit(1)
	}
	fmt.Println("  PASS: test 4 — partial failure + retry excludes bad-pkg, keeps good-pkg")

	fmt.Println("prebundle_plugins: all tests passed")
}
