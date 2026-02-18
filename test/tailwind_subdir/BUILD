subinclude("//build_defs:js")

js_binary(
    name = "tailwind_subdir",
    entry_point = "main.js",
    srcs = [
        "style.css",
        "src/app.js",
    ],
    tailwind_config = "tailwind.config.js",
    platform = "node",
    format = "cjs",
)

# Verify Tailwind scanned ./src/app.js (relative to config dir) and generated
# the bg-blue-500 utility class. Without the cmd.Dir fix, the relative content
# glob resolves from the repo root and finds nothing.
gentest(
    name = "tailwind_subdir_test",
    test_cmd = "grep -q bg-blue-500 test/tailwind_subdir/tailwind_subdir.css",
    data = [":tailwind_subdir"],
    no_test_output = True,
)
