The programs framefairy ships beside itself, built from source by
`.github/workflows/tools.yml`. These are not the app. The app picks them
up from here when a release is made, and `make` builds its own copies for
everyday work.

**ffmpeg and ffprobe** are LGPL 2.1, built without `--enable-gpl` and
without libx264. The binary says so itself, which is better than us
saying it:

    ffmpeg -L          the licence
    ffmpeg -buildconf  the configure line it was built with

The build refuses to finish if either of those comes out wrong, if libass
is missing, or if the binary names a library from the machine that built
it.

**llama-server** is MIT, from llama.cpp at a pinned tag. It is what runs a
language model on the user's own machine.

**framefairy-tools-source.tar.gz** is the source all of it was built from:
the upstream archives at the pinned versions and the two scripts that
configured them. That is what LGPL asks for, and it is here rather than on
request.

Each `manifest.txt` holds the sha256 of every binary, readable without
unpacking anything. To check that the copy inside a built app is one of
these:

    sh scripts/verify-tools.sh <the folder holding them> <the manifest>

And each archive is signed by GitHub with a build attestation, which
records that this exact file came out of this workflow, from this commit,
on a runner nobody had a shell on:

    gh attestation verify <the archive> --repo {REPO}
