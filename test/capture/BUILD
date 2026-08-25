subinclude("//build_defs:js")

# A tool that writes to both streams and fails. Without exit_code_out this would
# be a build failure, which is the right default -- so capturing the status is
# the thing that makes an expected failure expressible.
js_run_binary(
    name = "captured",
    package = "typescript",
    bin = "tsc",
    args = ["--nonsense-flag"],
    exit_code_out = "status",
    stderr = "err.txt",
    stdout = "out.txt",
)

build_rule(
    name = "capture_test",
    srcs = ["check.sh"],
    data = [":captured"],
    outs = ["capture_test"],
    cmd = "cp $SRCS $OUT",
    binary = True,
    test = True,
    no_test_output = True,
    test_cmd = "$TEST",
)
