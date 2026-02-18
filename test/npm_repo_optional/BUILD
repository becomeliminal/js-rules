subinclude("//build_defs:js")

npm_repo(
    name = "optional_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo_optional",
    entry_point = "index.js",
    platform = "node",
    deps = ["///test/npm_repo_optional/optional_npm//ms"],
)
