subinclude("///go//build_defs:go")

go_library(
    name = "hooks",
    srcs = ["hooks.go"],
    visibility = ["//tools/..."],
)

go_test(
    name = "hooks_test",
    srcs = ["hooks_test.go"],
    external = True,
    deps = [":hooks"],
)
