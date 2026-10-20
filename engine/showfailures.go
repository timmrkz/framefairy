package engine

import (
	"errors"
	"sync/atomic"
)

// TEMPORARY, issue #311, taken out before PR #412 is merged. On the build
// of PR #412 every search fails at once with the next of the failures a
// search now says in words, so Tim sees each in the row of a search
// without a key, the network or a wait. Only the app sets it, and only on
// that build.
var ShowFailures bool

var failuresShown atomic.Int64

func nextFailure(source string) error {
	a := providers[0]
	all := []error{
		busyErr(a, "Overloaded"),
		unreachable(a, errors.New("dial tcp: lookup api.anthropic.com: no such host")),
		refusedKey(a, "the keychain"),
		apiRefused(a, 400, `{"type":"error","error":{"type":"invalid_request_error","message":"Your credit balance is too low to access the Anthropic API. Please go to Plans & Billing to upgrade or purchase credits."}}`),
		saidErr("%s did not answer within 10 minutes. Try again in a few minutes.", a.Title),
		videoGone(source),
	}
	n := failuresShown.Add(1) - 1
	return all[n%int64(len(all))]
}
