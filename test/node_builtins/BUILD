subinclude("//build_defs:js")

# Browser platform: Node builtins are replaced with empty modules.
# Verifies the bundle succeeds — browsers can't resolve bare "stream" etc,
# so the empty module plugin must intercept them at bundle time.
# Also tests subpath imports like "node:fs/promises" and "stream/web".
js_binary(
    name = "node_builtins",
    entry_point = "index.js",
    platform = "browser",
    format = "esm",
)

# Verify the browser build succeeds (no unresolved node builtin subpaths).
gentest(
    name = "node_builtins_test",
    test_cmd = "test -f test/node_builtins/node_builtins.js",
    data = [":node_builtins"],
    no_test_output = True,
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
