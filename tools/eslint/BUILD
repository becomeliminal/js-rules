subinclude("//build_defs:js", "//build_defs:npm")

# ESLint over this repository's own JavaScript and TypeScript: `plz lint`.
#
# Also the worked example of a tool that lints a repository rather than its
# own run directory: it starts at the root (chdir), and loads its config from
# beside the tree its plugins resolve through ($RUNDIR).
js_binary(
    name = "eslint",
    srcs = ["eslint.config.mjs"],
    chdir = ".",
    node_modules = "//third_party/js/eslint:node_modules",
    runner = "eslint",
    runner_args = [
        "--config",
        "$RUNDIR/eslint.config.mjs",
        "--max-warnings",
        "0",
    ],
)

# ESLint's tree, repinned: edit third_party/js/eslint/package.json, then
# `plz run //tools/eslint:update`. Public, so the plugins built on this one
# lint themselves with the same tree.
npm_update(
    name = "update",
    dir = "third_party/js/eslint",
)
