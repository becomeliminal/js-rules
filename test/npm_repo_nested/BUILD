subinclude("//build_defs:js")

npm_repo(
    name = "nested_npm",
    package_lock = "package-lock.json",
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

gentest(
    name = "npm_repo_nested_test",
    test_cmd = "node test/npm_repo_nested/npm_repo_nested.js",
    data = [":npm_repo_nested"],
    no_test_output = True,
)
