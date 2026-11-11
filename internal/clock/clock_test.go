package clock_test

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/clock"
)

func TestRealNow(t *testing.T) {
	t.Parallel()

	before := time.Now()
	got := clock.Real().Now()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Errorf("Now() = %v, want within [%v, %v]", got, before, after)
	}
}

func TestRealSleep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		canceled bool
		timeout  time.Duration
		d        time.Duration
		calls    int
		want     error
	}{
		{name: "zero_d", d: 0, calls: 1, want: nil},
		{name: "negative_d", d: -time.Second, calls: 1, want: nil},
		{name: "short_d", d: time.Nanosecond, calls: 1, want: nil},
		{name: "done_ctx", canceled: true, d: time.Hour, calls: 1, want: context.Canceled},
		{name: "done_ctx_zero_d", canceled: true, d: 0, calls: 100, want: context.Canceled},
		{name: "deadline_during_wait", timeout: time.Millisecond, d: time.Hour, calls: 1, want: context.DeadlineExceeded},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if tt.timeout > 0 {
				var stop context.CancelFunc
				ctx, stop = context.WithTimeout(ctx, tt.timeout)
				defer stop()
			}
			if tt.canceled {
				cancel()
			}

			c := clock.Real()
			for i := range tt.calls {
				if got := c.Sleep(ctx, tt.d); !errors.Is(got, tt.want) {
					t.Fatalf("call %d: Sleep(%v) = %v, want %v", i+1, tt.d, got, tt.want)
				}
			}
		})
	}
}

func TestRealRunID(t *testing.T) {
	t.Parallel()

	before := time.Now()
	got := clock.Real().RunID()
	after := time.Now()

	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{6}Z$`).MatchString(got) {
		t.Fatalf("RunID() = %q, want yyyy-mm-ddThhmmssZ without suffix", got)
	}
	parsed, err := time.Parse("2006-01-02T150405Z", got)
	if err != nil {
		t.Fatalf("RunID() = %q is not a time: %v", got, err)
	}
	low, high := before.UTC().Truncate(time.Second), after.UTC()
	if parsed.Before(low) || parsed.After(high) {
		t.Errorf("RunID() = %q, want a time within [%v, %v]", got, low, high)
	}
}
