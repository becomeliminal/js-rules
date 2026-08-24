subinclude("//build_defs:npm")

# The list form makes a genuine collision visible: both entries are the package
# loose-envify, so only one can be imported under that name. Rebinding the older
# one is what npm aliasing is for.
npm_alias(
    name = "loose_envify_old_alias",
    pkg = "loose-envify-old",
    of = "loose_envify_old",
)

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
    packages = [
        "//third_party/js:loose_envify",
        "//third_party/js:react",
        "//third_party/js:react_dom",
        ":loose_envify_old_alias",
    ],
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
