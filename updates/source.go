package updates

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sync"
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
	// Key is the public key the build's claim is checked against, see
	// Build.Claim. A build is offered to the updater only once the entry
	// is signed as the build it says it is. Nil checks no claim, which is
	// only for the tests of what does not depend on it.
	Key ed25519.PublicKey
	// Cache is the folder the builds already downloaded are kept in, so
	// a channel picked again has its build at once. Empty keeps nothing.
	// See cache.go.
	Cache string
	// Retries is how long to wait before each new try when the channel
	// list answers 404. Nil is listRetries.
	Retries []time.Duration

	// fetches are the builds on their way into the cache, by SHA-256, see
	// fetch.go.
	fetchMu sync.Mutex
	fetches map[string]*fetching
}

// listRetries are the waits between tries while the channel list answers
// 404, seven seconds in all. The publish workflow puts a new list in the
// place of the old one, and GitHub cannot replace a release file in one
// step, so for a moment there is none. A check that lands in that moment
// waits it out rather than showing an error.
var listRetries = []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}

// statusError is a channel list that answered anything but 200.
type statusError struct {
	code   int
	status string
}

func (e *statusError) Error() string { return "the channel list answered " + e.status }

// Name implements updater.Provider.
func (s *Source) Name() string { return "channels" }

// Fetch reads the channel list, and tries again while it answers 404.
func (s *Source) Fetch(ctx context.Context) (List, error) {
	waits := s.Retries
	if waits == nil {
		waits = listRetries
	}
	for try := 0; ; try++ {
		list, err := s.fetch(ctx)
		var status *statusError
		if !errors.As(err, &status) || status.code != http.StatusNotFound || try == len(waits) {
			return list, err
		}
		select {
		case <-ctx.Done():
			return List{}, err
		case <-time.After(waits[try]):
		}
	}
}

func (s *Source) fetch(ctx context.Context) (List, error) {
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
		return List{}, &statusError{code: resp.StatusCode, status: resp.Status}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxList+1))
	if err != nil {
		return List{}, fmt.Errorf("the channel list could not be read: %w", err)
	}
	list, err := Parse(data)
	if err != nil {
		return List{}, err
	}
	s.prune(list)
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
	if s.Key != nil {
		if err := VerifyClaim(s.Key, b); err != nil {
			return nil, err
		}
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

// Download implements updater.Provider. A build kept in the cache is
// handed over at once. Any other goes into the cache first, and from there
// to the updater, so a download that is waited for no more still arrives,
// see fetch.go.
func (s *Source) Download(ctx context.Context, r *updater.Release, dst io.Writer, onProgress func(written, total int64)) error {
	where, _ := r.Metadata["url"].(string)
	if where == "" {
		return errors.New("the build has no address")
	}
	report := func(written, total int64) {
		onProgress(written, total)
		if s.Progress != nil {
			s.Progress(written, total)
		}
	}
	sum := ""
	if r.Verification != nil {
		sum = hex.EncodeToString(r.Verification.Digest)
	}
	total := r.Artifact.Size
	if found, err := s.fromCache(sum, total, dst, report); found {
		return err
	}
	if f := s.fetchOf(sum, where, total); f != nil {
		return s.await(ctx, f, sum, dst, report)
	}
	return s.stream(ctx, where, total, writerSink{dst}, func(written int64) { report(written, total) })
}

func (s *Source) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return http.DefaultClient
}
