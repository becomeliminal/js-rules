package bundle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"

	"tools/please_js/common"
)

// Args holds the arguments for the bundle subcommand.
type Args struct {
	Entry          string
	Out            string
	OutDir         string
	ModuleConfig   string
	Format         string
	Platform       string
	Target         string
	External       []string
	Define         []string
	Minify         bool
	Splitting      bool
	HTML           bool
	EnvFile        string
	EnvPrefix      string
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

	// Configure and run esbuild
	plugins := []api.Plugin{
		common.ModuleResolvePlugin(moduleMap, args.Platform),
		common.RawImportPlugin(),
	}
	// For browser builds, replace unresolved Node.js built-in imports with
	// empty CJS modules. Registered AFTER ModuleResolvePlugin so that npm
	// polyfill packages (e.g. "events", "buffer") are resolved first — only
	// builtins with no npm counterpart get the empty stub.
	if args.Platform != "node" {
		plugins = append(plugins, common.NodeBuiltinEmptyPlugin())
	}
	if args.TailwindBin != "" {
		plugins = append(plugins, common.TailwindPlugin(args.TailwindBin, args.TailwindConfig))
	}

	define := common.ParseDefines(args.Define)
	if args.EnvFile != "" {
		envDefines, err := common.LoadEnvFiles(args.EnvFile, "production", args.EnvPrefix)
		if err != nil {
			return fmt.Errorf("failed to load env files: %w", err)
		}
		for k, v := range envDefines {
			if _, ok := define[k]; !ok {
				define[k] = v
			}
		}
	}
	common.MergeEnvDefines(define, "production")

	opts := api.BuildOptions{
		EntryPoints:       []string{args.Entry},
		Bundle:            true,
		Write:             true,
		Format:            common.ParseFormat(args.Format),
		Platform:          common.ParsePlatform(args.Platform),
		Target:            api.ESNext,
		LogLevel:          api.LogLevelInfo,
		External:          args.External,
		Loader:            common.Loaders,
		Plugins:           plugins,
		Define:            define,
		MinifySyntax:      args.Minify,
		MinifyWhitespace:  args.Minify,
		MinifyIdentifiers: args.Minify,
		Sourcemap:         api.SourceMapLinked,
	}

	if args.Splitting {
		if err := os.MkdirAll(args.OutDir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
		opts.Outdir = args.OutDir
		opts.Splitting = true
		opts.Format = api.FormatESModule
		opts.ChunkNames = "chunk-[hash]"
		opts.AssetNames = "assets/[name]-[hash]"
		opts.Metafile = true
	} else {
		// Single-file output: ensure parent directory exists
		outDir := filepath.Dir(args.Out)
		if outDir != "" && outDir != "." {
			if err := os.MkdirAll(outDir, 0755); err != nil {
				return fmt.Errorf("failed to create output directory: %w", err)
			}
		}
		opts.Outfile = args.Out
	}

	if args.Tsconfig != "" {
		opts.Tsconfig = args.Tsconfig
	}
	result := api.Build(opts)

	if len(result.Errors) > 0 {
		return fmt.Errorf("esbuild bundle failed with %d errors", len(result.Errors))
	}

	if args.Splitting && args.HTML {
		if err := generateHTML(args.OutDir, args.Entry, result.Metafile); err != nil {
			return fmt.Errorf("failed to generate index.html: %w", err)
		}
	}

	return nil
}

// metafileData represents the relevant parts of esbuild's metafile JSON.
type metafileData struct {
	Outputs map[string]struct {
		Imports []struct {
			Path string `json:"path"`
			Kind string `json:"kind"`
		} `json:"imports"`
		EntryPoint string `json:"entryPoint"`
	} `json:"outputs"`
}

// generateHTML parses the esbuild metafile and writes an index.html with
// module script tags and preload hints for shared chunks. The entry parameter
// is the source entry point path (e.g. "src/main.js") used to identify
// the correct output chunk when multiple entry points exist (dynamic imports
// also get entryPoint fields in the metafile).
func generateHTML(outDir string, entry string, metafile string) error {
	var meta metafileData
	if err := json.Unmarshal([]byte(metafile), &meta); err != nil {
		return fmt.Errorf("failed to parse metafile: %w", err)
	}

	prefix := outDir + "/"

	// Find the entry chunk matching the source entry point, and collect CSS files
	var entryPath string
	var cssFiles []string
	for path, output := range meta.Outputs {
		rel := strings.TrimPrefix(path, prefix)
		if output.EntryPoint == entry {
			entryPath = rel
		}
		if strings.HasSuffix(rel, ".css") {
			cssFiles = append(cssFiles, rel)
		}
	}

	if entryPath == "" {
		return fmt.Errorf("no entry point found in metafile")
	}

	// Collect shared chunks from the entry's static imports
	var preloadChunks []string
	if entryOutput, ok := meta.Outputs[prefix+entryPath]; ok {
		for _, imp := range entryOutput.Imports {
			if imp.Kind == "import-statement" {
				rel := strings.TrimPrefix(imp.Path, prefix)
				preloadChunks = append(preloadChunks, rel)
			}
		}
	}

	// Build the HTML
	var b strings.Builder
	b.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n")
	for _, css := range cssFiles {
		fmt.Fprintf(&b, "  <link rel=\"stylesheet\" href=\"%s\">\n", css)
	}
	for _, chunk := range preloadChunks {
		fmt.Fprintf(&b, "  <link rel=\"modulepreload\" href=\"%s\">\n", chunk)
	}
	b.WriteString("</head>\n<body>\n  <div id=\"root\"></div>\n")
	fmt.Fprintf(&b, "  <script type=\"module\" src=\"%s\"></script>\n", entryPath)
	b.WriteString("</body>\n</html>\n")

	return os.WriteFile(filepath.Join(outDir, "index.html"), []byte(b.String()), 0644)
}
