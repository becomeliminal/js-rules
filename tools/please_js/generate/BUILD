subinclude("///go//build_defs:go")

go_library(
    name = "generate",
    srcs = [
        "generate.go",
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
