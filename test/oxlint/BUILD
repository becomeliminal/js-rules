subinclude("//build_defs:oxlint")

js_lint(
    name = "clean",
    srcs = [
        "clean.js",
        "clean.ts",
    ],
    config = ".oxlintrc.json",
)

js_lint(
    name = "violation",
    srcs = ["violation.js"],
    config = ".oxlintrc.json",
)

js_lint(
    name = "bad_config",
    srcs = ["clean.js"],
    config = "bad-config.json",
)

js_lint(
    name = "unsupported",
    srcs = ["unsupported.txt"],
    config = ".oxlintrc.json",
)

js_lint(
    name = "npm_runner_clean",
    srcs = ["clean.js"],
    config = "eslint.config.mjs",
    runner = "eslint|eslint",
    node_modules = "//third_party/js/eslint:node_modules",
)

js_lint(
    name = "npm_runner_violation",
    srcs = ["violation.js"],
    config = "eslint.config.mjs",
    runner = "eslint|eslint",
    node_modules = "//third_party/js/eslint:node_modules",
)
