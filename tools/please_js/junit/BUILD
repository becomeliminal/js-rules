subinclude("///go//build_defs:go")

go_library(
    name = "junit",
    srcs = [
        "editor.go",
        "junit.go",
        "lcov.go",
    ],
    visibility = ["//tools/..."],
)

go_test(
    name = "junit_test",
    srcs = [
        "junit_test.go",
        "lcov_test.go",
    ],
    external = True,
    deps = [":junit"],
)
