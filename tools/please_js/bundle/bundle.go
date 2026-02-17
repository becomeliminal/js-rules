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
	Entry        string
	Out          string
	ModuleConfig string
	Format       string
	Platform     string
	Target       string
	External     []string
}

// Run bundles JavaScript/TypeScript using esbuild.
// It reads a moduleconfig file to set up a node_modules directory with symlinks,
// then runs esbuild with its native resolver.
func Run(args Args) error {
	// Parse moduleconfig: each line is "module_name=path_to_output_dir"
	moduleMap, err := common.ParseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	// Create node_modules directory with symlinks for each module
	nodeModulesDir, err := common.SetupNodeModules(moduleMap)
	if err != nil {
		return err
	}

	// Determine output: if Out has no directory component, ensure parent exists
	outDir := filepath.Dir(args.Out)
	if outDir != "" && outDir != "." {
		if err := os.MkdirAll(outDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Configure and run esbuild
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{args.Entry},
		Outfile:     args.Out,
		Bundle:      true,
		Write:       true,
		Format:      common.ParseFormat(args.Format),
		Platform:    common.ParsePlatform(args.Platform),
		Target:      api.ESNext,
		LogLevel:    api.LogLevelInfo,
		External:    args.External,
		NodePaths:   []string{nodeModulesDir},
		Loader:      common.Loaders,
		Plugins:     []api.Plugin{common.RawImportPlugin()},
		Define: map[string]string{
			"process.env.NODE_ENV": `"production"`,
		},
		Sourcemap: api.SourceMapLinked,
	})

	if len(result.Errors) > 0 {
		return fmt.Errorf("esbuild bundle failed with %d errors", len(result.Errors))
	}
	return nil
}
