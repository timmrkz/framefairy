//go:build windows

package signer

import (
	"errors"
	"os"
)

// lock refuses on Windows: the signer runs on macOS or Linux, where the
// record can be held against a second signer.
func lock(*os.File) (func(), error) {
	return nil, errors.New("the signer runs on macOS or Linux")
}
