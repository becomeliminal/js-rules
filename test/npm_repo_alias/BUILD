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

gentest(
    name = "npm_repo_alias_test",
    test_cmd = "node test/npm_repo_alias/npm_repo_alias.js",
    data = [":npm_repo_alias"],
    no_test_output = True,
)
