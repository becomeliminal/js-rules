subinclude("//build_defs:js")

# A runner that works on files outside its run directory, as a linter works on
# the repository's: chdir starts it there, and $RUNDIR points it back at what
# the rule staged. tsc is the runner because the tree already holds it.

# Succeeds only if $RUNDIR expands: the project is staged in the run directory,
# and the runner starts in a directory that has none.
js_binary(
    name = "rundir_arg",
    srcs = [
        "project/ok.ts",
        "project/tsconfig.json",
    ],
    chdir = "test/runnerdir/elsewhere",
    runner = "typescript|tsc",
    runner_args = [
        "--project",
        "$RUNDIR/project/tsconfig.json",
    ],
)

# Succeeds only if the runner starts where chdir says: the file it is given by
# a relative name exists there and not in the run directory.
js_binary(
    name = "runner_chdir",
    chdir = "test/runnerdir/elsewhere",
    runner = "typescript|tsc",
    runner_args = [
        "--noEmit",
        "only_here.ts",
    ],
)

gentest(
    name = "runner_chdir_test",
    data = [
        ":rundir_arg",
        ":runner_chdir",
        "elsewhere/only_here.ts",
    ],
    no_test_output = True,
    test_cmd = "test/runnerdir/rundir_arg.sh && test/runnerdir/runner_chdir.sh",
)
