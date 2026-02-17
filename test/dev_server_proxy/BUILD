subinclude("//build_defs:js")

js_dev_server(
    name = "dev_server_proxy",
    entry_point = "main.js",
    srcs = ["index.html"],
    proxy = {
        "/api": "http://localhost:3001",
    },
)
