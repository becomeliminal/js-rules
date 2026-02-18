subinclude("//build_defs:js")

npm_repo(
    name = "peerdep_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo_peerdep",
    entry_point = "index.js",
    deps = [
        "///test/npm_repo_peerdep/peerdep_npm//react",
        "///test/npm_repo_peerdep/peerdep_npm//react-dom",
    ],
    format = "cjs",
    platform = "node",
)
