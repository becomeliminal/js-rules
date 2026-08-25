subinclude("//build_defs:js")

js_test(
    name = "runtime_test",
    entry_point = "main.js",
    chdir = "test/runtime",
    env = {"GIVEN": "a value with $dollars and spaces"},
    fixed_args = [
        "--fixed",
        "with space",
    ],
)

# Passes because the program fails. Without expected_exit_code this is a failing
# test, which is the right default -- a program that exits non-zero has usually
# gone wrong.
js_test(
    name = "expected_failure_test",
    entry_point = "fails.js",
    expected_exit_code = 3,
)

