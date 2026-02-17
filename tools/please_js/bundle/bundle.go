package bundle

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

// Args holds the arguments for the bundle subcommand.
type Args struct {
	Entry          string
	Out            string
	ModuleConfig   string
	Format         string
	Platform       string
	Target         string
	External       []string
	Tsconfig       string
	TailwindBin    string
	TailwindConfig string
}

// Run bundles JavaScript/TypeScript using esbuild.
// It reads a moduleconfig file to resolve module aliases, then runs esbuild.
func Run(args Args) error {
	// Parse moduleconfig: each line is "module_name=path_to_output_dir"
	moduleMap, err := common.ParseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	// Determine output: if Out has no directory component, ensure parent exists
	outDir := filepath.Dir(args.Out)
	if outDir != "" && outDir != "." {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Configure and run esbuild
	plugins := []api.Plugin{
		common.ModuleResolvePlugin(moduleMap),
		common.RawImportPlugin(),
	}
	if args.TailwindBin != "" {
		plugins = append(plugins, common.TailwindPlugin(args.TailwindBin, args.TailwindConfig))
	}

	opts := api.BuildOptions{
		EntryPoints: []string{args.Entry},
		Outfile:     args.Out,
		Bundle:      true,
		Write:       true,
		Format:      common.ParseFormat(args.Format),
		Platform:    common.ParsePlatform(args.Platform),
		Target:      api.ESNext,
		LogLevel:    api.LogLevelInfo,
		External:    args.External,
		Loader:      common.Loaders,
		Plugins:     plugins,
		Define: map[string]string{
			"process.env.NODE_ENV": `"production"`,
		},
		Sourcemap: api.SourceMapLinked,
	}
	if args.Tsconfig != "" {
		opts.Tsconfig = args.Tsconfig
	}
	result := api.Build(opts)

	if len(result.Errors) > 0 {
		return fmt.Errorf("esbuild bundle failed with %d errors", len(result.Errors))
	}
	return nil
}
