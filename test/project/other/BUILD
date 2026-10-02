subinclude("//build_defs:js", "//build_defs:npm")

# This package.json is a project in the Tree config's workspace; npm_project
# resolves its pins from the lockfile at build time and links them.
npm_project(
    name = "node_modules",
    package_json = "package.json",
    hoisted = True,
)

js_test(
    name = "project_test",
    entry_point = "main.js",
    node_modules = ":node_modules",
)

# The flat layout resolves the same import the same way.
js_test(
    name = "project_hoisted_test",
    entry_point = "main.js",
    node_modules = ":node_modules_hoisted",
)
