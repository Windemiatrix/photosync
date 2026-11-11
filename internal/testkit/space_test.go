package testkit_test

import (
	"errors"
	"slices"
	"sync"
	"testing"

	"github.com/Windemiatrix/photosync/internal/app"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

var _ app.SpaceManager = (*testkit.FakeSpaceManager)(nil)

func TestFakeSpaceManagerQueries(t *testing.T) {
	t.Parallel()

	const (
		library = "/Volumes/Photos/Photos Library.photoslibrary"
		scratch = "/private/tmp/photosync"
	)
	errDisk := errors.New("disk")
	tests := []struct {
		name     string
		fake     *testkit.FakeSpaceManager
		free     int64
		library  int64
		optimize bool
		err      error
	}{
		{name: "zero", fake: &testkit.FakeSpaceManager{}},
		{
			name: "values",
			fake: &testkit.FakeSpaceManager{FreeBytes: 5 << 30, LibraryBytes: 7 << 30, OptimizeStorage: true},
			free: 5 << 30, library: 7 << 30, optimize: true,
		},
		{
			name: "error",
			fake: &testkit.FakeSpaceManager{FreeBytes: 5 << 30, LibraryBytes: 7 << 30, OptimizeStorage: true, Err: errDisk},
			err:  errDisk,
		},
		{
			name: "pressure_error",
			fake: &testkit.FakeSpaceManager{FreeBytes: 5 << 30, LibraryBytes: 7 << 30, OptimizeStorage: true, PressureErr: errDisk},
			free: 5 << 30, library: 7 << 30, optimize: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			for _, path := range []string{library, scratch} {
				got, err := tc.fake.Free(ctx, path)
				if got != tc.free || !errors.Is(err, tc.err) {
					t.Errorf("Free(%q) = %d, %v, want %d, %v", path, got, err, tc.free, tc.err)
				}
			}
			size, err := tc.fake.LibrarySize(ctx, library)
			if size != tc.library || !errors.Is(err, tc.err) {
				t.Errorf("LibrarySize() = %d, %v, want %d, %v", size, err, tc.library, tc.err)
			}
			optimize, err := tc.fake.OptimizeStorageEnabled(ctx)
			if optimize != tc.optimize || !errors.Is(err, tc.err) {
				t.Errorf("OptimizeStorageEnabled() = %v, %v, want %v, %v", optimize, err, tc.optimize, tc.err)
			}
		})
	}
}

func TestFakeSpaceManagerPressure(t *testing.T) {
	t.Parallel()

	t.Run("records_and_releases", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		s := &testkit.FakeSpaceManager{}
		r1, err := s.Pressure(ctx, "/Volumes/Photos/a", 10<<30, 1<<30)
		if err != nil || r1 == nil {
			t.Fatalf("first Pressure() = %v, %v, want a release and no error", r1 != nil, err)
		}
		r2, err := s.Pressure(ctx, "/Volumes/Photos/b", 20<<30, 2<<30)
		if err != nil || r2 == nil {
			t.Fatalf("second Pressure() = %v, %v, want a release and no error", r2 != nil, err)
		}

		first := testkit.PressureCall{Dir: "/Volumes/Photos/a", Bytes: 10 << 30, KeepFree: 1 << 30}
		second := testkit.PressureCall{Dir: "/Volumes/Photos/b", Bytes: 20 << 30, KeepFree: 2 << 30}
		check := func(step int, want ...testkit.PressureCall) {
			t.Helper()
			if got := s.PressureCalls(); !slices.Equal(got, want) {
				t.Fatalf("step %d: PressureCalls() = %+v, want %+v", step, got, want)
			}
		}

		check(1, first, second)

		r2()
		second.Released = true
		check(2, first, second)

		r1()
		first.Released = true
		check(3, first, second)

		r1()
		check(4, first, second)
	})

	t.Run("copy", func(t *testing.T) {
		t.Parallel()

		s := &testkit.FakeSpaceManager{}
		if _, err := s.Pressure(t.Context(), "/Volumes/Photos/a", 1<<30, 1<<30); err != nil {
			t.Fatalf("Pressure() error = %v, want nil", err)
		}
		got := s.PressureCalls()
		if len(got) != 1 {
			t.Fatalf("len(PressureCalls()) = %d, want 1", len(got))
		}
		got[0].Released = true
		if again := s.PressureCalls(); again[0].Released {
			t.Errorf("PressureCalls()[0].Released = true after a change of an earlier result, want false")
		}
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		errDisk := errors.New("disk")
		s := &testkit.FakeSpaceManager{PressureErr: errDisk}
		release, err := s.Pressure(t.Context(), "/Volumes/Photos/a", 1<<30, 1<<30)
		if !errors.Is(err, errDisk) {
			t.Errorf("Pressure() error = %v, want %v", err, errDisk)
		}
		if release != nil {
			t.Errorf("Pressure() release is not nil, want nil")
		}
		if got := len(s.PressureCalls()); got != 0 {
			t.Errorf("len(PressureCalls()) = %d, want 0", got)
		}
	})

	t.Run("query_error", func(t *testing.T) {
		t.Parallel()

		s := &testkit.FakeSpaceManager{Err: errors.New("disk")}
		release, err := s.Pressure(t.Context(), "/Volumes/Photos/a", 1<<30, 1<<30)
		if err != nil || release == nil {
			t.Fatalf("Pressure() = %v, %v, want a release and no error", release != nil, err)
		}
		if got := len(s.PressureCalls()); got != 1 {
			t.Errorf("len(PressureCalls()) = %d, want 1", got)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		t.Parallel()

		const n = 32
		ctx := t.Context()
		s := &testkit.FakeSpaceManager{}
		var wg sync.WaitGroup
		for i := range n {
			wg.Go(func() {
				release, err := s.Pressure(ctx, "/Volumes/Photos", int64(i), 0)
				if err != nil || release == nil {
					t.Errorf("Pressure(%d) = %v, %v, want a release and no error", i, release != nil, err)
					return
				}
				release()
				_ = s.PressureCalls()
			})
		}
		wg.Wait()

		got := s.PressureCalls()
		if len(got) != n {
			t.Fatalf("len(PressureCalls()) = %d, want %d", len(got), n)
		}
		seen := make([]bool, n)
		for _, c := range got {
			if !c.Released {
				t.Errorf("call with Bytes %d is not released", c.Bytes)
			}
			if c.Bytes < 0 || c.Bytes >= n || seen[c.Bytes] {
				t.Errorf("call with Bytes %d is out of range or repeated", c.Bytes)
				continue
			}
			seen[c.Bytes] = true
		}
	})
}
