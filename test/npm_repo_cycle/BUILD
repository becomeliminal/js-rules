subinclude("//build_defs:js")

npm_repo(
    name = "cycle_npm",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
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

gentest(
    name = "npm_repo_cycle_test",
    test_cmd = "node test/npm_repo_cycle/npm_repo_cycle.js",
    data = [":npm_repo_cycle"],
    no_test_output = True,
)
