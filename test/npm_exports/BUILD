subinclude("//build_defs:js")

npm_repo(
    name = "exports_npm",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
)

js_binary(
    name = "npm_exports",
    entry_point = "index.js",
    deps = [
        "///test/npm_exports/exports_npm//devlop",
    ],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "npm_exports_test",
    test_cmd = "node test/npm_exports/npm_exports.js",
    data = [":npm_exports"],
    no_test_output = True,
)
