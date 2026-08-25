subinclude("///go//build_defs:go")

go_library(
    name = "generate",
    srcs = [
        "generate.go",
        "platform.go",
        "hash.go",
        "write.go",
    ],
    visibility = ["//tools/..."],
    deps = [
        "//third_party/go:buildtools",
        "//tools/please_js/lockfile",
        "//tools/please_js/store",
    ],
)

go_test(
    name = "generate_test",
    srcs = ["platform_test.go"],
    external = True,
    deps = [":generate"],
)
