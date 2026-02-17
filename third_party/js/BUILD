subinclude("//build_defs:js")

tailwind_toolchain(
    name = "tailwind",
    version = "3.4.17",
)

npm_module(
    name = "lodash",
    version = "4.17.23",
)

npm_module(
    name = "react",
    version = "18.3.1",
)

npm_module(
    name = "scheduler",
    version = "0.23.2",
)

npm_module(
    name = "react-dom",
    pkg_name = "react-dom",
    version = "18.3.1",
    deps = [":react", ":scheduler"],
)
