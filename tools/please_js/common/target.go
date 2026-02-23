package common

import (
	"fmt"
	"strings"
)

// FlattenPkgName converts an npm package name to a flat directory/target name.
// Mirrors go-rules' _module_rule_name: module.replace("/", "_")
// "@babel/runtime" → "babel_runtime"
// "@tiptap/react"  → "tiptap_react"
// "react"          → "react"
// "lodash-es"      → "lodash-es"
func FlattenPkgName(name string) string {
	name = strings.TrimPrefix(name, "@")
	return strings.ReplaceAll(name, "/", "_")
}

// DepTarget converts an npm package name to a subrepo target reference
// using the flat directory/target name.
// "react"          → "//react"
// "@types/react"   → "//types_react"
// "@babel/runtime" → "//babel_runtime"
func DepTarget(name string) string {
	return fmt.Sprintf("//%s", FlattenPkgName(name))
}

// VersionedTargetName generates a version-qualified target name for
// a version-conflict target, using the flat package name for global uniqueness.
// "zod", "4.3.6"          → "zod_v4_3_6"
// "@types/react", "17.0.0" → "types_react_v17_0_0"
func VersionedTargetName(name, version string) string {
	base := FlattenPkgName(name)
	v := strings.NewReplacer(".", "_", "-", "_").Replace(version)
	return fmt.Sprintf("%s_v%s", base, v)
}
