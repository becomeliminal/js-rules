package esmdev

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// trailingCommaRe matches trailing commas before closing braces/brackets.
var trailingCommaRe = regexp.MustCompile(`,\s*([}\]])`)

// stripJSONC removes comments and trailing commas from JSONC content (as used
// in tsconfig.json). Handles // line comments, /* */ block comments, and
// trailing commas before } or ]. String contents are preserved.
func stripJSONC(data []byte) []byte {
	var result []byte
	i := 0
	inString := false

	for i < len(data) {
		if inString {
			if data[i] == '\\' && i+1 < len(data) {
				result = append(result, data[i], data[i+1])
				i += 2
				continue
			}
			if data[i] == '"' {
				inString = false
			}
			result = append(result, data[i])
			i++
			continue
		}

		// Not in string
		if data[i] == '"' {
			inString = true
			result = append(result, data[i])
			i++
			continue
		}

		// Line comment
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}

		// Block comment
		if i+1 < len(data) && data[i] == '/' && data[i+1] == '*' {
			i += 2
			for i+1 < len(data) && !(data[i] == '*' && data[i+1] == '/') {
				i++
			}
			if i+1 < len(data) {
				i += 2
			}
			continue
		}

		result = append(result, data[i])
		i++
	}

	// Remove trailing commas before } and ]
	result = trailingCommaRe.ReplaceAll(result, []byte("$1"))

	return result
}

// parseTsconfigPaths reads a tsconfig.json and returns import map entries for
// path aliases. Wildcard entries like "@/*": ["./src/*"] produce prefix mappings
// "@/" → "/src/". Exact entries like "~utils": ["./src/utils"] produce exact
// mappings. All paths are resolved relative to baseUrl and made URL-absolute
// relative to packageRoot.
func parseTsconfigPaths(tsconfigPath, packageRoot string) map[string]string {
	data, err := os.ReadFile(tsconfigPath)
	if err != nil {
		return nil
	}

	// tsconfig.json supports JSONC (comments + trailing commas)
	clean := stripJSONC(data)

	var tsconfig struct {
		CompilerOptions struct {
			BaseUrl string              `json:"baseUrl"`
			Paths   map[string][]string `json:"paths"`
		} `json:"compilerOptions"`
	}
	if err := json.Unmarshal(clean, &tsconfig); err != nil {
		return nil
	}

	if len(tsconfig.CompilerOptions.Paths) == 0 {
		return nil
	}

	baseUrl := tsconfig.CompilerOptions.BaseUrl
	if baseUrl == "" {
		baseUrl = "."
	}
	// Resolve baseUrl relative to tsconfig directory
	tsconfigDir := filepath.Dir(tsconfigPath)
	absBaseUrl := filepath.Join(tsconfigDir, baseUrl)

	entries := make(map[string]string)
	for alias, targets := range tsconfig.CompilerOptions.Paths {
		if len(targets) == 0 {
			continue
		}
		target := targets[0] // Use the first mapping

		if strings.HasSuffix(alias, "/*") && strings.HasSuffix(target, "/*") {
			// Wildcard: "@/*" → "./src/*" becomes "@/" → "/src/"
			prefix := strings.TrimSuffix(alias, "*")
			targetDir := strings.TrimSuffix(target, "*")
			absTarget := filepath.Join(absBaseUrl, targetDir)
			rel, err := filepath.Rel(packageRoot, absTarget)
			if err != nil {
				continue
			}
			entries[prefix] = "/" + filepath.ToSlash(rel) + "/"
		} else {
			// Exact: "~utils" → "./src/utils/index.ts"
			absTarget := filepath.Join(absBaseUrl, target)
			rel, err := filepath.Rel(packageRoot, absTarget)
			if err != nil {
				continue
			}
			entries[alias] = "/" + filepath.ToSlash(rel)
		}
	}

	return entries
}
