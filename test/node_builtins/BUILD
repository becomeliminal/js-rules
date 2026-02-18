subinclude("//build_defs:js")

# Browser platform: Node builtins are replaced with empty modules.
# Verifies the bundle succeeds — browsers can't resolve bare "stream" etc,
# so the empty module plugin must intercept them at bundle time.
js_binary(
    name = "node_builtins",
    entry_point = "index.js",
    platform = "browser",
    format = "esm",
)

# Node platform: Node builtins are externalized (real modules at runtime).
# Verifies that the empty module plugin is NOT applied for node builds,
# so builtins resolve to real Node.js modules that actually work.
js_binary(
    name = "node_builtins_node",
    entry_point = "node_platform.js",
    platform = "node",
    format = "cjs",
)
