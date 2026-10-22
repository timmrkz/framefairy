# Gives every file git tracks a time made from its content, for Go's test
# cache, which knows a file a test read by its size and time. The same
# content then has the same time on any checkout, and other content
# another, whatever the history did: the merge commit CI checks out on a
# pull request, an amend, a rebase. Times from the history could not tell
# those apart, see docs/BUILD.md.
#
#   git ls-files -s -z | python3 -I scripts/blob-times.py
import os
import sys

for entry in sys.stdin.buffer.read().split(b"\0"):
    if not entry:
        continue
    meta, path = entry.split(b"\t", 1)
    mode, blob = meta.split()[:2]
    if mode == b"160000":  # a submodule has no file of its own
        continue
    # Seven digits of the hash, a time between 2000 and 2008.
    t = 946684800 + int(blob[:7], 16)
    os.utime(path, (t, t), follow_symlinks=False)
