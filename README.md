# JS Rules

JavaScript and TypeScript build rules for the [Please](https://please.build) build system.

Powered by [esbuild](https://esbuild.github.io/). All transpilation (TypeScript, JSX, TSX) and bundling runs through esbuild's Go API, so builds are fast and don't shell out to Node.js.

The key idea is **hermetic builds without node_modules**. Instead of letting npm install a mutable tree of packages on disk, every dependency is fetched as an individually content-addressed tarball and wired together at bundle time through a moduleconfig mechanism (inspired by [go-rules](https://github.com/please-build/go-rules)' importconfig pattern). This means builds are reproducible regardless of what's in your local `node_modules/` — or whether you have one at all.

## Quick Start

### 1. Install Please

If you don't already have [Please](https://please.build), install it:

```bash
curl -s https://get.please.build | bash
```

### 2. Add the plugin

Create `plugins/BUILD`:

```python
plugin_repo(
    name = "js",
    owner = "becomeliminal",
    plugin = "js-rules",
    revision = "<commit sha>",
)
```

### 3. Configure `.plzconfig`

```ini
[Plugin "js"]
Target = //plugins:js
```

### 4. Set up npm dependencies

If you have a `package-lock.json` (from `npm install`), you can generate a subrepo of build targets for all your packages automatically. In whatever package contains your lockfile (often the repo root), add:

```python
subinclude("///js//build_defs:js")

npm_repo(
    name = "npm",
    package_lock = "package-lock.json",
)
```

This reads your lockfile, downloads every package as a tarball from the npm registry, and creates a subrepo you can reference from anywhere in your repo:

```python
deps = [
    "///npm//react",
    "///npm//react-dom",
    "///npm//lodash",
]
```

### 5. Write your first binary

Create `app/BUILD`:

```python
subinclude("///js//build_defs:js")

js_binary(
    name = "app",
    entry_point = "main.tsx",
    deps = [
        "///npm//react",
        "///npm//react-dom",
    ],
    platform = "browser",
)
```

Create `app/main.tsx`:

```tsx
import React from "react";
import { renderToString } from "react-dom/server";

console.log(renderToString(<h1>Hello from Please</h1>));
```

### 6. Build and run

```bash
plz build //app
plz run //app
```

## How Dependencies Work

### The moduleconfig pattern

Every `js_library` and `npm_module` rule produces a `.moduleconfig` file — a simple `name=path` mapping that tells esbuild where to find a package on disk. When `js_binary` bundles your code, it aggregates all moduleconfig files from its transitive dependency tree and passes them to esbuild, which uses them to resolve bare `import` statements like `import React from "react"`.

This is directly inspired by the importconfig pattern in [go-rules](https://github.com/please-build/go-rules): each library declares where it lives, and the binary rule stitches them together at link time. There is no global `node_modules/` directory — every dependency is explicitly declared and content-addressed.

### npm_repo: generating targets from a lockfile

Writing `npm_module` rules by hand for every transitive dependency is tedious. `npm_repo` solves this by reading your `package-lock.json` and generating a subrepo containing an `npm_module` target for every package in the lockfile — including transitive dependencies with correct inter-package `deps`.

When you write:

```python
npm_repo(
    name = "npm",
    package_lock = "package-lock.json",
)
```

Please generates a subrepo named `npm`. You can then depend on any package as `///npm//<package-name>`:

```python
deps = [
    "///npm//react",
    "///npm//@babel/core",
]
```

If your `npm_repo` lives in a subdirectory (e.g. `frontend/BUILD`), the subrepo path includes the package path:

```python
# In frontend/BUILD:
npm_repo(name = "npm", package_lock = "package-lock.json")

# Referenced from anywhere as:
deps = ["///frontend/npm//react"]
```

### Why hermetic?

Traditional JavaScript tooling resolves dependencies at build time by walking the `node_modules/` tree on disk. This is problematic for a few reasons:

- **Non-determinism**: `node_modules/` is mutable. Running `npm install` at different times can produce different trees depending on registry state, even with a lockfile.
- **Implicit dependencies**: Code can accidentally import packages that happen to be hoisted into `node_modules/` without declaring them as deps, which breaks when the hoist layout changes.
- **Cache invalidation**: Build systems can't reliably cache outputs when inputs are a sprawling mutable directory.

js-rules avoids all of this. Each npm package is downloaded as an individual tarball at a pinned version, extracted into its own isolated directory, and registered via moduleconfig. esbuild resolves imports only through explicitly declared moduleconfig mappings — if a dependency isn't declared, the build fails, which is the right behavior.

This is similar in spirit to [Bazel's rules_js](https://github.com/aspect-build/rules_js), which also isolates npm packages rather than relying on a shared `node_modules/` tree.

## Build Rules Reference

### js_library

Compiles JavaScript or TypeScript sources into a reusable library. Produces transpiled output (TS/JSX/TSX -> JS) and a `.moduleconfig` file.

```python
js_library(
    name = "components",
    srcs = ["Button.tsx", "Input.tsx"],
    entry_point = "index.ts",       # default: "index.js"
    deps = ["///npm//react"],
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `srcs` | Source files (.js, .jsx, .ts, .tsx, .json) |
| `entry_point` | Entry point file within the library (default: `"index.js"`) |
| `deps` | Dependencies (other `js_library` or `npm_module` targets) |
| `module_name` | Module name for imports. Defaults to the package path |
| `visibility` | Visibility specification |
| `test_only` | If True, only visible to test rules |

### js_binary

Bundles JavaScript/TypeScript into a single output file using esbuild. Aggregates all moduleconfig files from transitive dependencies to resolve imports.

```python
js_binary(
    name = "app",
    entry_point = "main.tsx",
    deps = [
        "//src/components",
        "///npm//react",
        "///npm//react-dom",
    ],
    platform = "browser",
    format = "esm",
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `entry_point` | Entry point source file (default: `"index.js"`) |
| `srcs` | Additional source files |
| `deps` | Dependencies (`js_library`, `npm_module` targets) |
| `format` | Output format: `esm`, `cjs`, `iife` (default: `"esm"`) |
| `platform` | Target platform: `browser`, `node` (default: `"browser"`) |
| `tsconfig` | Path to `tsconfig.json` for JSX settings, paths, etc. |
| `tailwind_config` | Path to `tailwind.config.js` — enables Tailwind CSS compilation |
| `visibility` | Visibility specification |

When `platform = "node"`, the output gets a `#!/usr/bin/env node` shebang so it's directly executable with `plz run`.

### js_test

Bundles and runs JavaScript tests using Node.js.

```python
js_test(
    name = "math_test",
    srcs = ["math_test.js"],
    deps = ["//src/lib"],
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `srcs` | Test source files |
| `entry_point` | Test entry point. Defaults to first file in `srcs` |
| `deps` | Dependencies |
| `dev_deps` | Development-only dependencies (test frameworks, mocking tools) |
| `tsconfig` | Path to `tsconfig.json` |
| `tailwind_config` | Path to `tailwind.config.js` |
| `timeout` | Test timeout in seconds |
| `flaky` | True to mark the test as flaky, or an integer for reruns |
| `size` | Test size (`enormous`, `large`, `medium`, `small`) |

### js_dev_server

Creates a runnable dev server target with live reload. At build time, aggregates moduleconfigs from dependencies. At runtime (`plz run`), starts an esbuild-powered dev server that watches source files for changes and live-reloads the browser.

```python
js_dev_server(
    name = "dev",
    entry_point = "app.jsx",
    deps = [
        "//src/components",
        "///npm//react",
        "///npm//react-dom",
    ],
    servedir = ".",
    port = 8080,
)
```

Run with:

```bash
plz run //app:dev
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `entry_point` | Entry point source file |
| `srcs` | Additional source files |
| `deps` | Production dependencies |
| `dev_deps` | Development-only dependencies |
| `servedir` | Directory to serve static files from, relative to package (default: `"."`) |
| `port` | HTTP port (default: `8080`) |
| `format` | Output format: `esm`, `cjs`, `iife` (default: `"esm"`) |
| `platform` | Target platform: `browser`, `node` (default: `"browser"`) |
| `tsconfig` | Path to `tsconfig.json` |
| `tailwind_config` | Path to `tailwind.config.js` |

### npm_repo

Creates a subrepo of `npm_module` rules from a `package-lock.json`. This is the recommended way to manage npm dependencies.

```python
npm_repo(
    name = "npm",
    package_lock = "package-lock.json",
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Subrepo name (referenced as `///name//package`) |
| `package_lock` | Path to `package-lock.json` file |
| `no_dev` | Exclude devDependencies from the generated subrepo (default: `False`) |

### npm_module

Downloads an individual npm package and makes it available as a dependency. You usually don't need to write these by hand — `npm_repo` generates them for you. Useful when you only need a handful of packages without a lockfile.

```python
npm_module(
    name = "lodash",
    version = "4.17.23",
)

npm_module(
    name = "react-dom",
    pkg_name = "react-dom",
    version = "18.3.1",
    deps = [":react", ":scheduler"],
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `pkg_name` | npm package name (e.g. `"react"`, `"@babel/core"`). Defaults to `name` |
| `version` | Exact version to fetch (required) |
| `deps` | Dependencies on other `npm_module` targets |
| `entry_point` | Override the package entry point |
| `hashes` | Optional hashes for the download |

### tailwind_toolchain

Downloads the Tailwind CSS standalone CLI binary.

```python
tailwind_toolchain(
    name = "tailwind",
    version = "3.4.17",
)
```

Configure in `.plzconfig`:

```ini
[Plugin "js"]
TailwindTool = //third_party/js:tailwind
```

### tailwind_css

Compiles Tailwind CSS using the standalone CLI. Scans `content_srcs` for utility class names and produces optimized CSS.

```python
tailwind_css(
    name = "styles",
    src = "input.css",
    content_srcs = ["index.html", "src/App.tsx"],
    config = "tailwind.config.js",
)
```

| Parameter | Description |
|-----------|-------------|
| `name` | Name of the rule |
| `src` | Input CSS file (containing `@tailwind` directives) |
| `content_srcs` | Source files to scan for utility classes (HTML, JSX, TSX) |
| `config` | Optional `tailwind.config.js` |
| `minify` | Whether to minify the output (default: `True`) |
| `deps` | Dependencies (e.g. other CSS files) |

Tailwind is also supported directly in `js_binary` and `js_test` via the `tailwind_config` parameter, which compiles Tailwind CSS inline during bundling.

### js_toolchain

Downloads a Node.js SDK and exposes `node`, `npm`, and `npx` entry points. Optional — only needed if you want to pin a specific Node.js version rather than using the system `node`.

```python
js_toolchain(
    name = "node",
    version = "20.11.0",
)
```

Configure in `.plzconfig`:

```ini
[Plugin "js"]
NodeTool = //third_party/js:node|node
```

## Configuration

All configuration goes in the `[Plugin "js"]` section of `.plzconfig`:

```ini
[Plugin "js"]
Target = //plugins:js
; Optional: pin Node.js version (otherwise uses system node)
NodeTool = //third_party/js:node|node
; Optional: enable Tailwind CSS support
TailwindTool = //third_party/js:tailwind
```

| Config | Description | Required |
|--------|-------------|----------|
| `Target` | Plugin target | Yes |
| `PleaseJsTool` | Build label for the `please_js` companion tool | No (has default) |
| `NodeTool` | Build label for Node.js binary (from `js_toolchain`) | No |
| `TailwindTool` | Build label for Tailwind CLI (from `tailwind_toolchain`) | No |

## Monorepo Usage

js-rules works naturally in a monorepo alongside other Please plugins (Go, Rust, etc.). Shared JavaScript libraries live in a common directory and are depended on by any app in the repo — no publishing step, no versioning, just build targets.

```
repo/
  common/js/
    components/
      Button.tsx
      Modal.tsx
      BUILD
    hooks/
      BUILD
  blog/
    app/
      BUILD
  dashboard/
    BUILD
```

The shared libraries use `js_library` with a `module_name` so they can be imported cleanly:

```python
# common/js/components/BUILD
subinclude("///js//build_defs:js")

js_library(
    name = "components",
    srcs = ["Button.tsx", "Modal.tsx"],
    module_name = "common/components",
    deps = ["///npm//react"],
    visibility = ["PUBLIC"],
)
```

Any app in the repo can depend on them directly:

```python
# blog/app/BUILD
subinclude("///js//build_defs:js")

js_binary(
    name = "app",
    entry_point = "main.tsx",
    deps = [
        "//common/js/components",
        "//common/js/hooks",
        "///npm//react",
        "///npm//react-dom",
    ],
    platform = "browser",
)
```

Then in your source code:

```tsx
import { Button } from "common/components";
import { useAuth } from "common/hooks";
```

The `module_name` on the library controls the import path, and the moduleconfig mechanism wires it up at bundle time. No symlinks, no path aliases, no `tsconfig` paths hacks — just explicit deps.

## Examples

The `test/` directory contains working examples for common setups:

| Directory | What it demonstrates |
|-----------|---------------------|
| `test/basic` | Simple `js_binary` with `js_library` deps |
| `test/typescript` | TypeScript compilation |
| `test/npm_simple` | Using `npm_module` directly for a single package |
| `test/npm_repo` | Using `npm_repo` to generate deps from a lockfile |
| `test/react` | React with JSX/TSX — both browser and node builds |
| `test/js_test` | Running tests with `js_test` |
| `test/tailwind` | Tailwind CSS compilation with `tailwind_css` |
| `test/dev_server` | Live-reloading dev server with `js_dev_server` |
