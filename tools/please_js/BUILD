subinclude("///go//build_defs:go")

go_binary(
    name = "please_js",
    srcs = ["main.go"],
    visibility = ["PUBLIC"],
    deps = [
        "//third_party/go:go-flags",
        "//tools/please_js/generate",
        "//tools/please_js/hooks",
        "//tools/please_js/junit",
        "//tools/please_js/lockfile",
        "//tools/please_js/store",
    ],
)
