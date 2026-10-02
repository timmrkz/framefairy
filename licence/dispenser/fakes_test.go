package dispenser

import (
	"context"
	"errors"
	"slices"
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

// fakeShop is Paddle as far as the dispenser asks it: its sales as they
// stand, and when each last changed.
type fakeShop struct {
	mu      sync.Mutex
	sales   map[string]Sale
	changed map[string]time.Time
	down    bool
}

// sold records a sale of one seat to email, made now.
func (s *fakeShop) sold(ref, email string) {
	s.put(Sale{Ref: ref, Seats: 1, Email: email, At: now}, now)
}

func (s *fakeShop) put(sale Sale, at time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sales == nil {
		s.sales = map[string]Sale{}
		s.changed = map[string]time.Time{}
	}
	s.sales[sale.Ref] = sale
	s.changed[sale.Ref] = at
}

func (s *fakeShop) setDown(down bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.down = down
}

func (s *fakeShop) Sale(ctx context.Context, ref string) (Sale, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return Sale{}, errors.New("Paddle is down")
	}
	sale, ok := s.sales[ref]
	if !ok {
		return Sale{}, ErrNotFound
	}
	return sale, nil
}

func (s *fakeShop) Since(ctx context.Context, t time.Time) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return nil, errors.New("Paddle is down")
	}
	var refs []string
	for ref, at := range s.changed {
		if !at.Before(t) {
			refs = append(refs, ref)
		}
	}
	slices.Sort(refs)
	return refs, nil
}

func (s *fakeShop) Refs(ctx context.Context, email string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.down {
		return nil, errors.New("Paddle is down")
	}
	var refs []string
	for ref, sale := range s.sales {
		if sale.Email == email {
			refs = append(refs, ref)
		}
	}
	slices.Sort(refs)
	return refs, nil
}
