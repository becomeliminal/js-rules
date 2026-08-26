subinclude("//build_defs:npm")

# The one sanctioned way test/update/thirdparty's lockfile and BUILD change:
#   plz run //test/update:update
# Verified by running it and asserting git sees no diff -- an updater that
# changes committed output on a clean tree is broken, or the tree is.
npm_update(
    name = "update",
    dir = "test/update/thirdparty",
)
