package updates

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"time"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// Source is what Wails' updater reads releases from. It reads the channel
// list and offers the build of the channel being followed whenever that
// build is not the one running, newer or not. Moving from one pull request
// to another is a step sideways rather than up, and a channel's version
// only ever says which build it is, not whether it is better.
type Source struct {
	// URL is the channel list's address.
	URL    string
	Client *http.Client
	// Own is the channel the running build came from, empty for a build
	// made by make.
	Own string
	// Picked says which channel was picked in the app, if any.
	Picked func() string
	// Seen is told every list read, so the app can show the channels.
	Seen func(List)
	// Progress is told how far a download has come.
	Progress func(written, total int64)
}

// Name implements updater.Provider.
func (s *Source) Name() string { return "channels" }

// Fetch reads the channel list.
func (s *Source) Fetch(ctx context.Context) (List, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return List{}, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return List{}, fmt.Errorf("the channel list could not be fetched: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return List{}, fmt.Errorf("the channel list answered %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxList+1))
	if err != nil {
		return List{}, fmt.Errorf("the channel list could not be read: %w", err)
	}
	list, err := Parse(data)
	if err != nil {
		return List{}, err
	}
	if s.Seen != nil {
		s.Seen(list)
	}
	return list, nil
}

// Check implements updater.Provider.
func (s *Source) Check(ctx context.Context, req updater.CheckRequest) (*updater.Release, error) {
	list, err := s.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	picked := ""
	if s.Picked != nil {
		picked = s.Picked()
	}
	b, ok := list.Follow(picked, s.Own)
	if !ok || b.Version == req.CurrentVersion {
		return nil, nil
	}
	return Release(b)
}

// Release is a build as Wails' updater takes it.
func Release(b Build) (*updater.Release, error) {
	if err := b.Check(); err != nil {
		return nil, err
	}
	digest, err := hex.DecodeString(b.SHA256)
	if err != nil {
		return nil, err
	}
	sig, err := base64.StdEncoding.DecodeString(b.Signature)
	if err != nil {
		return nil, err
	}
	u, _ := url.Parse(b.URL)
	return &updater.Release{
		Version:     b.Version,
		Channel:     b.Channel,
		Name:        b.Name,
		PublishedAt: b.Published,
		Artifact: updater.Artifact{
			// The extension is what tells the updater to unpack it.
			Filename: path.Base(u.Path),
			Filetype: "zip",
			Size:     b.Size,
		},
		Verification: &updater.Verification{
			DigestAlgo:    "sha256",
			Digest:        digest,
			SignatureAlgo: "ed25519",
			Signature:     sig,
		},
		Metadata: map[string]any{"url": b.URL, "commit": b.Commit},
	}, nil
}

// Download implements updater.Provider. It refuses more bytes than the
// list said there would be, so a server that never stops sending fills
// nothing.
func (s *Source) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	where, _ := r.Metadata["url"].(string)
	if where == "" {
		return errors.New("the build has no address")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, where, nil)
	if err != nil {
		return err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return fmt.Errorf("the build could not be downloaded: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("the build answered %s", resp.Status)
	}
	total := r.Artifact.Size
	body := io.LimitReader(resp.Body, total+1)
	buf := make([]byte, 256<<10)
	var written int64
	for {
		n, rerr := body.Read(buf)
		if n > 0 {
			if written+int64(n) > total {
				return errors.New("the build is larger than the channel list says")
			}
			if _, err := dst.Write(buf[:n]); err != nil {
				return err
			}
			written += int64(n)
			onProgress(written, total)
			if s.Progress != nil {
				s.Progress(written, total)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return fmt.Errorf("the download stopped: %w", rerr)
		}
	}
	if written != total {
		return fmt.Errorf("the download stopped after %d of %d bytes", written, total)
	}
	return nil
}

func (s *Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}
