subinclude("//build_defs:npm")

npm_repo(
    name = "aspect-test_c_1.0.0",
    has_bin = True,
    hashes = ["377b6b21398755f7c7f7e8aa7951b7de34535e62f8c7c3c6629e5042193ab02d"],
    pkg = "@aspect-test/c",
    version = "1.0.0",
)

npm_repo(
    name = "aspect-test_c_2.0.2",
    has_bin = True,
    hashes = ["d0ef956f1b677704bf2a2d24023af1d9445c667ae17065a2549319282fdb418e"],
    pkg = "@aspect-test/c",
    version = "2.0.2",
)

npm_repo(
    name = "aspect-test_e_1.0.0",
    has_bin = True,
    hashes = ["0bfe158a27af8708c4883a99914a7602fc7587a71b9cb434e855472cd6c5cf81"],
    pkg = "@aspect-test/e",
    version = "1.0.0",
)

npm_repo(
    name = "aspect-test_f_1.0.0",
    has_bin = True,
    hashes = ["c7b63f640f2f2dc86a64d9ee7c3250e7290ea524090a1fc8ca691ac4cf8c032b"],
    pkg = "@aspect-test/f",
    version = "1.0.0",
)

npm_link(
    name = "node_modules_lib",
    packages = {
        "@aspect-test/e": ":aspect-test_e_1.0.0",
        "@aspect-test/f": ":aspect-test_f_1.0.0",
    },
    store = [
        ":aspect-test_e_1.0.0",
        ":aspect-test_f_1.0.0",
    ],
    visibility = ["PUBLIC"],
)

npm_link(
    name = "node_modules_lib-dupes",
    packages = {
        "@aspect-test/c": ":aspect-test_c_2.0.2",
        "@aspect-test/c1": ":aspect-test_c_1.0.0",
    },
    store = [
        ":aspect-test_c_1.0.0",
        ":aspect-test_c_2.0.2",
    ],
    visibility = ["PUBLIC"],
)

# A lockfile translated into targets, built, and resolved by node. The alias
# case is the interesting one: one package under two names at two versions.
build_rule(
    name = "dupes_test",
    srcs = ["run.sh"],
    data = [
        "dupes_test.js",
        ":node_modules_lib-dupes",
        "//third_party/node:node",
    ],
    outs = ["dupes_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
