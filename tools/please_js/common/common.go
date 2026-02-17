package common

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"
)

// Loaders maps file extensions to esbuild loaders.
var Loaders = map[string]api.Loader{
	".js":    api.LoaderJS,
	".jsx":   api.LoaderJSX,
	".ts":    api.LoaderTS,
	".tsx":   api.LoaderTSX,
	".json":  api.LoaderJSON,
	".css":   api.LoaderCSS,
	".mjs":   api.LoaderJS,
	".cjs":   api.LoaderJS,
	".md":    api.LoaderText,
	".woff":  api.LoaderFile,
	".woff2": api.LoaderFile,
	".ttf":   api.LoaderFile,
	".eot":   api.LoaderFile,
	".svg":   api.LoaderFile,
	".png":   api.LoaderFile,
	".jpg":   api.LoaderFile,
	".gif":   api.LoaderFile,
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

// ModuleResolvePlugin returns an esbuild plugin that resolves bare import
// specifiers using the moduleconfig map. Unlike esbuild's Alias option,
// this uses build.Resolve() to properly handle package.json "exports",
// "main", "module" fields, and subpath imports.
func ModuleResolvePlugin(moduleMap map[string]string) api.Plugin {
	return api.Plugin{
		Name: "module-resolve",
		Setup: func(build api.PluginBuild) {
			build.OnResolve(api.OnResolveOptions{Filter: ".*"},
				func(args api.OnResolveArgs) (api.OnResolveResult, error) {
					// Skip relative and absolute paths
					if len(args.Path) == 0 || args.Path[0] == '.' || args.Path[0] == '/' {
						return api.OnResolveResult{}, nil
					}

					// Find longest matching module prefix
					bestMatch := ""
					bestPath := ""
					for name, path := range moduleMap {
						if args.Path == name || strings.HasPrefix(args.Path, name+"/") {
							if len(name) > len(bestMatch) {
								bestMatch = name
								bestPath = path
							}
						}
					}

					if bestMatch == "" {
						return api.OnResolveResult{}, nil
					}

					// Re-resolve using esbuild's resolver from the package dir.
					// This correctly handles exports maps, main/module fields, etc.
					resolvePath := "."
					if args.Path != bestMatch {
						resolvePath = "./" + strings.TrimPrefix(args.Path, bestMatch+"/")
					}
					result := build.Resolve(resolvePath, api.ResolveOptions{
						ResolveDir: bestPath,
						Kind:       args.Kind,
					})
					if len(result.Errors) == 0 {
						return api.OnResolveResult{Path: result.Path}, nil
					}

					return api.OnResolveResult{}, nil
				},
			)
		},
	}
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

// tailwindCache caches Tailwind CLI output and tracks when it was last generated.
type tailwindCache struct {
	mu          sync.Mutex
	css         string
	lastRunTime time.Time
}

// isStale reports whether the CSS input file or Tailwind config has been
// modified since the cache was last populated. JS/TS content files are NOT
// checked — in a dev server, the file edit that triggers each rebuild would
// always have a newer mtime, defeating the cache entirely. The trade-off is
// that adding a brand-new Tailwind class in JSX won't appear until the CSS
// file or config is also saved (or the server is restarted).
func (c *tailwindCache) isStale(paths ...string) bool {
	if c.lastRunTime.IsZero() {
		return true
	}
	for _, p := range paths {
		if p == "" {
			continue
		}
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if info.ModTime().After(c.lastRunTime) {
			return true
		}
	}
	return false
}

// TailwindPlugin returns an esbuild plugin that processes CSS files containing
// @tailwind directives through the Tailwind CLI binary. CSS files without
// @tailwind directives are left for esbuild's default CSS loader.
// Results are cached and only recomputed when the CSS input or config changes.
func TailwindPlugin(tailwindBin, tailwindConfig string) api.Plugin {
	cache := &tailwindCache{}

	return api.Plugin{
		Name: "tailwind-css",
		Setup: func(build api.PluginBuild) {
			build.OnLoad(api.OnLoadOptions{Filter: `\.css$`},
				func(args api.OnLoadArgs) (api.OnLoadResult, error) {
					content, err := os.ReadFile(args.Path)
					if err != nil {
						return api.OnLoadResult{}, err
					}

					// Only process files that contain @tailwind directives
					if !bytes.Contains(content, []byte("@tailwind")) {
						return api.OnLoadResult{}, nil
					}

					cache.mu.Lock()
					defer cache.mu.Unlock()

					if !cache.isStale(args.Path, tailwindConfig) {
						css := cache.css
						return api.OnLoadResult{
							Contents: &css,
							Loader:   api.LoaderCSS,
						}, nil
					}

					cmdArgs := []string{"--input", args.Path}
					if tailwindConfig != "" {
						cmdArgs = append(cmdArgs, "--config", tailwindConfig)
					}

					cmd := exec.Command(tailwindBin, cmdArgs...)
					var stdout, stderr bytes.Buffer
					cmd.Stdout = &stdout
					cmd.Stderr = &stderr

					if err := cmd.Run(); err != nil {
						return api.OnLoadResult{}, fmt.Errorf("tailwind failed on %s: %v\n%s", args.Path, err, stderr.String())
					}

					cache.css = stdout.String()
					cache.lastRunTime = time.Now()

					css := cache.css
					return api.OnLoadResult{
						Contents: &css,
						Loader:   api.LoaderCSS,
					}, nil
				},
			)
		},
	}
}
