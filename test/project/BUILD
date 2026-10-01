subinclude("//build_defs:npm")

# Regenerates the fixture tree. It lives here, one package up, because a run
# rewrites test/project/tree/BUILD wholesale.
npm_update(
    name = "update",
    args = [
        "--hoisted-link",
        "--subinclude",
        "//build_defs:npm",
    ],
    dir = "test/project/tree",
)
