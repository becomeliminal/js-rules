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
	Entry        string
	ModuleConfig string
	Servedir     string
	Port         int
	Format       string
	Platform     string
}

// liveReloadBanner is injected into the bundle to enable live reload via esbuild's SSE endpoint.
const liveReloadBanner = `new EventSource("/esbuild").addEventListener("change", () => location.reload());`

// buildTimerPlugin measures and prints build/rebuild times.
func buildTimerPlugin() api.Plugin {
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
				if first {
					// Initial build — branding line
					if len(result.Errors) == 0 {
						fmt.Printf("\n  \033[1;36mPLEASE_JS\033[0m  ready in \033[1m%dms\033[0m\n", ms)
					} else {
						fmt.Printf("\n  \033[1;36mPLEASE_JS\033[0m  build failed with %d errors\n", len(result.Errors))
					}
				} else {
					// Watch rebuild
					if len(result.Errors) == 0 {
						fmt.Printf("  \033[2m[rebuild]\033[0m %dms\n", ms)
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

	ctx, ctxErr := api.Context(api.BuildOptions{
		EntryPoints: []string{args.Entry},
		Outdir:      outdir,
		Bundle:      true,
		Write:       false,
		Format:      common.ParseFormat(args.Format),
		Platform:    common.ParsePlatform(args.Platform),
		Target:   api.ESNext,
		JSX:      api.JSXAutomatic,
		LogLevel: api.LogLevelWarning,
		Loader:   common.Loaders,
		Plugins: []api.Plugin{
			common.ModuleResolvePlugin(moduleMap),
			common.RawImportPlugin(),
			buildTimerPlugin(),
		},
		Banner: map[string]string{
			"js": liveReloadBanner,
		},
		Define: map[string]string{
			"process.env.NODE_ENV": `"development"`,
		},
		Sourcemap: api.SourceMapInline,
	})
	if ctxErr != nil {
		return fmt.Errorf("esbuild context creation failed: %v", ctxErr)
	}

	// Start watching for file changes — triggers initial build which
	// prints the "ready in Xms" line via the build timer plugin
	if err := ctx.Watch(api.WatchOptions{}); err != nil {
		return fmt.Errorf("esbuild watch failed: %v", err)
	}

	// Start the HTTP server — serves built output from memory + static
	// files from servedir (index.html, images, etc.)
	serveResult, serveErr := ctx.Serve(api.ServeOptions{
		Servedir: args.Servedir,
		Port:     uint16(port),
		Fallback: "index.html",
	})
	if serveErr != nil {
		return fmt.Errorf("esbuild serve failed: %v", serveErr)
	}

	fmt.Printf("\n  ➜  \033[1mLocal:\033[0m   http://localhost:%d/\n", serveResult.Port)
	for _, ip := range getLocalIPs() {
		fmt.Printf("  ➜  \033[2mNetwork:\033[0m http://%s:%d/\n", ip, serveResult.Port)
	}
	fmt.Println()

	// Block until Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	ctx.Dispose()
	return nil
}
