subinclude("//build_defs:npm")

# The pins for this workspace are not written into any file: npm_workspace
# translates the committed lockfile into npm_repo targets at build time, in the
# subrepo ///test/subrepo/tree/npm, and Please regenerates them exactly when the
# lockfile changes.
npm_workspace(
    name = "npm",
    subinclude = "///build_defs:npm",
)
