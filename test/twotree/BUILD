subinclude("//build_defs:js")

# A tree with nothing in it, so the sources below cannot possibly resolve the
# compiler through the tree they resolve their own imports through.
genrule(
    name = "empty_tree",
    outs = ["empty_tree"],
    cmd = "mkdir -p $OUT",
)

# Two trees, which is what a bundler needs and what one node_modules slot
# cannot serve: the tool is an npm package, so are the packages it operates on,
# and a tree cannot hold two entries claiming one name -- while every consumer
# bundling a real application has their own copy of the bundler.
#
# typescript exists only in the tool's tree and @test/tool-lib only in the
# sources' tree, so a build that succeeds is proof that neither resolution walked
# into the other.
js_run_binary(
    name = "compile",
    package = "typescript",
    bin = "tsc",
    tool_modules = "//third_party/js:node_modules",
    node_modules = ":empty_tree",
    deps = ["//test/tool/lib"],
    srcs = ["uses_lib.ts"],
    outs = ["uses_lib.js"],
    args = [
        "uses_lib.ts",
        "--target",
        "es2022",
        "--module",
        "commonjs",
    ],
)

js_test(
    name = "two_trees_test",
    entry_point = ":compile",
    deps = ["//test/tool/lib"],
)
