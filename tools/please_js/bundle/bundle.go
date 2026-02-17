package bundle

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
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
	moduleMap, err := parseModuleConfig(args.ModuleConfig)
	if err != nil {
		return fmt.Errorf("failed to parse moduleconfig: %w", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create node_modules directory with symlinks for each module
	nodeModulesDir := filepath.Join(wd, "node_modules")
	if err := os.MkdirAll(nodeModulesDir, 0755); err != nil {
		return fmt.Errorf("failed to create node_modules: %w", err)
	}

	for moduleName, modulePath := range moduleMap {
		absPath := modulePath
		if !filepath.IsAbs(absPath) {
			absPath = filepath.Join(wd, absPath)
		}

		linkPath := filepath.Join(nodeModulesDir, moduleName)

		// Handle scoped packages: @scope/pkg needs @scope/ directory
		if strings.Contains(moduleName, "/") {
			if err := os.MkdirAll(filepath.Dir(linkPath), 0755); err != nil {
				return fmt.Errorf("failed to create scope directory for %s: %w", moduleName, err)
			}
		}

		// Remove existing symlink if any
		os.Remove(linkPath)

		if err := os.Symlink(absPath, linkPath); err != nil {
			return fmt.Errorf("failed to symlink %s -> %s: %w", linkPath, absPath, err)
		}
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
		Format:      parseFormat(args.Format),
		Platform:    parsePlatform(args.Platform),
		Target:      api.ESNext,
		LogLevel:    api.LogLevelInfo,
		External:    args.External,
		NodePaths:   []string{nodeModulesDir},
		Loader: map[string]api.Loader{
			".js":   api.LoaderJS,
			".jsx":  api.LoaderJSX,
			".ts":   api.LoaderTS,
			".tsx":  api.LoaderTSX,
			".json": api.LoaderJSON,
			".css":  api.LoaderCSS,
			".mjs":  api.LoaderJS,
			".cjs":  api.LoaderJS,
		},
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

func parseModuleConfig(path string) (map[string]string, error) {
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

func parseFormat(f string) api.Format {
	switch f {
	case "cjs":
		return api.FormatCommonJS
	case "iife":
		return api.FormatIIFE
	default:
		return api.FormatESModule
	}
}

func parsePlatform(p string) api.Platform {
	switch p {
	case "node":
		return api.PlatformNode
	default:
		return api.PlatformBrowser
	}
}
