package clock

import (
	"context"
	"sync"
	"time"
)

// Fake is a Clock for tests: its time moves only by Advance, and Sleep waits
// for Advance, not for real time. Its methods are safe for concurrent use.
type Fake struct {
	Clock

	mu      sync.Mutex
	changed sync.Cond
	now     time.Time
	waiting map[chan struct{}]time.Time
	ids     runIDs
}

// NewFake returns a Fake whose time is start.
func NewFake(start time.Time) *Fake {
	f := &Fake{now: start, waiting: map[chan struct{}]time.Time{}}
	f.changed.L = &f.mu
	f.Clock = Clock{
		Now:   f.current,
		Sleep: f.sleep,
		RunID: func() string { return f.ids.next(f.current()) },
	}
	return f
}

func (f *Fake) current() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

func (f *Fake) sleep(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}

	wake := make(chan struct{})
	f.mu.Lock()
	f.waiting[wake] = f.now.Add(d)
	f.changed.Broadcast()
	f.mu.Unlock()

	select {
	case <-wake:
		return nil
	case <-ctx.Done():
		f.mu.Lock()
		delete(f.waiting, wake)
		f.changed.Broadcast()
		f.mu.Unlock()
		return ctx.Err()
	}
}

// Advance moves the time by d and wakes every Sleep whose deadline it reaches.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
	for w, until := range f.waiting {
		if !until.After(f.now) {
			close(w)
			delete(f.waiting, w)
		}
	}
	f.changed.Broadcast()
}

// BlockUntil returns when exactly n calls of Sleep are waiting. A test calls
// it before Advance: a Sleep that starts after Advance counts its deadline
// from the new time.
func (f *Fake) BlockUntil(n int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for len(f.waiting) != n {
		f.changed.Wait()
	}
}
