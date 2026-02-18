subinclude("//build_defs:js")

npm_repo(
    name = "cycle_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo_cycle",
    entry_point = "index.js",
    deps = [
        "///test/npm_repo_cycle/cycle_npm//debug",
        "///test/npm_repo_cycle/cycle_npm//ms",
    ],
    format = "cjs",
    platform = "node",
)
