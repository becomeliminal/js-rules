subinclude("//build_defs:js", "//build_defs:npm")

# The same npm_project as any app, pointed at a tree whose pins live in a
# generated subrepo rather than a generated BUILD file.
npm_project(
    name = "node_modules",
    package_json = "package.json",
    tree = "//test/subrepo/tree",
    tree_repo = "npm",
)

js_test(
    name = "subrepo_test",
    entry_point = "main.js",
    node_modules = ":node_modules",
)
