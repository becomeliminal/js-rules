package dev

import (
	"fmt"
	"net"
	"os"
	"os/signal"
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
	Tsconfig       string
	TailwindBin    string
	TailwindConfig string
}

// liveReloadBanner is injected into the bundle to enable live reload via esbuild's SSE endpoint.
const liveReloadBanner = `new EventSource("/esbuild").addEventListener("change", () => window.location.reload());`

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

// buildTimerPlugin measures and prints build/rebuild times with output diagnostics.
// On the first build it also prints the URL block, so that branding appears before URLs.
func buildTimerPlugin(info *serverInfo) api.Plugin {
	return api.Plugin{
		Name: "build-timer",
		Setup: func(build api.PluginBuild) {
			var mu sync.Mutex
			var buildStart time.Time
			var isFirst = true

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

						// Metafile analysis — top modules by size
						if result.Metafile != "" {
							analysis := api.AnalyzeMetafile(result.Metafile, api.AnalyzeMetafileOptions{})
							fmt.Println(analysis)
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
					// Watch rebuild — compact single line
					if len(result.Errors) == 0 {
						fmt.Printf("  \033[2m[rebuild]\033[0m \033[1m%d ms\033[0m \033[2m(%s, %d files)\033[0m\n", ms, formatSize(totalSize), numFiles)
					}
				}
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

// Run starts the esbuild dev server with watch mode and live reload.
//
// The entry point and servedir should be real filesystem paths (not plz-out),
// so that esbuild's watch mode detects edits to actual source files.
// The moduleconfig maps module names to plz-out dependency outputs, which
// are stable during development and don't trigger spurious rebuilds.
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
	// Serve mode keeps output in memory (doesn't write to disk).
	outdir := args.Servedir
	if outdir == "" {
		outdir = "."
	}

	info := &serverInfo{}
	plugins := []api.Plugin{
		common.ModuleResolvePlugin(moduleMap),
		common.RawImportPlugin(),
		buildTimerPlugin(info),
	}
	if args.TailwindBin != "" {
		plugins = append(plugins, common.TailwindPlugin(args.TailwindBin, args.TailwindConfig))
	}

	format := common.ParseFormat(args.Format)

	opts := api.BuildOptions{
		EntryPoints: []string{args.Entry},
		Outdir:      outdir,
		Bundle:      true,
		Write:       false,
		Format:      format,
		Platform:    common.ParsePlatform(args.Platform),
		Target:   api.ESNext,
		LogLevel: api.LogLevelWarning,
		Loader:   common.Loaders,
		Plugins:  plugins,
		Banner: map[string]string{
			"js": liveReloadBanner,
		},
		Define: map[string]string{
			"process.env.NODE_ENV": `"development"`,
		},
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

	// Start the HTTP server first so we know the port before the
	// initial build completes and the plugin prints the URL block.
	serveResult, serveErr := ctx.Serve(api.ServeOptions{
		Servedir: args.Servedir,
		Port:     uint16(port),
		Fallback: "index.html",
	})
	if serveErr != nil {
		return fmt.Errorf("esbuild serve failed: %v", serveErr)
	}

	// Populate server info before watch triggers the first build.
	info.port = serveResult.Port
	info.ips = getLocalIPs()

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
	return nil
}
