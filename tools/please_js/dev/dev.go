package dev

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

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

	nodeModulesDir, err := common.SetupNodeModules(moduleMap)
	if err != nil {
		return err
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
		Target:      api.ESNext,
		LogLevel:    api.LogLevelInfo,
		NodePaths:   []string{nodeModulesDir},
		Loader:      common.Loaders,
		Plugins:     []api.Plugin{common.RawImportPlugin()},
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

	// Start watching for file changes — watches the real source files
	// referenced by the entry point, not plz-out copies
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

	fmt.Printf("\n  Dev server running at http://localhost:%d/\n", serveResult.Port)
	fmt.Printf("  Watching %s for changes...\n\n", args.Entry)

	// Block until Ctrl+C
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	fmt.Println("\nShutting down...")
	ctx.Dispose()
	return nil
}
