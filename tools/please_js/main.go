package main

import (
	"log"
	"os"

	"github.com/thought-machine/go-flags"

	"tools/please_js/bundle"
	"tools/please_js/dev"
	"tools/please_js/resolve"
	"tools/please_js/transpile"
)

var opts = struct {
	Usage string

	Bundle struct {
		Entry          string   `short:"e" long:"entry" required:"true" description:"Entry point file"`
		Out            string   `short:"o" long:"out" required:"true" description:"Output file"`
		ModuleConfig   string   `short:"m" long:"moduleconfig" description:"Aggregated moduleconfig file"`
		Format         string   `short:"f" long:"format" default:"esm" description:"Output format: esm, cjs, iife"`
		Platform       string   `short:"p" long:"platform" default:"browser" description:"Target platform: browser, node"`
		Target         string   `short:"t" long:"target" default:"esnext" description:"Target ES version"`
		External       []string `long:"external" description:"External packages to exclude from bundle"`
		Tsconfig       string   `long:"tsconfig" description:"Path to tsconfig.json (for JSX settings, paths, etc.)"`
		Define         []string `long:"define" description:"Define substitutions (key=value)"`
		Minify         bool     `long:"minify" description:"Minify output (syntax, whitespace, identifiers)"`
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
		TailwindBin    string   `long:"tailwind-bin" description:"Path to Tailwind CSS binary"`
		TailwindConfig string   `long:"tailwind-config" description:"Path to tailwind.config.js"`
	} `command:"dev" alias:"d" description:"Start dev server with live reload using esbuild"`
}{
	Usage: `
please_js is the companion tool for the JavaScript/TypeScript Please build rules.

It provides four main operations:
  - bundle:    Bundle JS/TS files using esbuild with moduleconfig-based dependency resolution
  - transpile: Transpile individual TS/JSX files to JS without bundling
  - resolve:   Generate npm_module BUILD files from package-lock.json
  - dev:       Start a dev server with live reload
`,
}

var subCommands = map[string]func() int{
	"bundle": func() int {
		if err := bundle.Run(bundle.Args{
			Entry:          opts.Bundle.Entry,
			Out:            opts.Bundle.Out,
			ModuleConfig:   opts.Bundle.ModuleConfig,
			Format:         opts.Bundle.Format,
			Platform:       opts.Bundle.Platform,
			Target:         opts.Bundle.Target,
			External:       opts.Bundle.External,
			Define:         opts.Bundle.Define,
			Minify:         opts.Bundle.Minify,
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
			Tsconfig:       opts.Dev.Tsconfig,
			TailwindBin:    opts.Dev.TailwindBin,
			TailwindConfig: opts.Dev.TailwindConfig,
		}); err != nil {
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
