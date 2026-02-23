subinclude("//build_defs:js")

npm_repo(
    name = "conflict_cycle_npm",
    package_lock = "package-lock.json",
    subinclude_path = "@//build_defs:js",
)

js_binary(
    name = "npm_repo_conflict_cycle",
    entry_point = "index.js",
    deps = [
        "///test/npm_repo_conflict_cycle/conflict_cycle_npm//kind-of",
        "///test/npm_repo_conflict_cycle/conflict_cycle_npm//is-number",
    ],
    format = "cjs",
    platform = "node",
)

gentest(
    name = "npm_repo_conflict_cycle_test",
    test_cmd = "node test/npm_repo_conflict_cycle/npm_repo_conflict_cycle.js",
    data = [":npm_repo_conflict_cycle"],
    no_test_output = True,
)
