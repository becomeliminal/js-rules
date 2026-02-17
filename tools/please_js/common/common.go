package common

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Loaders maps file extensions to esbuild loaders.
var Loaders = map[string]api.Loader{
	".js":   api.LoaderJS,
	".jsx":  api.LoaderJSX,
	".ts":   api.LoaderTS,
	".tsx":  api.LoaderTSX,
	".json": api.LoaderJSON,
	".css":  api.LoaderCSS,
	".mjs":  api.LoaderJS,
	".cjs":  api.LoaderJS,
	".md":   api.LoaderText,
}

// ParseModuleConfig reads a moduleconfig file mapping module names to paths.
// Each line has the format "module_name=path_to_output_dir".
func ParseModuleConfig(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		// Empty moduleconfig is valid (no dependencies)
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer f.Close()

	modules := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			modules[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return modules, scanner.Err()
}

// BuildAlias converts a moduleconfig map to an esbuild Alias map with
// absolute paths. esbuild's Alias option maps bare import specifiers
// (e.g. "react") directly to filesystem paths (plz-out outputs),
// eliminating the need for node_modules symlinks.
func BuildAlias(moduleMap map[string]string) (map[string]string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get working directory: %w", err)
	}

	alias := make(map[string]string, len(moduleMap))
	for moduleName, modulePath := range moduleMap {
		absPath := modulePath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(wd, absPath)
		}
		alias[moduleName] = absPath
	}
	return alias, nil
}

// ParseFormat converts a format string to an esbuild Format constant.
func ParseFormat(f string) api.Format {
	switch f {
	case "cjs":
		return api.FormatCommonJS
	case "iife":
		return api.FormatIIFE
	default:
		return api.FormatESModule
	}
}

// ParsePlatform converts a platform string to an esbuild Platform constant.
func ParsePlatform(p string) api.Platform {
	switch p {
	case "node":
		return api.PlatformNode
	default:
		return api.PlatformBrowser
	}
}

// RawImportPlugin returns an esbuild plugin that strips ?raw suffixes from
// import paths. Files loaded this way use the text loader, returning contents
// as a string — equivalent to Vite's ?raw imports.
func RawImportPlugin() api.Plugin {
	return api.Plugin{
		Name: "raw-import",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: `\?raw$`},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					// Strip ?raw suffix and resolve normally
					cleanPath := strings.TrimSuffix(args.Path, "?raw")
					// Resolve relative to the importer's directory
					resolveDir := args.ResolveDir
					resolved := filepath.Join(resolveDir, cleanPath)
					return api.OnResolveResult{
						Path:      resolved,
						Namespace: "file",
					}, nil
				},
			)
		},
	}
}
