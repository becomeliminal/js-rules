package common

import (
	"fmt"
	"strings"
)

// DepTarget converts an npm package name to a subrepo target reference.
// "react" → "//react"
// "@types/react" → "//@types/react:react"
func DepTarget(name string) string {
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		targetName := parts[len(parts)-1]
		return fmt.Sprintf("//%s:%s", name, targetName)
	}
	return fmt.Sprintf("//%s", name)
}

// VersionedTargetName generates a version-qualified target name for
// a version-conflict target.
// "zod", "4.3.6" → "zod_v4_3_6"
// "@types/react", "17.0.0" → "react_v17_0_0"
func VersionedTargetName(name, version string) string {
	base := name
	if strings.Contains(name, "/") {
		parts := strings.Split(name, "/")
		base = parts[len(parts)-1]
	}
	v := strings.NewReplacer(".", "_", "-", "_").Replace(version)
	return fmt.Sprintf("%s_v%s", base, v)
}
