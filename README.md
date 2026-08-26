# js-rules

JavaScript rules for the [Please](https://please.build) build system.

Layer 1 of a four-layer stack:

| layer | repo | provides |
|---|---|---|
| 0 | [node-rules](https://github.com/becomeliminal/node-rules) | a pinned, hermetic node |
| 1 | **js-rules** | packages, `node_modules`, running programs |
| 2 | [ts-rules](https://github.com/becomeliminal/ts-rules) | compiling and type-checking TypeScript |
| 3 | [js-bundler-rules](https://github.com/becomeliminal/js-bundler-rules) | esbuild, vite, rollup, webpack, terser |

## The architecture, in one paragraph

The build graph decides **what exists**; node decides **what it means**. Every
rule that runs JavaScript first assembles a real `node_modules` tree from the
target's declared dependencies -- third-party packages from the lockfile,
first-party libraries overlaid by package name -- and then hands it to node,
or to whatever tool the consumer chose. Nothing here reimplements module
resolution: `exports` maps, conditional exports, self-references, subpaths and
extensions are all answered by the tool's own resolver against a filesystem it
recognises. An undeclared import still fails, not because a custom resolver
refuses it, but because the overlay never staged it: native semantics, graph
discipline.

(An earlier version of this repo embedded esbuild as a Go library and
intercepted resolution through a mapping file. That design was replaced by the
one above -- too many packages and tools assume node's filesystem rather than
asking a resolver -- and no trace of it remains in the rules.)

## The tree

`npm_repo` fetches one package, hash-verified, with platform filtering: all 26
esbuild platform packages can be declared honestly, and only the one that can
run here is fetched. `npm_link` assembles a tree in pnpm's store layout --
relative symlinks, each package's dependencies siblings inside its own store
entry, so nothing resolves upward into something undeclared. `hoisted = True`
produces npm's flat layout instead, for tools that cannot tolerate symlinks; a
development server is the case that forces this.

The generated `BUILD` file and the lockfile are sources, and `npm_update` is
the one sanctioned way they change:

```python
npm_update(
    name = "update",
    dir = "third_party/js",
)
```

`plz run //third_party/js:update` drives pnpm through the corepack inside the
pinned node toolchain -- nothing installed -- then translates the lockfile into
`npm_repo`/`npm_link` targets. Policy flags (`--no-dev`, `--lifecycle-hooks`,
`--hoisted-link`) are recorded on the target, so every repin applies the same
policy.

Also supported, each with a test that proves it: npm and yarn lockfiles,
package aliases, workspace packages (`link:`), private registries with
per-scope URLs and secret headers, patches (zero-fuzz, a failed hunk fails the
build), bins a manifest omits, and `public_hoist`-style escape hatches.

### Lifecycle hooks are off by default

Installing a package does not mean executing its code. A package's install
scripts run only when a human names it in `--lifecycle-hooks` (or sets
`run_hooks` on its target), which makes "why did this package execute code at
install time?" a question the build graph can answer.

## The rules

`js_library` packages first-party sources with a manifest, in the same shape a
published package has, split into two named outputs -- `|pkg` (runtime) and
`|types` (declarations) -- so Please invalidates each independently: an
implementation edit does not re-run a consumer that only reads declarations.

`js_binary` / `js_test` run a program or a test against the assembled tree.
Tests use `node:test` with no framework installed, reported per-case to
Please; a `runner` (jest, vitest) replaces the entry point while keeping the
same staging and reporting. `jest_test` is the packaged form. Coverage flows
from node's own coverage through `please_js lcov2cover` into the format
`plz cover` reads, attributed back to source files.

`js_run_binary` is the keystone the other layers build on: run an executable
published by an npm package as a build action, with `node_modules` staged
beside its inputs so the tool's own resolution works unchanged. Compilers,
bundlers and test runners above this layer are wrappers over it.

`npm_package` / `npm_link_package` package first-party code for publishing and
consume it back by name; stamping (`{revision}`, `{describe}`, `{date}`)
requires `stamp = True` and refuses otherwise.

`js_project` writes the tsconfig fragment an editor needs so the TypeScript
language server can find the assembled `node_modules` -- run it once per
checkout, output gitignored.

## The companion tool

`tools/please_js` is a single Go binary driving the mechanical parts: lockfile
translation (`update`), tree assembly (`link`, `overlay`), manifest description
(`describe`), junit and coverage conversion, editor config, tsconfig
validation. It never runs node and never resolves imports -- node is a runtime,
not a compiler, and resolution belongs to the tools.

## Where to look

The tests are the living documentation: every feature above has a fixture
under `test/` that builds it for real, and most were verified by watching the
failure mode first. `plz test //...` runs them all.
