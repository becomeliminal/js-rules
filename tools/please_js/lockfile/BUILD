subinclude("///go//build_defs:go")

go_library(
    name = "lockfile",
    srcs = [
        "lockfile.go",
        "npm.go",
    ],
    visibility = ["//tools/..."],
    deps = ["//third_party/go:yaml"],
)

go_test(
    name = "lockfile_test",
    srcs = ["lockfile_test.go"],
    external = True,
    deps = [":lockfile"],

)
