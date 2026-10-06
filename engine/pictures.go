package engine

import "errors"

// Pictures decodes the picture of an episode for the video preview from
// its samples, the bytes as they lie in the file, one at a time in the
// order they decode, for a file the webview cannot decode itself. It is
// the system's own decoder in the app's own process, open for as long as
// the preview needs it: no program started, no file index read again, no
// session made again for every jump. The page reads the file and knows its
// samples, see frontend/src/lib/frames/mp4.ts, so this needs nothing of
// the file but what the page hands it.
//
// Every frame comes out scaled to Size in 8-bit NV12: the light, a byte a
// pixel, then the colour, two bytes for every two by two pixels.
//
// Only macOS has one, VideoToolbox. Elsewhere, and for a file it will not
// take, OpenPictures says ErrNoPictureDecoder and the preview takes its
// frames from ffmpeg instead, see PreviewFrames.
type Pictures interface {
	Size() (width, height int)
	// Decode decodes one sample, and with keep answers its frame. A
	// sample that makes no frame of its own answers none.
	Decode(sample []byte, microseconds int64, keep bool) ([]byte, error)
	Close()
}

// ErrNoPictureDecoder is what OpenPictures says where there is no
// decoder for the picture.
var ErrNoPictureDecoder = errors.New("there is no system decoder here for this picture")

// OpenPictures opens a decoder for a track of this codec, as the file
// names it (avc1, avc3, hvc1, hev1), described by its avcC or hvcC box,
// with the picture's own size, putting out frames of width by height.
func OpenPictures(codec string, config []byte, codedWidth, codedHeight, width, height int) (Pictures, error) {
	return openPictures(codec, config, codedWidth, codedHeight, width, height)
}
