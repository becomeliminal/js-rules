subinclude("//build_defs:js")

npm_repo(
    name = "peerdep_npm",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
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

gentest(
    name = "npm_repo_peerdep_test",
    test_cmd = "node test/npm_repo_peerdep/npm_repo_peerdep.js",
    data = [":npm_repo_peerdep"],
    no_test_output = True,
)
