package updates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// What a channel has published, read for the release tool, which keeps
// the libraries of an earlier build as they were signed. It is read the
// way the app reads a build it updates to, with the same limits and
// waits, so there is one reader of what a build is published as.

// PublishedParts are the build a channel's own entry in Entries names,
// and its parts, read against the size and checksum of the zip that
// stores its files, the zip the parts make. A build published without
// parts has none to read.
func (s *Source) PublishedParts(ctx context.Context, channel string) (Build, Parts, error) {
	if s.Entries == "" || !ValidChannel(channel) {
		return Build{}, Parts{}, fmt.Errorf("%q is no channel", channel)
	}
	data, err := s.read(ctx, s.Entries+"channel-"+channel+".json")
	if err != nil {
		return Build{}, Parts{}, err
	}
	var b Build
	if err := json.Unmarshal(data, &b); err != nil {
		return Build{}, Parts{}, fmt.Errorf("the entry is not JSON: %w", err)
	}
	if err := b.Check(); err != nil {
		return Build{}, Parts{}, err
	}
	st, ok := b.AsStored()
	if !ok {
		return Build{}, Parts{}, errors.New("its build was published without parts")
	}
	data, err = s.read(ctx, PartsURL(b.URL))
	if err != nil {
		return Build{}, Parts{}, err
	}
	p, err := ParseParts(data, st.Size, st.SHA256)
	return b, p, err
}

// Piece is the file a part of the build b holds, downloaded packed from
// beside its parts and unpacked, and refused when it is not the file the
// part names.
func (s *Source) Piece(ctx context.Context, b Build, part Part) ([]byte, error) {
	return s.piece(ctx, PartsURL(b.URL), part, func(int64) {})
}
