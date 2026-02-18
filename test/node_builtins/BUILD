subinclude("//build_defs:js")

# Browser-platform binary that imports Node.js builtins.
# Verifies that the bundler externalizes them rather than failing.
js_binary(
    name = "node_builtins",
    entry_point = "index.js",
    platform = "browser",
    format = "esm",
)
