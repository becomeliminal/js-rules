subinclude("//build_defs:js", "//build_defs:npm")

# This package's package.json is a project in //test/project/tree's workspace,
# and this line is all it takes to get its trees. tree is passed only because
# this repo's Tree config points at its own third_party tree.
npm_project(tree = "//test/project/tree")

js_test(
    name = "project_test",
    entry_point = "main.js",
    node_modules = ":node_modules",
)

# The flat layout resolves the same import the same way.
js_test(
    name = "project_hoisted_test",
    entry_point = "main.js",
    node_modules = ":hoisted",
)
