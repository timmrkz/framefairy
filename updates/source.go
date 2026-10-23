package updates

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"slices"
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
	// Copies are the addresses of the two copies of the list, read before
	// URL, see ListCopies. Empty reads URL alone.
	Copies []string
	// Entries is the folder each channel's own entry is in, as
	// channel-<channel>.json, read for the channel followed, see fresher.
	// Empty reads the list alone.
	Entries string
	// Newer is told when the followed channel's own entry is of a newer
	// build than the list's, and is offered in its place.
	Newer func(listed, entry Build)
	// Refused is told when the followed channel's own entry could not be
	// read or was not taken, and why. An entry of the build the list has
	// already, the usual case, is not refused.
	Refused func(channel string, why error)

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

// Fetch reads the channel list: the newer of its two copies, or with
// neither to be had the list itself, tried again while it answers 404.
func (s *Source) Fetch(ctx context.Context) (List, error) {
	// The entry of the channel followed is read beside the list, not
	// after it, so it adds no time to a check. Its read ends with Fetch,
	// so a list that failed takes no entry read on with it.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	entry := s.readEntry(ctx)
	list, err := s.fetchCopies(ctx)
	if err != nil {
		list, err = s.fetchRetrying(ctx)
	}
	if err != nil {
		return List{}, err
	}
	list = s.fresher(list, <-entry)
	s.prune(list)
	if s.Seen != nil {
		s.Seen(list)
	}
	return list, nil
}

// ownEntry is the followed channel's own entry as it was read, see
// readEntry. An empty channel is no entry to read.
type ownEntry struct {
	channel string
	data    []byte
	err     error
}

// readEntry starts reading the followed channel's own entry, and hands it
// over on the channel it returns once it is read.
func (s *Source) readEntry(ctx context.Context) <-chan ownEntry {
	out := make(chan ownEntry, 1)
	picked := ""
	if s.Picked != nil {
		picked = s.Picked()
	}
	wanted := Followed(picked, s.Own)
	if s.Entries == "" || !ValidChannel(wanted) {
		out <- ownEntry{}
		return out
	}
	go func() {
		data, err := s.read(ctx, s.Entries+"channel-"+wanted+".json")
		out <- ownEntry{wanted, data, err}
	}()
	return out
}

// fresher is the list with the followed channel's own entry in the place
// of the list's, when the entry is of a newer build. The publish workflow
// uploads a build and then its entry, and writes the list from every
// entry only after that, in one queue that every build of every channel
// shares. On 20 October the list took two minutes to follow a build of a
// pull request, and Tim picked it in between: the list still named the
// build before, which was kept in the cache, so the app came back as that
// at once, issue #420. The entry is as untrusted as the list and is
// checked the same way, its claim among it before it is installed. An
// entry that cannot be read, or is of the build the list has or an older
// one, leaves the list as it is.
func (s *Source) fresher(list List, entry ownEntry) List {
	if entry.channel == "" {
		return list
	}
	wanted := entry.channel
	at := slices.IndexFunc(list.Channels, func(b Build) bool { return b.Channel == wanted })
	if at < 0 {
		return list
	}
	refuse := func(why error) List {
		if s.Refused != nil {
			s.Refused(wanted, why)
		}
		return list
	}
	if entry.err != nil {
		return refuse(entry.err)
	}
	var b Build
	if err := json.Unmarshal(entry.data, &b); err != nil {
		return refuse(fmt.Errorf("the entry is not a build: %w", err))
	}
	if err := b.Check(); err != nil {
		return refuse(err)
	}
	if b.Channel != wanted {
		return refuse(fmt.Errorf("the entry is of the channel %q", b.Channel))
	}
	listed := list.Channels[at]
	if !b.Published.After(listed.Published) {
		return list
	}
	// What only the list knows stays: the pull request's title now, the
	// newer commit still to be built, unless it is this build's, and
	// whether it merges.
	b.Name, b.Newest, b.Conflict = listed.Name, listed.Newest, listed.Conflict
	b.Newest = b.Behind()
	if s.Newer != nil {
		s.Newer(listed, b)
	}
	list.Channels = slices.Clone(list.Channels)
	list.Channels[at] = b
	return list
}

// fetchCopies reads both copies of the list at once and keeps the one
// written last. The workflow replaces only the older copy each time it
// writes the list, so the other is there the whole time, however long
// GitHub takes to serve the new one: a list replaced at one address was
// missing for a moment each time, and an app that checked then said the
// check did not get through, retries and all. An error only when neither
// copy could be read.
func (s *Source) fetchCopies(ctx context.Context) (List, error) {
	if len(s.Copies) == 0 {
		return List{}, errors.New("the channel list has no copies")
	}
	lists := make([]List, len(s.Copies))
	errs := make([]error, len(s.Copies))
	var wg sync.WaitGroup
	for i, addr := range s.Copies {
		wg.Go(func() { lists[i], errs[i] = s.fetch(ctx, addr) })
	}
	wg.Wait()
	best := -1
	for i := range lists {
		if errs[i] == nil && (best < 0 || lists[i].Written.After(lists[best].Written)) {
			best = i
		}
	}
	if best < 0 {
		return List{}, errors.Join(errs...)
	}
	return lists[best], nil
}

// fetchRetrying reads the list at URL, and tries again while it answers
// 404. It is what a release written before the copies has, and what is
// read while neither copy can be.
func (s *Source) fetchRetrying(ctx context.Context) (List, error) {
	waits := s.Retries
	if waits == nil {
		waits = listRetries
	}
	for try := 0; ; try++ {
		list, err := s.fetch(ctx, s.URL)
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

func (s *Source) fetch(ctx context.Context, addr string) (List, error) {
	data, err := s.read(ctx, addr)
	if err != nil {
		return List{}, err
	}
	return Parse(data)
}

// read reads a file of the release, a copy of the list or an entry, no
// more than MaxList of it, so Parse can tell one that is too large.
func (s *Source) read(ctx context.Context, addr string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, addr, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	resp, err := s.client().Do(req)
	if err != nil {
		return nil, fmt.Errorf("the channel list could not be fetched: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{code: resp.StatusCode, status: resp.Status}
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxList+1))
	if err != nil {
		return nil, fmt.Errorf("the channel list could not be read: %w", err)
	}
	return data, nil
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
