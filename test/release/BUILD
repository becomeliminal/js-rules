subinclude("//build_defs:npm")

# The released binary, fetched and verified -- what a consumer repo does
# instead of carrying the Go plugin. The version test below runs it, which is
# the only proof a downloaded binary deserves.
please_js_binary(
    name = "please_js",
    hashes = {
        "darwin_amd64": "9e0ffae07437d3ec85c5c34a6a3ba9dcd2f4ab5c3d87decd06ca8c760942131a",
        "darwin_arm64": "f78762f05689f5c26df5de90c372c934dbc8f43722e03793693b280b9b1b6446",
        "linux_amd64": "72ff56e2ad5a5cabb448e33268cbdbee726d4a0716565f074c04cca806fb5bcb",
        "linux_arm64": "2deb867a302bd757c6bb30db662911163709848a07b9e74b79c7dde4b031defd",
    },
    version = "0.1.0",
)

build_rule(
    name = "release_test",
    srcs = ["check.sh"],
    data = [":please_js"],
    outs = ["release_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
