subinclude("///go//build_defs:go")

package(default_visibility = ["PUBLIC"])

go_toolchain(
    name = "gotool",
    strip_srcs = True,
    version = "1.23.4",
)

# Emitting BUILD files through the same AST Please parses with is what keeps
# generated output stable against plz fmt, and is the reason this tool is Go.
go_module(
    name = "buildtools",
    install = [
        "build",
        "tables",
    ],
    module = "github.com/please-build/buildtools",
    version = "v0.0.0-20240111140234-77ffe55926d9",
)

go_module(
    name = "yaml",
    module = "gopkg.in/yaml.v3",
    version = "v3.0.1",
)

go_module(
    name = "go-flags",
    module = "github.com/thought-machine/go-flags",
    version = "v1.5.0",
    deps = [":x_sys"],
)

go_module(
    name = "x_sys",
    install = ["..."],
    module = "golang.org/x/sys",
    version = "v0.28.0",
)
