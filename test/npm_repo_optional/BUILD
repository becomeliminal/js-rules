subinclude("//build_defs:js")

npm_repo(
    name = "optional_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_repo_optional",
    entry_point = "index.js",
    format = "cjs",
    platform = "node",
    deps = [
        "///test/npm_repo_optional/optional_npm//debug",
        "///test/npm_repo_optional/optional_npm//ms",
    ],
)

gentest(
    name = "npm_repo_optional_test",
    test_cmd = "node test/npm_repo_optional/npm_repo_optional.js",
    data = [":npm_repo_optional"],
    no_test_output = True,
)
