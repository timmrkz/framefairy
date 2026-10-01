package dispenser

import (
	"context"
	"errors"
	"sync"
	"time"
)

// clock is a time that moves only when a test moves it.
type clock struct {
	mu sync.Mutex
	at time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

// sent is a letter the fake mailer took.
type sent struct {
	to     string
	letter Letter
}

// fakeMail is a mail service that keeps what it was given and fails when
// told to.
type fakeMail struct {
	mu      sync.Mutex
	letters []sent
	us      []string
	fail    func(to string, l Letter) bool
}

func (m *fakeMail) Keys(ctx context.Context, to string, l Letter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil && m.fail(to, l) {
		return errors.New("the mail service is down")
	}
	m.letters = append(m.letters, sent{to, l})
	return nil
}

func (m *fakeMail) Us(ctx context.Context, subject, body string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.us = append(m.us, subject+": "+body)
	return nil
}

func (m *fakeMail) sent() []sent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sent(nil), m.letters...)
}

func (m *fakeMail) warnings() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.us...)
}

func (m *fakeMail) failing(f func(to string, l Letter) bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fail = f
}

// fakeShop is Paddle as far as the dispenser asks it: whose sale is whose.
type fakeShop struct {
	mu    sync.Mutex
	buyer map[string]string // reference to address
	down  bool
}

func (s *fakeShop) sold(ref, email string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.buyer == nil {
		s.buyer = map[string]string{}
	}
	s.buyer[ref] = email
}

func (s *fakeShop) Email(ctx context.Context, ref string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return "", errors.New("Paddle is down")
	}
	e, ok := s.buyer[ref]
	if !ok {
		return "", errors.New("no such transaction")
	}
	return e, nil
}

func (s *fakeShop) Refs(ctx context.Context, email string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return nil, errors.New("Paddle is down")
	}
	var refs []string
	for ref, e := range s.buyer {
		if e == email {
			refs = append(refs, ref)
		}
	}
	return refs, nil
}
