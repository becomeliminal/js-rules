subinclude("//build_defs:js")

# The "sideEffects" field, which is how a library tells a bundler which of its
# files are loaded for what they do. Stated on the rule, written into the
# generated manifest -- and into the declarations twin's, which is the one
# that wins when the two are merged.
js_library(
    name = "styled",
    package = "@test/styled",
    srcs = ["index.js"],
    data = ["theme.css"],
    side_effects = ["**/*.css"],
)

js_library(
    name = "pure",
    package = "@test/pure",
    srcs = ["index.js"],
    side_effects = False,
)

# Unset: the field is not written, and a bundler assumes the safe thing.
js_library(
    name = "unstated",
    package = "@test/unstated",
    srcs = ["index.js"],
)

js_test(
    name = "sideeffects_test",
    srcs = ["sideeffects.test.js"],
    data = [
        ":pure",
        ":styled",
        ":unstated",
    ],
)
