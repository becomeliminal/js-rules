package(default_visibility = ["PUBLIC"])

go_toolchain(
    name = "gotool",
    strip_srcs = True,
    version = "1.23.4",
)

go_mod_download(
    name = "esbuild_download",
    module = "github.com/evanw/esbuild",
    version = "v0.27.3",
)

go_module(
    name = "esbuild",
    binary = True,
    download = ":esbuild_download",
    install = ["cmd/esbuild"],
    module = "github.com/evanw/esbuild",
    deps = [":x_sys"],
)

go_module(
    name = "esbuild_api",
    download = ":esbuild_download",
    install = ["pkg/api", "internal/..."],
    module = "github.com/evanw/esbuild",
    deps = [":x_sys"],
)

go_module(
    name = "x_sys",
    install = ["..."],
    module = "golang.org/x/sys",
    version = "v0.28.0",
)

go_module(
    name = "go-flags",
    module = "github.com/thought-machine/go-flags",
    version = "v1.5.0",
    deps = [":x_sys"],
)
