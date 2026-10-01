subinclude("//build_defs:js", "//build_defs:npm")

# This package's package.json is a project in //test/project/tree's workspace
# (the Tree config), and this line is all it takes to get its trees.
npm_project()

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
