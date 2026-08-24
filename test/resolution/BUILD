subinclude("//build_defs:npm")

npm_link(
    name = "node_modules",
    # The closure, not just the top-level links: scheduler and both js-tokens
    # are reachable only through other packages' dep symlinks, and a store entry
    # that is not staged leaves a dangling link.
    store = [
        "//third_party/js:js_tokens_3",
        "//third_party/js:js_tokens_4",
        "//third_party/js:loose_envify",
        "//third_party/js:loose_envify_old",
        "//third_party/js:react",
        "//third_party/js:react_dom",
        "//third_party/js:scheduler",
    ],
    packages = {
        "loose-envify": "//third_party/js:loose_envify",
        "loose-envify-old": "//third_party/js:loose_envify_old",
        "react": "//third_party/js:react",
        "react-dom": "//third_party/js:react_dom",
    },
)

# Node resolving a real package graph through a store Please built: leaf
# packages, dep edges, subpath exports, a transitive dep, and two versions of
# one dependency side by side.
build_rule(
    name = "resolve_test",
    srcs = ["run.sh"],
    data = [
        "resolve_test.js",
        ":node_modules",
        "//third_party/node:node",
    ],
    outs = ["resolve_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
