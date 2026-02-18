subinclude("//build_defs:js")

npm_repo(
    name = "exports_npm",
    package_lock = "package-lock.json",
)

js_binary(
    name = "npm_exports",
    entry_point = "index.js",
    deps = [
        "///test/npm_exports/exports_npm//devlop",
    ],
    format = "cjs",
    platform = "node",
)
