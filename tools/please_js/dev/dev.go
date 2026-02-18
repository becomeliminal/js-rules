package dev

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

// Args holds the arguments for the dev subcommand.
type Args struct {
	Entry          string
	ModuleConfig   string
	Servedir       string
	Port           int
	Format         string
	Platform       string
	Define         []string
	Proxy          []string
	EnvFile        string
	EnvPrefix      string
	Tsconfig       string
	TailwindBin    string
	TailwindConfig string
}

// liveReloadBanner is injected into the bundle to enable live reload via SSE.
// Parses the SSE event data and only reloads when output files actually changed.
// Debounced to collapse rapid rebuilds into a single reload.
const liveReloadBanner = `(() => { let t; new EventSource("/esbuild").addEventListener("change", (e) => { try { const d = JSON.parse(e.data); if (!d.added.length && !d.removed.length && !d.updated.length) return; } catch {} clearTimeout(t); t = setTimeout(() => window.location.reload(), 200); }); })();`

// formatSize formats a byte count as a human-readable string.
func formatSize(bytes int) string {
	if bytes >= 1024*1024 {
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
	return fmt.Sprintf("%.0fKB", float64(bytes)/1024)
}

// serverInfo holds serve details for the build timer plugin to print after the first build.
type serverInfo struct {
	port uint16
	ips  []string
}

// sseEvent is the JSON payload sent to clients on rebuild.
type sseEvent struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Updated []string `json:"updated"`
}

// devServer serves built output from memory and static files from disk,
// replacing esbuild's ctx.Serve() to avoid request-triggered rebuilds.
type devServer struct {
	mu          sync.RWMutex
	outputFiles map[string][]byte // URL path ("/main.js") -> contents
	fileHashes  map[string]string // URL path -> SHA-256 hex

	sseMu   sync.Mutex
	clients map[chan sseEvent]struct{}

	outdir        string // absolute, for stripping OutputFile.Path prefix
	servedir      string // absolute, for static file serving
	proxies       map[string]*httputil.ReverseProxy
	proxyPrefixes []string // sorted longest-first for greedy matching
}

// parseProxies converts "prefix=target" strings into reverse proxy instances.
// Each proxy is configured with Vite-equivalent defaults:
//   - changeOrigin: Host header is rewritten to the target (so backends
//     behind virtual hosts or CORS checks see the right origin)
//   - secure=false: TLS certificate verification is skipped (dev servers
//     commonly proxy to localhost HTTPS with self-signed certs)
//   - All headers (including Cookie / Set-Cookie) are forwarded as-is
func parseProxies(specs []string) (map[string]*httputil.ReverseProxy, []string) {
	proxies := make(map[string]*httputil.ReverseProxy, len(specs))
	var prefixes []string
	for _, spec := range specs {
		parts := strings.SplitN(spec, "=", 2)
		if len(parts) != 2 {
			continue
		}
		prefix := strings.TrimSpace(parts[0])
		target := strings.TrimSpace(parts[1])
		u, err := url.Parse(target)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: invalid proxy target %q: %v\n", target, err)
			continue
		}
		proxy := httputil.NewSingleHostReverseProxy(u)

		// changeOrigin: rewrite Host header to the target host so backends
		// behind virtual hosts / CORS checks see the correct origin.
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.Host = u.Host
		}

		// secure=false: skip TLS verification for the proxy target.
		proxy.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		proxies[prefix] = proxy
		prefixes = append(prefixes, prefix)
	}
	// Sort longest-first so /api/v2 matches before /api
	sort.Slice(prefixes, func(i, j int) bool {
		return len(prefixes[i]) > len(prefixes[j])
	})
	return proxies, prefixes
}

func newDevServer(outdir, servedir string, proxySpecs []string) *devServer {
	absOutdir, _ := filepath.Abs(outdir)
	absServedir, _ := filepath.Abs(servedir)
	proxies, proxyPrefixes := parseProxies(proxySpecs)
	return &devServer{
		outputFiles:   make(map[string][]byte),
		fileHashes:    make(map[string]string),
		clients:       make(map[chan sseEvent]struct{}),
		outdir:        absOutdir,
		servedir:      absServedir,
		proxies:       proxies,
		proxyPrefixes: proxyPrefixes,
	}
}

func (s *devServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	urlPath := r.URL.Path

	// SSE endpoint
	if urlPath == "/esbuild" {
		s.handleSSE(w, r)
		return
	}

	// Proxy matching requests to backend services
	for _, prefix := range s.proxyPrefixes {
		if strings.HasPrefix(urlPath, prefix) {
			fmt.Printf("  \033[2m[proxy] %s %s\033[0m\n", r.Method, urlPath)
			s.proxies[prefix].ServeHTTP(w, r)
			return
		}
	}

	// Try built files from in-memory map
	s.mu.RLock()
	data, ok := s.outputFiles[urlPath]
	s.mu.RUnlock()
	if ok {
		ct := mime.TypeByExtension(filepath.Ext(urlPath))
		if ct == "" {
			ct = "application/octet-stream"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(data)))
		w.Write(data)
		fmt.Printf("  \033[2m[req] %s %s \u2192 200 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// Try static file from servedir
	filePath := filepath.Join(s.servedir, filepath.FromSlash(urlPath))
	if info, err := os.Stat(filePath); err == nil && !info.IsDir() {
		http.ServeFile(w, r, filePath)
		fmt.Printf("  \033[2m[req] %s %s \u2192 200 (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	// SPA fallback — serve index.html
	indexPath := filepath.Join(s.servedir, "index.html")
	if _, err := os.Stat(indexPath); err == nil {
		http.ServeFile(w, r, indexPath)
		fmt.Printf("  \033[2m[req] %s %s \u2192 200 fallback (%dms)\033[0m\n",
			r.Method, urlPath, time.Since(start).Milliseconds())
		return
	}

	http.NotFound(w, r)
	fmt.Printf("  \033[2m[req] %s %s \u2192 404 (%dms)\033[0m\n",
		r.Method, urlPath, time.Since(start).Milliseconds())
}

func (s *devServer) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	flusher.Flush()

	ch := make(chan sseEvent, 1)
	s.sseMu.Lock()
	s.clients[ch] = struct{}{}
	s.sseMu.Unlock()

	defer func() {
		s.sseMu.Lock()
		delete(s.clients, ch)
		s.sseMu.Unlock()
	}()

	keepAlive := time.NewTicker(30 * time.Second)
	defer keepAlive.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt := <-ch:
			data, _ := json.Marshal(evt)
			fmt.Fprintf(w, "event: change\ndata: %s\n\n", data)
			flusher.Flush()
		case <-keepAlive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

// onBuildComplete updates the in-memory output map and broadcasts changes via SSE.
func (s *devServer) onBuildComplete(result *api.BuildResult, newHashes map[string]string, changed bool) {
	// Build new output map, converting absolute paths to URL paths
	newOutputFiles := make(map[string][]byte, len(result.OutputFiles))
	for _, f := range result.OutputFiles {
		rel, err := filepath.Rel(s.outdir, f.Path)
		if err != nil {
			rel = filepath.Base(f.Path)
		}
		urlPath := "/" + filepath.ToSlash(rel)
		newOutputFiles[urlPath] = f.Contents
	}

	// Convert absolute-path hashes to URL-path hashes
	newURLHashes := make(map[string]string, len(newHashes))
	for absPath, hash := range newHashes {
		rel, err := filepath.Rel(s.outdir, absPath)
		if err != nil {
			rel = filepath.Base(absPath)
		}
		urlPath := "/" + filepath.ToSlash(rel)
		newURLHashes[urlPath] = hash
	}

	s.mu.Lock()
	oldHashes := s.fileHashes
	s.outputFiles = newOutputFiles
	s.fileHashes = newURLHashes
	s.mu.Unlock()

	if !changed {
		return
	}

	// Compute diff
	var evt sseEvent
	for p := range newURLHashes {
		if _, ok := oldHashes[p]; !ok {
			evt.Added = append(evt.Added, p)
		} else if oldHashes[p] != newURLHashes[p] {
			evt.Updated = append(evt.Updated, p)
		}
	}
	for p := range oldHashes {
		if _, ok := newURLHashes[p]; !ok {
			evt.Removed = append(evt.Removed, p)
		}
	}

	if len(evt.Added) == 0 && len(evt.Removed) == 0 && len(evt.Updated) == 0 {
		return
	}

	// Broadcast to all SSE clients (non-blocking)
	s.sseMu.Lock()
	for ch := range s.clients {
		select {
		case ch <- evt:
		default:
		}
	}
	s.sseMu.Unlock()
}

// buildTimerPlugin measures and prints build/rebuild times with output diagnostics.
// On the first build it also prints the URL block, so that branding appears before URLs.
func buildTimerPlugin(info *serverInfo, server *devServer) api.Plugin {
	return api.Plugin{
		Name: "build-timer",
		Setup: func(build api.PluginBuild) {
			var mu sync.Mutex
			var buildStart time.Time
			var isFirst = true
			var lastFileHashes map[string]string

			build.OnStart(func() (api.OnStartResult, error) {
				mu.Lock()
				buildStart = time.Now()
				mu.Unlock()
				return api.OnStartResult{}, nil
			})

			build.OnEnd(func(result *api.BuildResult) (api.OnEndResult, error) {
				mu.Lock()
				elapsed := time.Since(buildStart)
				first := isFirst
				isFirst = false

				// Content-addressed output hashing: only consider a rebuild
				// meaningful if the actual output bytes changed. Per-file
				// comparison avoids false positives from OutputFiles reordering.
				newHashes := make(map[string]string, len(result.OutputFiles))
				for _, f := range result.OutputFiles {
					fh := sha256.Sum256(f.Contents)
					newHashes[f.Path] = hex.EncodeToString(fh[:])
				}
				changed := len(newHashes) != len(lastFileHashes)
				if !changed {
					for path, hash := range newHashes {
						if lastFileHashes[path] != hash {
							changed = true
							break
						}
					}
				}
				prevHashes := lastFileHashes
				lastFileHashes = newHashes
				mu.Unlock()

				ms := elapsed.Milliseconds()

				// Calculate output stats
				totalSize := 0
				for _, f := range result.OutputFiles {
					totalSize += len(f.Contents)
				}
				numFiles := len(result.OutputFiles)

				if first {
					if len(result.Errors) == 0 {
						// Branding line
						fmt.Printf("\n  \033[1;36mPLEASE_JS\033[0m  ready in \033[1m%d ms\033[0m \033[2m(%s, %d files)\033[0m\n", ms, formatSize(totalSize), numFiles)

						// Metafile analysis — top modules by size (JS bundle only)
						if result.Metafile != "" {
							analysis := api.AnalyzeMetafile(result.Metafile, api.AnalyzeMetafileOptions{})
							sections := strings.Split(strings.TrimSpace(analysis), "\n\n")
							if len(sections) > 0 {
								lines := strings.Split(sections[0], "\n")
								const maxInputs = 10
								for i, line := range lines {
									if i > maxInputs {
										fmt.Printf("   └ \033[2m... and %d more\033[0m\n", len(lines)-1-maxInputs)
										break
									}
									fmt.Println(line)
								}
							}
						}

						// URL block
						fmt.Printf("\n  \033[36m➜\033[0m  \033[1mLocal:\033[0m   http://localhost:\033[1m%d\033[0m/\n", info.port)
						for _, ip := range info.ips {
							fmt.Printf("  \033[36m➜\033[0m  \033[2mNetwork: http://%s:%d/\033[0m\n", ip, info.port)
						}
						fmt.Println()
					} else {
						fmt.Printf("\n  \033[1;36mPLEASE_JS\033[0m  build failed with %d errors\n", len(result.Errors))
					}
				} else {
					// Watch rebuild
					if len(result.Errors) == 0 && changed {
						fmt.Printf("  \033[2m[rebuild]\033[0m \033[1m%d ms\033[0m \033[2m(%s, %d files)\033[0m\n", ms, formatSize(totalSize), numFiles)
						for path, hash := range newHashes {
							if prev, ok := prevHashes[path]; ok && prev != hash {
								fmt.Printf("    \033[33m\u0394 %s\033[0m\n", path)
							}
						}
						for path := range newHashes {
							if _, ok := prevHashes[path]; !ok {
								fmt.Printf("    \033[32m+ %s\033[0m\n", path)
							}
						}
					} else if len(result.Errors) == 0 && !changed {
						fmt.Printf("  \033[2m[rebuild] %d ms (no change)\033[0m\n", ms)
					}
				}

				// Update dev server with build output and broadcast changes
				server.onBuildComplete(result, newHashes, changed)

				return api.OnEndResult{}, nil
			})
		},
	}
}

// getLocalIPs returns non-loopback IPv4 addresses.
func getLocalIPs() []string {
	var ips []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			ips = append(ips, ipnet.IP.String())
		}
	}
	return ips
}

// Run starts the dev server with esbuild watch mode and live reload.
//
// Uses esbuild's ctx.Watch() for file-change detection only. HTTP serving
// is handled by our own net/http server, which serves built output from
// memory and static files from disk. This avoids esbuild's ctx.Serve()
// which triggers rebuilds on every HTTP request.
func Run(args Args) error {
	moduleMap, err := common.ParseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	port := args.Port
	if port == 0 {
		port = 8080
	}

	// Outdir = servedir so built output is served at /<basename>.js.
	servedir := args.Servedir
	if servedir == "" {
		servedir = "."
	}
	outdir := servedir

	server := newDevServer(outdir, servedir, args.Proxy)
	info := &serverInfo{
		port: uint16(port),
		ips:  getLocalIPs(),
	}

	plugins := []api.Plugin{
		common.ModuleResolvePlugin(moduleMap),
		common.RawImportPlugin(),
		buildTimerPlugin(info, server),
	}
	if args.TailwindBin != "" {
		plugins = append(plugins, common.TailwindPlugin(args.TailwindBin, args.TailwindConfig))
	}

	format := common.ParseFormat(args.Format)

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

	opts := api.BuildOptions{
		EntryPoints: []string{args.Entry},
		Outdir:      outdir,
		Bundle:      true,
		Write:       false,
		Format:      format,
		Platform:    common.ParsePlatform(args.Platform),
		Target:      api.ESNext,
		LogLevel:    api.LogLevelWarning,
		Loader:      common.Loaders,
		Plugins:     plugins,
		Banner: map[string]string{
			"js": liveReloadBanner,
		},
		Define:    define,
		Sourcemap: api.SourceMapLinked,
		Metafile:  true,
	}
	if args.Tsconfig != "" {
		opts.Tsconfig = args.Tsconfig
	}
	ctx, ctxErr := api.Context(opts)
	if ctxErr != nil {
		return fmt.Errorf("esbuild context creation failed: %v", ctxErr)
	}

	// Start our HTTP server (replaces esbuild's ctx.Serve)
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

	// Start watching for file changes — triggers initial build which
	// prints the branding line and URL block via the build timer plugin.
	if err := ctx.Watch(api.WatchOptions{}); err != nil {
		return fmt.Errorf("esbuild watch failed: %v", err)
	}

	// Block until Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	ctx.Dispose()
	httpServer.Close()
	return nil
}
