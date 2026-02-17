subinclude("//build_defs:js")

npm_repo(
    name = "npm_test",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo",
    entry_point = "index.js",
    deps = [
        "///test/npm_repo/npm_test//lodash",
        "///test/npm_repo/npm_test//ms",
    ],
    format = "cjs",
    platform = "node",
)
