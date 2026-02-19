package main

import (
	"log"
	"os"

	"github.com/thought-machine/go-flags"

	"tools/please_js/bundle"
	"tools/please_js/dev"
	"tools/please_js/esmdev"
	"tools/please_js/resolve"
	"tools/please_js/transpile"
)

var opts = struct {
	Usage string

	Bundle struct {
		Entry          string   `short:"e" long:"entry" required:"true" description:"Entry point file"`
		Out            string   `short:"o" long:"out" description:"Output file"`
		OutDir         string   `long:"out-dir" description:"Output directory (for code splitting)"`
		ModuleConfig   string   `short:"m" long:"moduleconfig" description:"Aggregated moduleconfig file"`
		Format         string   `short:"f" long:"format" default:"esm" description:"Output format: esm, cjs, iife"`
		Platform       string   `short:"p" long:"platform" default:"browser" description:"Target platform: browser, node"`
		Target         string   `short:"t" long:"target" default:"esnext" description:"Target ES version"`
		External       []string `long:"external" description:"External packages to exclude from bundle"`
		Tsconfig       string   `long:"tsconfig" description:"Path to tsconfig.json (for JSX settings, paths, etc.)"`
		Define         []string `long:"define" description:"Define substitutions (key=value)"`
		Minify         bool     `long:"minify" description:"Minify output (syntax, whitespace, identifiers)"`
		Splitting      bool     `long:"splitting" description:"Enable code splitting (requires ESM format)"`
		HTML           bool     `long:"html" description:"Generate index.html with module scripts and preload hints"`
		EnvFile        string   `long:"env-file" description:"Base .env file path for auto-discovery"`
		EnvPrefix      string   `long:"env-prefix" default:"PLZ_" description:"Prefix filter for .env variables"`
		TailwindBin    string   `long:"tailwind-bin" description:"Path to Tailwind CSS binary"`
		TailwindConfig string   `long:"tailwind-config" description:"Path to tailwind.config.js"`
	} `command:"bundle" alias:"b" description:"Bundle JavaScript/TypeScript using esbuild"`

	Transpile struct {
		OutDir string `short:"o" long:"out-dir" required:"true" description:"Output directory for transpiled files"`
		Args   struct {
			Sources []string `positional-arg-name:"sources" description:"Source files to transpile"`
		} `positional-args:"true"`
	} `command:"transpile" alias:"t" description:"Transpile individual files (TS->JS, JSX->JS) without bundling"`

	Resolve struct {
		Lockfile       string `short:"l" long:"lockfile" required:"true" description:"Path to package-lock.json"`
		Out            string `short:"o" long:"out" required:"true" description:"Output directory for generated BUILD files"`
		NoDev          bool   `long:"no-dev" description:"Exclude dev dependencies"`
		SubincludePath string `long:"subinclude-path" default:"///js//build_defs:js" description:"Subinclude path for generated BUILD files"`
	} `command:"resolve" alias:"r" description:"Generate npm_module BUILD files from package-lock.json"`

	Dev struct {
		Entry          string `short:"e" long:"entry" required:"true" description:"Entry point file"`
		ModuleConfig   string `short:"m" long:"moduleconfig" description:"Aggregated moduleconfig file"`
		Servedir       string `short:"s" long:"servedir" default:"." description:"Directory to serve static files from"`
		Port           int    `short:"p" long:"port" default:"8080" description:"HTTP port"`
		Format         string `short:"f" long:"format" default:"esm" description:"Output format: esm, cjs, iife"`
		Platform       string `long:"platform" default:"browser" description:"Target platform: browser, node"`
		Tsconfig       string `long:"tsconfig" description:"Path to tsconfig.json (for JSX settings, paths, etc.)"`
		Define         []string `long:"define" description:"Define substitutions (key=value)"`
		Proxy          []string `long:"proxy" description:"Proxy rules (prefix=target)"`
		EnvFile        string   `long:"env-file" description:"Base .env file path for auto-discovery"`
		EnvPrefix      string   `long:"env-prefix" default:"PLZ_" description:"Prefix filter for .env variables"`
		TailwindBin    string   `long:"tailwind-bin" description:"Path to Tailwind CSS binary"`
		TailwindConfig string   `long:"tailwind-config" description:"Path to tailwind.config.js"`
	} `command:"dev" alias:"d" description:"Start dev server with live reload using esbuild"`

	EsmDev struct {
		Entry        string   `short:"e" long:"entry" required:"true" description:"Entry point file"`
		ModuleConfig string   `short:"m" long:"moduleconfig" description:"Aggregated moduleconfig file"`
		Servedir     string   `short:"s" long:"servedir" default:"." description:"Directory to serve static files from"`
		Port         int      `short:"p" long:"port" default:"3000" description:"HTTP port"`
		Tsconfig     string   `long:"tsconfig" description:"Path to tsconfig.json"`
		Define       []string `long:"define" description:"Define substitutions (key=value)"`
		Proxy        []string `long:"proxy" description:"Proxy rules (prefix=target)"`
		EnvFile      string   `long:"env-file" description:"Base .env file path for auto-discovery"`
		EnvPrefix    string   `long:"env-prefix" default:"PLZ_" description:"Prefix filter for .env variables"`
		PrebundleDir string   `long:"prebundle-dir" description:"Path to pre-bundled deps directory (skips runtime prebundle)"`
		Root         string   `long:"root" description:"Package root directory for source file resolution"`
	} `command:"esm-dev" description:"Start ESM dev server with native import maps"`

	Prebundle struct {
		ModuleConfig string `short:"m" long:"moduleconfig" required:"true" description:"Aggregated moduleconfig file"`
		Out          string `short:"o" long:"out" required:"true" description:"Output directory for pre-bundled deps"`
	} `command:"prebundle" description:"Pre-bundle all npm dependencies for ESM dev server"`

	PrebundlePkg struct {
		ModuleConfig string `short:"m" long:"moduleconfig" required:"true" description:"Moduleconfig for a single package"`
		Out          string `short:"o" long:"out" required:"true" description:"Output directory for pre-bundled package"`
	} `command:"prebundle-pkg" description:"Pre-bundle a single npm package for ESM dev server"`

	MergeImportmaps struct {
		Out          string `short:"o" long:"out" required:"true" description:"Output importmap.json path"`
		ModuleConfig string `short:"m" long:"moduleconfig" description:"Moduleconfig for resolving missing transitive deps"`
		DepsDir      string `short:"d" long:"deps-dir" description:"Combined deps directory to scan for missing imports"`
		Args         struct {
			Files []string `positional-arg-name:"files" description:"importmap.json files to merge"`
		} `positional-args:"true"`
	} `command:"merge-importmaps" description:"Merge multiple importmap.json files into one"`
}{
	Usage: `
please_js is the companion tool for the JavaScript/TypeScript Please build rules.

It provides these main operations:
  - bundle:           Bundle JS/TS files using esbuild with moduleconfig-based dependency resolution
  - transpile:        Transpile individual TS/JSX files to JS without bundling
  - resolve:          Generate npm_module BUILD files from package-lock.json
  - dev:              Start a dev server with live reload
  - esm-dev:          Start ESM dev server with native import maps
  - prebundle:        Pre-bundle all npm dependencies for ESM dev server
  - prebundle-pkg:    Pre-bundle a single npm package for ESM dev server
  - merge-importmaps: Merge multiple importmap.json files into one
`,
}

var subCommands = map[string]func() int{
	"bundle": func() int {
		if err := bundle.Run(bundle.Args{
			Entry:          opts.Bundle.Entry,
			Out:            opts.Bundle.Out,
			OutDir:         opts.Bundle.OutDir,
			ModuleConfig:   opts.Bundle.ModuleConfig,
			Format:         opts.Bundle.Format,
			Platform:       opts.Bundle.Platform,
			Target:         opts.Bundle.Target,
			External:       opts.Bundle.External,
			Define:         opts.Bundle.Define,
			Minify:         opts.Bundle.Minify,
			Splitting:      opts.Bundle.Splitting,
			HTML:           opts.Bundle.HTML,
			EnvFile:        opts.Bundle.EnvFile,
			EnvPrefix:      opts.Bundle.EnvPrefix,
			Tsconfig:       opts.Bundle.Tsconfig,
			TailwindBin:    opts.Bundle.TailwindBin,
			TailwindConfig: opts.Bundle.TailwindConfig,
		}); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"transpile": func() int {
		if err := transpile.Run(transpile.Args{
			OutDir: opts.Transpile.OutDir,
			Srcs:   opts.Transpile.Args.Sources,
		}); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"resolve": func() int {
		if err := resolve.Run(resolve.Args{
			Lockfile:       opts.Resolve.Lockfile,
			Out:            opts.Resolve.Out,
			NoDev:          opts.Resolve.NoDev,
			SubincludePath: opts.Resolve.SubincludePath,
		}); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"dev": func() int {
		if err := dev.Run(dev.Args{
			Entry:          opts.Dev.Entry,
			ModuleConfig:   opts.Dev.ModuleConfig,
			Servedir:       opts.Dev.Servedir,
			Port:           opts.Dev.Port,
			Format:         opts.Dev.Format,
			Platform:       opts.Dev.Platform,
			Define:         opts.Dev.Define,
			Proxy:          opts.Dev.Proxy,
			EnvFile:        opts.Dev.EnvFile,
			EnvPrefix:      opts.Dev.EnvPrefix,
			Tsconfig:       opts.Dev.Tsconfig,
			TailwindBin:    opts.Dev.TailwindBin,
			TailwindConfig: opts.Dev.TailwindConfig,
		}); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"esm-dev": func() int {
		if err := esmdev.Run(esmdev.Args{
			Entry:        opts.EsmDev.Entry,
			ModuleConfig: opts.EsmDev.ModuleConfig,
			Servedir:     opts.EsmDev.Servedir,
			Port:         opts.EsmDev.Port,
			Tsconfig:     opts.EsmDev.Tsconfig,
			Define:       opts.EsmDev.Define,
			Proxy:        opts.EsmDev.Proxy,
			EnvFile:      opts.EsmDev.EnvFile,
			EnvPrefix:    opts.EsmDev.EnvPrefix,
			PrebundleDir: opts.EsmDev.PrebundleDir,
			Root:         opts.EsmDev.Root,
		}); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"prebundle": func() int {
		if err := esmdev.PrebundleAll(opts.Prebundle.ModuleConfig, opts.Prebundle.Out); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"prebundle-pkg": func() int {
		if err := esmdev.PrebundlePkg(opts.PrebundlePkg.ModuleConfig, opts.PrebundlePkg.Out); err != nil {
			log.Fatal(err)
		}
		return 0
	},
	"merge-importmaps": func() int {
		if err := esmdev.MergeImportmaps(opts.MergeImportmaps.Args.Files, opts.MergeImportmaps.Out,
			opts.MergeImportmaps.ModuleConfig, opts.MergeImportmaps.DepsDir); err != nil {
			log.Fatal(err)
		}
		return 0
	},
}

func main() {
	p := flags.NewParser(&opts, flags.Default)
	cmd, err := p.Parse()
	if err != nil {
		os.Exit(1)
	}
	_ = cmd
	if p.Active == nil {
		p.WriteHelp(os.Stderr)
		os.Exit(1)
	}
	os.Exit(subCommands[p.Active.Name]())
}
