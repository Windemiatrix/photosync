package clock_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/clock"
)

func goSleep(ctx context.Context, f *clock.Fake, d time.Duration) <-chan error {
	res := make(chan error, 1)
	go func() { res <- f.Sleep(ctx, d) }()
	return res
}

func TestFakeNow(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	f := clock.NewFake(start)
	c := f.Clock
	if got := f.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}

	f.Advance(90 * time.Second)
	want := time.Date(2026, 1, 2, 3, 5, 35, 0, time.UTC)
	if got := f.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance(90s) = %v, want %v", got, want)
	}

	f.Advance(0)
	if got := f.Now(); !got.Equal(want) {
		t.Errorf("Now() after Advance(0) = %v, want %v", got, want)
	}

	if got := c.Now(); !got.Equal(want) {
		t.Errorf("embedded Clock Now() = %v, want %v", got, want)
	}
}

func TestFakeRunID(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 3, 1, 23, 59, 58, 500_000_000, time.FixedZone("UTC-5", -5*60*60))
	f := clock.NewFake(start)

	steps := []struct {
		advance time.Duration
		want    string
	}{
		{want: "2026-03-02T045958Z"},
		{want: "2026-03-02T045958Z-1"},
		{want: "2026-03-02T045958Z-2"},
		{advance: 499 * time.Millisecond, want: "2026-03-02T045958Z-3"},
		{advance: time.Millisecond, want: "2026-03-02T045959Z"},
		{want: "2026-03-02T045959Z-1"},
	}
	for i, s := range steps {
		f.Advance(s.advance)
		if got := f.RunID(); got != s.want {
			t.Errorf("step %d: RunID() = %q, want %q", i+1, got, s.want)
		}
	}
}

func TestFakeSleep(t *testing.T) {
	t.Parallel()

	start := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	check := func(t *testing.T, what string, got, want error) {
		t.Helper()
		if !errors.Is(got, want) {
			t.Errorf("%s = %v, want %v", what, got, want)
		}
	}

	t.Run("wakes_at_deadline", func(t *testing.T) {
		t.Parallel()

		f := clock.NewFake(start)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		res := goSleep(ctx, f, time.Minute)
		f.BlockUntil(1)
		f.Advance(time.Minute)
		f.BlockUntil(0)
		check(t, "Sleep", <-res, nil)
	})

	t.Run("short_of_deadline", func(t *testing.T) {
		t.Parallel()

		f := clock.NewFake(start)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		f.Advance(30 * time.Second)
		res := goSleep(ctx, f, time.Minute)
		f.BlockUntil(1)
		f.Advance(time.Minute - time.Nanosecond)
		cancel()
		f.BlockUntil(0)
		check(t, "Sleep", <-res, context.Canceled)
	})

	t.Run("wakes_only_due", func(t *testing.T) {
		t.Parallel()

		f := clock.NewFake(start)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		res10 := goSleep(ctx, f, 10*time.Second)
		res20 := goSleep(ctx, f, 20*time.Second)
		res30 := goSleep(ctx, f, 30*time.Second)
		f.BlockUntil(3)
		f.Advance(20 * time.Second)
		f.BlockUntil(1)
		check(t, "Sleep(10s)", <-res10, nil)
		check(t, "Sleep(20s)", <-res20, nil)
		cancel()
		f.BlockUntil(0)
		check(t, "Sleep(30s)", <-res30, context.Canceled)
	})

	t.Run("done_ctx", func(t *testing.T) {
		t.Parallel()

		f := clock.NewFake(start)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()

		check(t, "Sleep(1m)", f.Sleep(ctx, time.Minute), context.Canceled)
		check(t, "Sleep(0)", f.Sleep(ctx, 0), context.Canceled)
	})

	t.Run("non_positive_d", func(t *testing.T) {
		t.Parallel()

		f := clock.NewFake(start)
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()

		check(t, "Sleep(0)", f.Sleep(ctx, 0), nil)
		check(t, "Sleep(-1s)", f.Sleep(ctx, -time.Second), nil)
		if got := f.Now(); !got.Equal(start) {
			t.Errorf("Now() = %v, want %v", got, start)
		}
	})
}
