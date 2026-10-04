subinclude("//build_defs:js", "//build_defs:npm")

npm_repo(
    name = "ms_2.1.3",
    hashes = ["f6616e15e530ed552f9daa2d3ce71963947c6bc7c98c9b64fd3e673fd02622c6"],
    pkg = "ms",
    version = "2.1.3",
)

npm_link(
    name = "node_modules",
    lock = "pnpm-lock.yaml",
    packages = [":ms_2.1.3"],
)

# A first-party program run from a directory of linked sources: it imports a
# package from the tree, reads a source by a path relative to where it starts,
# and writes into out/, which is linked to this package's out/ in the
# repository. `plz run //test/rundir:render`; verify_run_dir.sh checks it.
js_binary(
    name = "render",
    entry_point = "render.mjs",
    link_dirs = ["out"],
    link_srcs = ["src/input.txt"],
    node_modules = ":node_modules",
    # Copied, not linked: read once, as a tool's config would be.
    srcs = ["config.json"],
)

# A program whose only srcs are a directory -- a built site a server serves --
# staged whole beside it. `plz run //test/rundir:serve` prints its index.html.
js_binary(
    name = "serve",
    entry_point = "serve.mjs",
    link_srcs = ["serve.mjs"],
    node_modules = ":node_modules",
    srcs = [":site"],
)

genrule(
    name = "site",
    outs = ["site"],
    cmd = "mkdir -p $OUT && echo STAGED_DIR_MARKER > $OUT/index.html",
)
