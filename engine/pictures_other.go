//go:build !darwin

package engine

func openPictures(string, []byte, int, int, int, int) (Pictures, error) {
	return nil, ErrNoPictureDecoder
}
