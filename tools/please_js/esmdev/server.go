package esmdev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"tools/please_js/common"
)

// Args holds the arguments for the esm-dev subcommand.
type Args struct {
	Entry        string
	ModuleConfig string
	Servedir     string
	Port         int
	Tsconfig     string
	Define       []string
	Proxy        []string
	EnvFile      string
	EnvPrefix    string
	PrebundleDir string // path to pre-bundled deps dir (skips runtime prebundle)
	Root         string // package root for source file resolution
}

// esmServer serves individual ES modules with on-demand transformation.
type esmServer struct {
	sourceRoot     string // servedir — HTML and static files
	packageRoot    string // package root — source files (JS/TS)
	depCache       map[string][]byte // "/@deps/react.js" → pre-bundled ESM
	onDemandDeps   sync.Map          // lazily-bundled subpath deps (/@deps/... → []byte)
	moduleMap      map[string]string // package name → dir (for on-demand bundling)
	transCache     sync.Map          // abs path → *transformEntry
	importMapJSON  []byte
	clients        map[chan sseEvent]struct{}
	sseMu          sync.Mutex
	proxies        map[string]*httputil.ReverseProxy
	proxyPrefixes  []string
	define         map[string]string
	tsconfig       string
	hasRefresh     bool     // true if react-refresh found in pre-bundled deps
	entryURLPath   string   // entry file URL path (e.g., "/main.jsx") — skip HMR for this
	componentFiles sync.Map // abs path → bool (true if last transform found components)
}

func (s *esmServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	urlPath := r.URL.Path

	// 1. SSE endpoint
	if urlPath == "/__esm_dev_sse" {
		s.handleSSE(w, r)
		return
	}

	// 2. Proxy matching
	for _, prefix := range s.proxyPrefixes {
		if strings.HasPrefix(urlPath, prefix) {
			fmt.Printf("  \033[2m[proxy] %s %s\033[0m\n", r.Method, urlPath)
			s.proxies[prefix].ServeHTTP(w, r)
			return
		}
	}

	// 3. Pre-bundled deps
	if strings.HasPrefix(urlPath, "/@deps/") {
		if data, ok := s.depCache[urlPath]; ok {
			w.Header().Set("Content-Type", "application/javascript")
			w.Header().Set("Cache-Control", "no-cache")
			w.Write(data)
			fmt.Printf("  \033[2m[dep] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
		// On-demand bundling for subpath imports resolved via prefix import map entries
		s.handleDepOnDemand(w, r, urlPath, start)
		return
	}

	// 4. HTML files — inject import map + live reload
	if strings.HasSuffix(urlPath, ".html") || urlPath == "/" {
		s.handleHTML(w, r, start)
		return
	}

	// 5. JS/TS/JSX/TSX source files — on-demand transform
	ext := filepath.Ext(urlPath)
	isSourceExt := ext == ".js" || ext == ".jsx" || ext == ".ts" || ext == ".tsx" || ext == ".mjs"
	if isSourceExt || ext == "" {
		s.handleSource(w, r, urlPath, start)
		return
	}

	// 6. CSS files — serve as JS style injector when imported as ES module.
	// Browsers send Sec-Fetch-Dest: script for `import "./style.css"` and
	// Sec-Fetch-Dest: style for `<link rel="stylesheet">`. The ?module query
	// param serves as a fallback for non-browser clients.
	if ext == ".css" {
		fetchDest := r.Header.Get("Sec-Fetch-Dest")
		if fetchDest == "script" || r.URL.Query().Get("module") != "" {
			s.handleCSSModule(w, r, urlPath, start)
			return
		}
	}

	// 6b. Asset files — serve as JS module when imported as ES module.
	if isAssetExt(ext) {
		fetchDest := r.Header.Get("Sec-Fetch-Dest")
		if fetchDest == "script" || r.URL.Query().Get("module") != "" {
			s.handleAssetModule(w, r, urlPath, start)
			return
		}
	}

	// 7. Static files from servedir or packageRoot
	filePath := filepath.Join(s.sourceRoot, filepath.FromSlash(urlPath))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		fmt.Printf("  \033[2m[static] %s %s → 200 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}
	if s.packageRoot != s.sourceRoot {
		filePath = filepath.Join(s.packageRoot, filepath.FromSlash(urlPath))
		if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
			http.ServeFile(w, r, filePath)
			fmt.Printf("  \033[2m[static] %s %s → 200 (%dms)\033[0m\n",
				r.Method, urlPath, time.Since(start).Milliseconds())
			return
		}
	}

	// 8. SPA fallback → index.html with import map injection
	s.handleHTML(w, r, start)
}

// Run starts the ESM dev server.
func Run(args Args) error {
	moduleMap, err := common.ParseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	port := args.Port
	if port == 0 {
		port = 3000
	}

	servedir := args.Servedir
	if servedir == "" {
		servedir = "."
	}
	absServedir, _ := filepath.Abs(servedir)

	// Compute package root for source file resolution (defaults to servedir)
	absPackageRoot := absServedir
	if args.Root != "" {
		absPackageRoot, _ = filepath.Abs(args.Root)
	}

	// Parse defines and env
	define := common.ParseDefines(args.Define)
	if args.EnvFile != "" {
		envDefines, err := common.LoadEnvFiles(args.EnvFile, "development", args.EnvPrefix)
		if err != nil {
			return fmt.Errorf("failed to load env files: %w", err)
		}
		for k, v := range envDefines {
			if _, ok := define[k]; !ok {
				define[k] = v
			}
		}
	}
	common.MergeEnvDefines(define, "development")

	prebundleStart := time.Now()
	var depCache map[string][]byte
	var importMapJSON []byte

	if args.PrebundleDir != "" {
		// Build-time pre-bundled: load from directory (instant).
		depCache, importMapJSON, err = LoadPrebundleDir(args.PrebundleDir)
		if err != nil {
			return fmt.Errorf("failed to load prebundle dir %s: %w", args.PrebundleDir, err)
		}
		var imData struct{ Imports map[string]string }
		json.Unmarshal(importMapJSON, &imData)
		fmt.Printf("  \033[2mLoaded %d deps from prebundle dir in %dms\033[0m\n",
			len(imData.Imports), time.Since(prebundleStart).Milliseconds())
	} else {
		// Runtime fallback: scan sources and pre-bundle on the fly.
		usedImports := scanSourceImports(absPackageRoot, moduleMap)
		// Always include react-refresh if available (injected by HMR scripts).
		if _, ok := moduleMap["react-refresh"]; ok {
			usedImports["react-refresh"] = true
		}

		cacheKey := prebundleCacheKey(args.ModuleConfig, usedImports)
		cacheDir := filepath.Join(".esm-dev-cache", cacheKey)

		if dc, im, loadErr := loadPrebundleCache(cacheDir); loadErr == nil {
			depCache = dc
			importMapJSON = im
			var imData struct{ Imports map[string]string }
			json.Unmarshal(importMapJSON, &imData)
			fmt.Printf("  \033[2mLoaded %d deps from cache in %dms\033[0m\n",
				len(imData.Imports), time.Since(prebundleStart).Milliseconds())
		} else {
			fmt.Printf("  \033[2mPre-bundling dependencies...\033[0m\n")
			depCache, importMapJSON, err = prebundleDeps(moduleMap, usedImports)
			if err != nil {
				return fmt.Errorf("failed to pre-bundle dependencies: %w", err)
			}
			// Save cache (clean old entries first)
			os.RemoveAll(".esm-dev-cache")
			savePrebundleCache(cacheDir, depCache, importMapJSON)
			var imData struct{ Imports map[string]string }
			json.Unmarshal(importMapJSON, &imData)
			fmt.Printf("  \033[2mPre-bundled %d deps in %dms\033[0m\n",
				len(imData.Imports), time.Since(prebundleStart).Milliseconds())
		}
	}

	// Merge tsconfig path aliases into the import map (lower priority than npm deps)
	if args.Tsconfig != "" {
		if pathAliases := parseTsconfigPaths(args.Tsconfig, absPackageRoot); len(pathAliases) > 0 {
			var imData struct {
				Imports map[string]string `json:"imports"`
			}
			json.Unmarshal(importMapJSON, &imData)
			if imData.Imports == nil {
				imData.Imports = make(map[string]string)
			}
			for alias, target := range pathAliases {
				if _, exists := imData.Imports[alias]; !exists {
					imData.Imports[alias] = target
				}
			}
			importMapJSON, _ = json.Marshal(imData)
		}
	}

	// Parse proxies
	proxies, proxyPrefixes := parseProxies(args.Proxy)

	// Detect react-refresh in pre-bundled deps
	hasRefresh := false
	for urlPath := range depCache {
		if strings.Contains(urlPath, "react-refresh") {
			hasRefresh = true
			break
		}
	}

	// Normalize entry point to URL path relative to packageRoot
	absEntry, _ := filepath.Abs(args.Entry)
	entryRel, _ := filepath.Rel(absPackageRoot, absEntry)
	entryURLPath := "/" + filepath.ToSlash(entryRel)

	server := &esmServer{
		sourceRoot:    absServedir,
		packageRoot:   absPackageRoot,
		depCache:      depCache,
		moduleMap:     moduleMap,
		importMapJSON: importMapJSON,
		clients:       make(map[chan sseEvent]struct{}),
		proxies:       proxies,
		proxyPrefixes: proxyPrefixes,
		define:        define,
		tsconfig:      args.Tsconfig,
		hasRefresh:    hasRefresh,
		entryURLPath:  entryURLPath,
	}

	// Start file watcher
	go server.watchFiles()

	// Start HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: server,
	}
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "HTTP server error: %v\n", err)
			os.Exit(1)
		}
	}()

	// Print banner
	fmt.Printf("\n  \033[1;36mPLEASE_JS ESM\033[0m  dev server ready\n")
	if hasRefresh {
		fmt.Printf("  \033[1;35mHMR\033[0m  React Fast Refresh enabled\n")
	}
	fmt.Printf("\n  \033[36m➜\033[0m  \033[1mLocal:\033[0m   http://localhost:\033[1m%d\033[0m/\n", port)
	for _, ip := range getLocalIPs() {
		fmt.Printf("  \033[36m➜\033[0m  \033[2mNetwork: http://%s:%d/\033[0m\n", ip, port)
	}
	fmt.Println()

	// Block until Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	httpServer.Close()
	return nil
}
