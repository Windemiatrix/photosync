package clock

import (
	"context"
	"strconv"
	"sync"
	"time"
)

// Clock is how the app and the adapters read the time, wait and get a run id,
// so tests can replace all three. The zero value is not usable: take Real or
// NewFake.
type Clock struct {
	Now func() time.Time
	// Sleep returns nil after d, at once when d <= 0, and ctx.Err() when ctx
	// is done before or during the wait.
	Sleep func(ctx context.Context, d time.Duration) error
	// RunID returns Now in UTC as 2006-01-02T150405Z. A call within the same
	// second as the previous call returns it with the next suffix -1, -2, …,
	// so a caller that finds an id taken calls again.
	RunID func() string
}

// Real returns the clock of the running program. Each Clock it returns numbers
// its run ids on its own.
func Real() Clock {
	ids := &runIDs{}
	return Clock{
		Now:   time.Now,
		Sleep: sleep,
		RunID: func() string { return ids.next(time.Now()) },
	}
}

func sleep(ctx context.Context, d time.Duration) error {
	// Without this check a done ctx and d <= 0 would make select pick at random.
	if err := ctx.Err(); err != nil {
		return err
	}
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// runIDs is shared by Real and Fake so the id format is written once. Two runs
// in one second must not share an id.
type runIDs struct {
	mu   sync.Mutex
	last string
	n    int
}

func (r *runIDs) next(now time.Time) string {
	id := now.UTC().Format("2006-01-02T150405Z")

	r.mu.Lock()
	defer r.mu.Unlock()
	if id != r.last {
		r.last = id
		r.n = 0
		return id
	}
	r.n++
	return id + "-" + strconv.Itoa(r.n)
}
