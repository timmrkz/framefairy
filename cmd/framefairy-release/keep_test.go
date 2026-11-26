package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
)

// toolRelease is the release release-tool as a test needs it: its files,
// and the digests GitHub still holds a signature for.
type toolRelease struct {
	files      []asset
	data       map[string][]byte
	signed     map[string]bool
	unsignFail error
	// unsignIgnored is GitHub saying yes to a delete it did not do.
	unsignIgnored bool
}

func (r *toolRelease) assets() ([]asset, error) { return slices.Clone(r.files), nil }

func (r *toolRelease) download(a asset) ([]byte, error) {
	if b, ok := r.data[a.name]; ok {
		return b, nil
	}
	return nil, errNotFound
}

func (r *toolRelease) upload(name string, data []byte) error {
	r.files = slices.DeleteFunc(r.files, func(a asset) bool { return a.name == name })
	r.files = append(r.files, asset{name: name, digest: digestOf(data)})
	r.data[name] = data
	return nil
}

func (r *toolRelease) remove(a asset) error {
	r.files = slices.DeleteFunc(r.files, func(b asset) bool { return b.name == a.name })
	delete(r.data, a.name)
	return nil
}

func (r *toolRelease) unsign(digest string) error {
	if r.unsignFail != nil {
		return r.unsignFail
	}
	if !r.unsignIgnored {
		delete(r.signed, digest)
	}
	return nil
}

func (r *toolRelease) signatures(digest string) (int, error) {
	if r.signed[digest] {
		return 1, nil
	}
	return 0, nil
}

// A build kept in the release takes the signature of every older build
// away, the one it replaces, a build kept under an older name, and one
// GitHub names no digest for, so no job can be handed an older build
// again. Only the new build is left, signed.
func TestKeepingABuildTakesTheSignaturesOfTheOlderOnesAway(t *testing.T) {
	t.Parallel()
	older, keyed, nameless, newer := []byte("older"), []byte("keyed"), []byte("nameless"), []byte("newer")
	r := &toolRelease{
		files: []asset{
			{name: toolName, digest: digestOf(older)},
			{name: "framefairy-release-0123456789abcdef", digest: digestOf(keyed)},
			{name: "framefairy-release-old"},
		},
		data: map[string][]byte{
			toolName:                              older,
			"framefairy-release-0123456789abcdef": keyed,
			"framefairy-release-old":              nameless,
		},
		signed: map[string]bool{
			digestOf(older): true, digestOf(keyed): true, digestOf(nameless): true, digestOf(newer): true,
		},
	}
	if err := keepTool(r, newer, io.Discard); err != nil {
		t.Fatal(err)
	}
	if len(r.files) != 1 || r.files[0].name != toolName || string(r.data[toolName]) != "newer" {
		t.Errorf("the release holds %v, want only the new build", r.files)
	}
	if len(r.signed) != 1 || !r.signed[digestOf(newer)] {
		t.Errorf("signed are %v, want only the new build", r.signed)
	}
}

// The same build kept again keeps its signature: GitHub takes away every
// signature of a digest, the one just made too.
func TestKeepingTheSameBuildAgainKeepsItsSignature(t *testing.T) {
	t.Parallel()
	same := []byte("same")
	r := &toolRelease{
		files:  []asset{{name: toolName, digest: digestOf(same)}},
		data:   map[string][]byte{toolName: same},
		signed: map[string]bool{digestOf(same): true},
	}
	if err := keepTool(r, same, io.Discard); err != nil {
		t.Fatal(err)
	}
	if !r.signed[digestOf(same)] {
		t.Error("the build kept lost its signature")
	}
}

// A signature that cannot be taken away fails the run, so it is seen.
func TestASignatureThatStaysFailsTheRun(t *testing.T) {
	t.Parallel()
	r := &toolRelease{
		files:      []asset{{name: toolName, digest: digestOf([]byte("older"))}},
		data:       map[string][]byte{toolName: []byte("older")},
		signed:     map[string]bool{},
		unsignFail: errors.New("403 Resource not accessible by integration"),
	}
	if err := keepTool(r, []byte("newer"), io.Discard); err == nil {
		t.Error("the run passed with the older build still signed")
	}
}

// A delete GitHub answers but does not do fails the run too: whether a
// signature is gone is asked, not assumed.
func TestASignatureGitHubKeptFailsTheRun(t *testing.T) {
	t.Parallel()
	older := []byte("older")
	r := &toolRelease{
		files:         []asset{{name: toolName, digest: digestOf(older)}},
		data:          map[string][]byte{toolName: older},
		signed:        map[string]bool{digestOf(older): true},
		unsignIgnored: true,
	}
	if err := keepTool(r, []byte("newer"), io.Discard); err == nil {
		t.Error("the run passed with the older build still signed")
	}
}

// A file with no digest that cannot be read is no build to keep beside:
// nothing is uploaded, since its signature would stay.
func TestAnOlderBuildThatCannotBeReadKeepsNothing(t *testing.T) {
	t.Parallel()
	r := &toolRelease{
		files:  []asset{{name: toolName}},
		data:   map[string][]byte{},
		signed: map[string]bool{},
	}
	if err := keepTool(r, []byte("newer"), io.Discard); err == nil {
		t.Fatal("a build was kept beside one whose signature could stay")
	}
	if len(r.files) != 1 || r.files[0].digest != "" {
		t.Errorf("the release changed: %v", r.files)
	}
}

// GitHub keeps the signatures under the account that owns the
// repository, and deletes them by the digest of the file.
func TestASignatureIsTakenAwayByItsDigest(t *testing.T) {
	t.Parallel()
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.EscapedPath())
		if r.URL.Path == "/users/o/attestations/digest/sha256:gone" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	gh := &restGitHub{api: srv.URL, repo: "o/r", token: "t", client: srv.Client(), release: toolBuilds}
	if err := gh.unsign("sha256:abc"); err != nil {
		t.Fatal(err)
	}
	if err := gh.unsign("sha256:gone"); err != nil {
		t.Errorf("a signature already gone: %v", err)
	}
	want := []string{"DELETE /users/o/attestations/digest/sha256:abc", "DELETE /users/o/attestations/digest/sha256:gone"}
	if !slices.Equal(got, want) {
		t.Errorf("asked %v, want %v", got, want)
	}
}

// GitHub lists a digest's signatures under the repository, and says not
// found when there is none.
func TestTheSignaturesOfADigestAreCounted(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/attestations/sha256:two":
			_, _ = w.Write([]byte(`{"attestations":[{"bundle":{}},{"bundle":{}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	gh := &restGitHub{api: srv.URL, repo: "o/r", token: "t", client: srv.Client(), release: toolBuilds}
	for digest, want := range map[string]int{"sha256:two": 2, "sha256:none": 0} {
		if n, err := gh.signatures(digest); err != nil || n != want {
			t.Errorf("%s has %d signatures, %v, want %d", digest, n, err, want)
		}
	}
}
