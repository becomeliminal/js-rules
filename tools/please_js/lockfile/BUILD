subinclude("///go//build_defs:go")

go_library(
    name = "lockfile",
    srcs = [
        "documents.go",
        "lockfile.go",
        "npm.go",
        "slice.go",
    ],
    visibility = ["//tools/..."],
    deps = ["//third_party/go:yaml"],
)

go_test(
    name = "lockfile_test",
    srcs = [
        "lockfile_test.go",
        "slice_test.go",
    ],
    external = True,
    deps = [":lockfile"],

)
