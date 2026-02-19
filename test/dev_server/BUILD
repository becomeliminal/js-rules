subinclude("//build_defs:js")

js_dev_server(
    name = "dev",
    entry_point = "app.jsx",
    deps = [
        "//third_party/js:react",
        "//third_party/js:react-dom",
    ],
    servedir = ".",
    port = 9999,
    format = "esm",
    platform = "browser",
)

gentest(
    name = "dev_server_test",
    test_cmd = "test -x test/dev_server/dev.sh",
    data = [":dev"],
    no_test_output = True,
)
