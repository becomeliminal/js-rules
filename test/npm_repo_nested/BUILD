subinclude("//build_defs:js")

npm_repo(
    name = "nested_npm",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
)

js_binary(
    name = "npm_repo_nested",
    entry_point = "index.js",
    deps = [
        "///test/npm_repo_nested/nested_npm//ms",
        "///test/npm_repo_nested/nested_npm//debug",
        "///test/npm_repo_nested/nested_npm//has-flag",
    ],
    format = "cjs",
    platform = "node",
)
