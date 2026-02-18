subinclude("//build_defs:js")

npm_repo(
    name = "alias_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo_alias",
    entry_point = "index.js",
    platform = "node",
    deps = ["///test/npm_repo_alias/alias_npm//my-ms"],
)
