package esmdev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

func (s *esmServer) handleHTML(w http.ResponseWriter, r *http.Request, start time.Time) {
	htmlPath := r.URL.Path
	if htmlPath == "/" || !strings.HasSuffix(htmlPath, ".html") {
		htmlPath = "/index.html"
	}

	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(htmlPath))
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[req] %s %s → 404 (%dms)\033[0m\n",
			r.Method, r.URL.Path, time.Since(start).Milliseconds())
		return
	}

	html := rewriteHTML(string(data), s.importMapJSON, s.hasRefresh, s.entryURLPath, s.sourceRoot, s.packageRoot)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html))
	fmt.Printf("  \033[2m[html] %s %s → 200 (%dms)\033[0m\n",
		r.Method, r.URL.Path, time.Since(start).Milliseconds())
}

func (s *esmServer) handleSource(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	resolved := resolveSourceFile(s.packageRoot, urlPath)
	if resolved == "" && s.packageRoot != s.sourceRoot {
		resolved = resolveSourceFile(s.sourceRoot, urlPath)
	}
	if resolved == "" {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[req] %s %s → 404 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Check cache
	info, err := os.Stat(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if cached, ok := s.transCache.Load(resolved); ok {
		entry := cached.(*transformEntry)
		if entry.modTime.Equal(info.ModTime()) {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(entry.code)
			return
		}
	}

	// Read and transform
	src, err := os.ReadFile(resolved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loader := loaderForFile(resolved)

	transformOpts := api.TransformOptions{
		Loader:         loader,
		Format:         api.FormatESModule,
		Target:         api.ESNext,
		JSX:            api.JSXAutomatic,
		Sourcemap:      api.SourceMapInline,
		SourcesContent: api.SourcesContentInclude,
		Sourcefile:     urlPath,
		Define:         s.define,
		LogLevel:       api.LogLevelWarning,
	}
	if s.tsconfig != "" {
		// Read tsconfig for JSX settings
		tsconfigData, err := os.ReadFile(s.tsconfig)
		if err == nil {
			transformOpts.TsconfigRaw = string(tsconfigData)
		}
	}

	result := api.Transform(string(src), transformOpts)
	if len(result.Errors) > 0 {
		// Return error as a JS module that logs the error
		errMsg := result.Errors[0].Text
		errJS := fmt.Sprintf(`console.error("[esm-dev] Transform error in %s:\\n%s");`, urlPath, strings.ReplaceAll(errMsg, `"`, `\"`))
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK) // Browser needs 200 to execute the error reporter
		w.Write([]byte(errJS))
		fmt.Printf("  \033[31m[error] %s %s: %s\033[0m\n", r.Method, urlPath, errMsg)
		return
	}

	code := result.Code

	// Inject React Fast Refresh registration if enabled and not the entry file.
	if s.hasRefresh && urlPath != s.entryURLPath {
		components := detectComponents(string(code))
		s.componentFiles.Store(resolved, len(components) > 0)
		if len(components) > 0 {
			code = injectRefreshRegistration(code, urlPath, components)
		}
	}

	// Cache the result
	s.transCache.Store(resolved, &transformEntry{
		code:    code,
		modTime: info.ModTime(),
	})

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(code)
	fmt.Printf("  \033[2m[transform] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

// handleLibSource serves local js_library source files via /@lib/ URLs.
// Strips the /@lib/ prefix, finds the matching library by longest-prefix match,
// resolves the file, and on-demand transforms it (same as handleSource).
func (s *esmServer) handleLibSource(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	// Strip /@lib/ prefix: "/@lib/common/js/ui/Spinner.tsx" → "common/js/ui/Spinner.tsx"
	specPath := strings.TrimPrefix(urlPath, "/@lib/")

	// Find matching library by longest-prefix match
	bestLib := ""
	bestDir := ""
	for name, dir := range s.localLibs {
		if specPath == name || strings.HasPrefix(specPath, name+"/") {
			if len(name) > len(bestLib) {
				bestLib = name
				bestDir = dir
			}
		}
	}
	if bestLib == "" {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[lib] %s %s → 404 no matching lib (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Extract subpath: "common/js/ui/Spinner.tsx" with lib "common/js/ui" → "/Spinner.tsx"
	subpath := "/"
	if specPath != bestLib {
		subpath = "/" + strings.TrimPrefix(specPath, bestLib+"/")
	}

	resolved := resolveSourceFile(bestDir, subpath)
	if resolved == "" {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[lib] %s %s → 404 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Check cache
	info, err := os.Stat(resolved)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	if cached, ok := s.transCache.Load(resolved); ok {
		entry := cached.(*transformEntry)
		if entry.modTime.Equal(info.ModTime()) {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(entry.code)
			return
		}
	}

	// Read and transform
	src, err := os.ReadFile(resolved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	loader := loaderForFile(resolved)

	transformOpts := api.TransformOptions{
		Loader:         loader,
		Format:         api.FormatESModule,
		Target:         api.ESNext,
		JSX:            api.JSXAutomatic,
		Sourcemap:      api.SourceMapInline,
		SourcesContent: api.SourcesContentInclude,
		Sourcefile:     urlPath,
		Define:         s.define,
		LogLevel:       api.LogLevelWarning,
	}
	if s.tsconfig != "" {
		tsconfigData, err := os.ReadFile(s.tsconfig)
		if err == nil {
			transformOpts.TsconfigRaw = string(tsconfigData)
		}
	}

	result := api.Transform(string(src), transformOpts)
	if len(result.Errors) > 0 {
		errMsg := result.Errors[0].Text
		errJS := fmt.Sprintf(`console.error("[esm-dev] Transform error in %s:\\n%s");`, urlPath, strings.ReplaceAll(errMsg, `"`, `\"`))
		w.Header().Set("Content-Type", "application/javascript")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(errJS))
		fmt.Printf("  \033[31m[error] %s %s: %s\033[0m\n", r.Method, urlPath, errMsg)
		return
	}

	code := result.Code

	// Inject React Fast Refresh registration if enabled.
	if s.hasRefresh {
		components := detectComponents(string(code))
		s.componentFiles.Store(resolved, len(components) > 0)
		if len(components) > 0 {
			code = injectRefreshRegistration(code, urlPath, components)
		}
	}

	// Cache the result
	s.transCache.Store(resolved, &transformEntry{
		code:    code,
		modTime: info.ModTime(),
	})

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(code)
	fmt.Printf("  \033[2m[lib] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *esmServer) handleCSSModule(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	filePath := filepath.Join(s.packageRoot, filepath.FromSlash(urlPath))
	if _, err := os.Stat(filePath); err != nil && s.packageRoot != s.sourceRoot {
		filePath = filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
	}
	data, err := os.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	cssContent := string(data)

	// Process through Tailwind if configured and file has @tailwind directives
	if s.tailwindBin != "" && strings.Contains(cssContent, "@tailwind") {
		compiled, err := s.compileTailwind(filePath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  tailwind error: %v\n", err)
			// Fall through with raw CSS on error
		} else {
			cssContent = compiled
		}
	}

	// JSON-encode the CSS content for safe embedding in JS
	cssJSON, err := json.Marshal(cssContent)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js := fmt.Sprintf(cssModuleTemplate, urlPath, string(cssJSON))
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(js))
	fmt.Printf("  \033[2m[css-module] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *esmServer) handleAssetModule(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	js := fmt.Sprintf(assetModuleTemplate, urlPath)
	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(js))
	fmt.Printf("  \033[2m[asset-module] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

// handleDepOnDemand lazily bundles a dependency subpath that wasn't pre-bundled.
// This handles requests resolved via prefix import map entries (e.g.,
// "use-sync-external-store/shim/with-selector.js" → "/@deps/use-sync-external-store/shim/with-selector.js").
func (s *esmServer) handleDepOnDemand(w http.ResponseWriter, r *http.Request, urlPath string, start time.Time) {
	// Check on-demand cache first
	if data, ok := s.onDemandDeps.Load(urlPath); ok {
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(data.([]byte))
		fmt.Printf("  \033[2m[dep-lazy] %s %s → 200 cached (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Extract bare specifier from URL: "/@deps/pkg/sub/path.js" → "pkg/sub/path.js"
	spec := strings.TrimPrefix(urlPath, "/@deps/")
	// Strip .js suffix if the specifier without it resolves (import map may add .js)
	specNoJS := strings.TrimSuffix(spec, ".js")

	pkgName := packageNameFromSpec(spec)
	pkgDir, ok := s.moduleMap[pkgName]
	if !ok {
		http.NotFound(w, r)
		fmt.Printf("  \033[2m[dep-lazy] %s %s → 404 unknown package (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	absPkgDir, _ := filepath.Abs(pkgDir)

	// Compute subpath: "pkg/sub/path.js" → "./sub/path.js"
	subpath := "."
	if spec != pkgName {
		subpath = "./" + strings.TrimPrefix(spec, pkgName+"/")
	}
	subpathNoJS := "."
	if specNoJS != pkgName {
		subpathNoJS = "./" + strings.TrimPrefix(specNoJS, pkgName+"/")
	}

	// Try to resolve the entry point
	ep := common.ResolvePackageEntry(absPkgDir, subpath, "browser")
	if ep == "" {
		ep = common.ResolvePackageEntry(absPkgDir, subpathNoJS, "browser")
	}
	if ep == "" {
		// Direct file path fallback
		candidate := filepath.Join(absPkgDir, strings.TrimPrefix(spec, pkgName+"/"))
		if _, err := os.Stat(candidate); err == nil {
			ep = candidate
		}
	}
	if ep == "" {
		candidate := filepath.Join(absPkgDir, strings.TrimPrefix(specNoJS, pkgName+"/"))
		if _, err := os.Stat(candidate); err == nil {
			ep = candidate
		}
	}
	if ep == "" {
		// ResolvePackageEntry doesn't handle wildcard exports (e.g. "./*").
		// Fall back to esbuild's resolver via a virtual stdin entry.
		code, err := s.bundleViaStdin(spec, pkgName, pkgDir)
		if err != nil {
			http.NotFound(w, r)
			fmt.Printf("  \033[1;31m[dep-lazy] %s %s → 404 unresolvable (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
		s.onDemandDeps.Store(urlPath, code)
		w.Header().Set("Content-Type", "application/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(code)
		fmt.Printf("  \033[2m[dep-lazy] %s %s → 200 stdin (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Bundle the single entry point with esbuild
	singlePkgMap := map[string]string{pkgName: pkgDir}
	result := api.Build(api.BuildOptions{
		EntryPoints:       []string{ep},
		Bundle:            true,
		Write:             false,
		Format:            api.FormatESModule,
		Platform:          api.PlatformBrowser,
		Target:            api.ESNext,
		LogLevel:          api.LogLevelSilent,
		Define:            s.define,
		IgnoreAnnotations: true,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(singlePkgMap, "browser"),
			common.NodeBuiltinEmptyPlugin(s.moduleMap),
			common.UnknownExternalPlugin(singlePkgMap),
		},
	})

	if len(result.Errors) > 0 || len(result.OutputFiles) == 0 {
		errMsg := "no output"
		if len(result.Errors) > 0 {
			errMsg = result.Errors[0].Text
		}
		http.Error(w, errMsg, http.StatusInternalServerError)
		fmt.Printf("  \033[1;31m[dep-lazy] %s %s → 500 %s (%dms)\033[0m\n",
			r.Method, urlPath, errMsg, time.Since(start).Milliseconds())
		return
	}

	code := fixupOnDemandDep(result.OutputFiles[0].Contents)
	s.onDemandDeps.Store(urlPath, code)

	w.Header().Set("Content-Type", "application/javascript")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(code)
	fmt.Printf("  \033[2m[dep-lazy] %s %s → 200 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

// bundleViaStdin bundles a bare specifier using esbuild's stdin API.
// This lets esbuild's native resolver handle wildcard exports, conditional
// exports, and other resolution edge cases that ResolvePackageEntry misses.
func (s *esmServer) bundleViaStdin(spec, pkgName, pkgDir string) ([]byte, error) {
	contents := fmt.Sprintf("export * from %q;\n", spec)
	singlePkgMap := map[string]string{pkgName: pkgDir}
	result := api.Build(api.BuildOptions{
		Stdin: &api.StdinOptions{
			Contents:   contents,
			ResolveDir: pkgDir,
			Loader:     api.LoaderJS,
		},
		Bundle:   true,
		Write:    false,
		Format:   api.FormatESModule,
		Platform: api.PlatformBrowser,
		Target:   api.ESNext,
		LogLevel: api.LogLevelSilent,
		Define:   s.define,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(singlePkgMap, "browser"),
			common.NodeBuiltinEmptyPlugin(s.moduleMap),
			common.UnknownExternalPlugin(singlePkgMap),
		},
	})
	if len(result.Errors) > 0 || len(result.OutputFiles) == 0 {
		return nil, fmt.Errorf("esbuild failed")
	}
	return fixupOnDemandDep(result.OutputFiles[0].Contents), nil
}
