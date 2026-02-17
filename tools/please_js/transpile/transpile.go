package transpile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

// Args holds the arguments for the transpile subcommand.
type Args struct {
	OutDir string
	Srcs   []string
}

// Run transpiles individual source files (TS->JS, JSX->JS) without bundling.
func Run(args Args) error {
	if err := os.MkdirAll(args.OutDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	for _, src := range args.Srcs {
		data, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read %s: %w", src, err)
		}

		ext := filepath.Ext(src)
		loader := loaderForExt(ext)

		// Plain JS, JSON, and CSS files are just copied
		if loader == api.LoaderJS || loader == api.LoaderJSON || loader == api.LoaderCSS {
			outPath := filepath.Join(args.OutDir, filepath.Base(src))
			if err := os.WriteFile(outPath, data, 0644); err != nil {
				return fmt.Errorf("failed to write %s: %w", outPath, err)
			}
			continue
		}

		// Transpile TS/TSX/JSX using esbuild Transform API
		result := api.Transform(string(data), api.TransformOptions{
			Loader:      loader,
			Format:      api.FormatESModule,
			Target:      api.ESNext,
			JSX:         api.JSXAutomatic,
			Sourcemap:   api.SourceMapInline,
			SourceRoot:  filepath.Dir(src),
			Sourcefile:  filepath.Base(src),
		})

		if len(result.Errors) > 0 {
			for _, e := range result.Errors {
				if e.Location != nil {
					fmt.Fprintf(os.Stderr, "%s:%d:%d: %s\n", src, e.Location.Line, e.Location.Column, e.Text)
				} else {
					fmt.Fprintf(os.Stderr, "%s: %s\n", src, e.Text)
				}
			}
			return fmt.Errorf("transpilation failed for %s", src)
		}

		outName := strings.TrimSuffix(filepath.Base(src), ext) + ".js"
		outPath := filepath.Join(args.OutDir, outName)
		if err := os.WriteFile(outPath, result.Code, 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", outPath, err)
		}
	}
	return nil
}

func loaderForExt(ext string) api.Loader {
	switch ext {
	case ".ts":
		return api.LoaderTS
	case ".tsx":
		return api.LoaderTSX
	case ".jsx":
		return api.LoaderJSX
	case ".json":
		return api.LoaderJSON
	case ".css":
		return api.LoaderCSS
	default:
		return api.LoaderJS
	}
}
