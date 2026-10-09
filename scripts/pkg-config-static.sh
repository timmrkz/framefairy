#!/bin/sh
# pkg-config asked for static linking, for the episode's decoder: our
# ffmpeg is built static, and each of its libraries needs the ones it was
# built with named too, libass, freetype and the rest. cgo runs PKG_CONFIG
# without arguments of its own, so this adds the one it needs. See
# cmd/framefairy-frames and the frames target in the Makefile.
exec pkg-config --static "$@"
