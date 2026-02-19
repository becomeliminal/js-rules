subinclude("//build_defs:js")

js_dev_server(
    name = "dev_server_proxy",
    entry_point = "main.js",
    srcs = ["index.html"],
    proxy = {
        "/api": "http://localhost:3001",
    },
)

gentest(
    name = "dev_server_proxy_test",
    test_cmd = "test -x test/dev_server_proxy/dev_server_proxy.sh",
    data = [":dev_server_proxy"],
    no_test_output = True,
)
