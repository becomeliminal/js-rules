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
			fmt.Printf("  \033[2m[cached] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
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

	// JSON-encode the CSS content for safe embedding in JS
	cssJSON, err := json.Marshal(string(data))
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
