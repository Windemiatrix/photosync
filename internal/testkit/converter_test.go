package testkit_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/Windemiatrix/photosync/internal/app"
	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

var _ app.Converter = (*testkit.FakeConverter)(nil)

func TestFakeConverterToMP4(t *testing.T) {
	t.Parallel()

	errCodec := errors.New("codec")
	write := func(t *testing.T, path, data string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatalf("write %s: %v", filepath.Base(path), err)
		}
	}
	read := func(t *testing.T, path string) string {
		t.Helper()
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", filepath.Base(path), err)
		}
		return string(b)
	}
	absent := func(t *testing.T, path string) {
		t.Helper()
		if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("%s after a failed ToMP4: %v, want it missing", filepath.Base(path), err)
		}
	}
	calls := func(t *testing.T, c *testkit.FakeConverter, want ...testkit.ToMP4Call) {
		t.Helper()
		if got := c.ToMP4Calls(); !slices.Equal(got, want) {
			t.Errorf("ToMP4Calls() = %+v, want %+v", got, want)
		}
	}

	t.Run("converts", func(t *testing.T) {
		t.Parallel()

		ctx := t.Context()
		dir := t.TempDir()
		a := filepath.Join(dir, "clip.avi")
		b := filepath.Join(dir, "clip.mkv")
		write(t, a, "avi 1")
		write(t, b, "mkv 2")
		out1 := filepath.Join(dir, "clip-1.mp4")
		out2 := filepath.Join(dir, "clip-2.mp4")
		c := &testkit.FakeConverter{}
		if err := c.ToMP4(ctx, a, out1); err != nil {
			t.Fatalf("ToMP4(clip.avi) error = %v, want nil", err)
		}
		if err := c.ToMP4(ctx, b, out2); err != nil {
			t.Fatalf("ToMP4(clip.mkv) error = %v, want nil", err)
		}

		if got := read(t, out1); got != "avi 1" {
			t.Errorf("clip-1.mp4 = %q, want %q", got, "avi 1")
		}
		if got := read(t, out2); got != "mkv 2" {
			t.Errorf("clip-2.mp4 = %q, want %q", got, "mkv 2")
		}
		if got := read(t, a); got != "avi 1" {
			t.Errorf("clip.avi = %q after ToMP4, want %q", got, "avi 1")
		}
		calls(t, c, testkit.ToMP4Call{Src: a, Dst: out1}, testkit.ToMP4Call{Src: b, Dst: out2})
	})

	t.Run("not_installed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "clip.avi")
		dst := filepath.Join(dir, "clip.mp4")
		write(t, src, "avi 1")
		c := &testkit.FakeConverter{NotInstalled: true, Err: errCodec}
		if err := c.ToMP4(t.Context(), src, dst); !errors.Is(err, domain.ErrNotSupported) {
			t.Errorf("ToMP4() error = %v, want %v", err, domain.ErrNotSupported)
		}
		absent(t, dst)
		calls(t, c, testkit.ToMP4Call{Src: src, Dst: dst})
	})

	t.Run("error", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "clip.avi")
		dst := filepath.Join(dir, "clip.mp4")
		write(t, src, "avi 1")
		c := &testkit.FakeConverter{Err: errCodec}
		if err := c.ToMP4(t.Context(), src, dst); !errors.Is(err, errCodec) {
			t.Errorf("ToMP4() error = %v, want %v", err, errCodec)
		}
		absent(t, dst)
		calls(t, c, testkit.ToMP4Call{Src: src, Dst: dst})
	})

	t.Run("missing_src", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "clip.avi")
		dst := filepath.Join(dir, "clip.mp4")
		c := &testkit.FakeConverter{}
		if err := c.ToMP4(t.Context(), src, dst); !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("ToMP4() error = %v, want %v", err, fs.ErrNotExist)
		}
		absent(t, dst)
		calls(t, c, testkit.ToMP4Call{Src: src, Dst: dst})
	})

	t.Run("dst_taken", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "clip.avi")
		dst := filepath.Join(dir, "clip.mp4")
		write(t, src, "avi 1")
		write(t, dst, "mp4 old")
		c := &testkit.FakeConverter{}
		if err := c.ToMP4(t.Context(), src, dst); !errors.Is(err, fs.ErrExist) {
			t.Errorf("ToMP4() error = %v, want %v", err, fs.ErrExist)
		}
		if got := read(t, dst); got != "mp4 old" {
			t.Errorf("clip.mp4 = %q after a failed ToMP4, want %q", got, "mp4 old")
		}
		calls(t, c, testkit.ToMP4Call{Src: src, Dst: dst})
	})

	t.Run("calls_copy", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		src := filepath.Join(dir, "clip.avi")
		dst := filepath.Join(dir, "clip.mp4")
		write(t, src, "avi 1")
		c := &testkit.FakeConverter{}
		if err := c.ToMP4(t.Context(), src, dst); err != nil {
			t.Fatalf("ToMP4() error = %v, want nil", err)
		}
		got := c.ToMP4Calls()
		if len(got) != 1 {
			t.Fatalf("len(ToMP4Calls()) = %d, want 1", len(got))
		}
		got[0].Src = "changed"
		if again := c.ToMP4Calls(); again[0].Src != src {
			t.Errorf("ToMP4Calls() shares its slice: Src = %q after a change of an earlier result, want %q",
				filepath.Base(again[0].Src), filepath.Base(src))
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		t.Parallel()

		const n = 16
		ctx := t.Context()
		dir := t.TempDir()
		c := &testkit.FakeConverter{}
		pairs := make([]testkit.ToMP4Call, 0, n)
		want := map[testkit.ToMP4Call]bool{}
		for i := range n {
			call := testkit.ToMP4Call{
				Src: filepath.Join(dir, fmt.Sprintf("clip-%02d.avi", i)),
				Dst: filepath.Join(dir, fmt.Sprintf("clip-%02d.mp4", i)),
			}
			write(t, call.Src, "avi 1")
			pairs = append(pairs, call)
			want[call] = true
		}
		var wg sync.WaitGroup
		for _, p := range pairs {
			wg.Go(func() {
				if err := c.ToMP4(ctx, p.Src, p.Dst); err != nil {
					t.Errorf("ToMP4(%s) error = %v, want nil", filepath.Base(p.Src), err)
					return
				}
				_ = c.ToMP4Calls()
			})
		}
		wg.Wait()

		got := c.ToMP4Calls()
		if len(got) != n {
			t.Fatalf("len(ToMP4Calls()) = %d, want %d", len(got), n)
		}
		for _, call := range got {
			if !want[call] {
				t.Errorf("unexpected or repeated call %+v", call)
			}
			delete(want, call)
		}
		if len(want) != 0 {
			t.Errorf("%d calls are missing from ToMP4Calls()", len(want))
		}
	})
}

func TestFakeConverterVersion(t *testing.T) {
	t.Parallel()

	errCodec := errors.New("codec")
	tests := []struct {
		name string
		fake *testkit.FakeConverter
		want string
		err  error
	}{
		{name: "installed", fake: &testkit.FakeConverter{}, want: "fake 1.0"},
		{name: "error", fake: &testkit.FakeConverter{Err: errCodec}, want: "fake 1.0"},
		{name: "not_installed", fake: &testkit.FakeConverter{NotInstalled: true}, err: domain.ErrNotSupported},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := tc.fake.Version(t.Context())
			if got != tc.want || !errors.Is(err, tc.err) {
				t.Errorf("Version() = %q, %v, want %q, %v", got, err, tc.want, tc.err)
			}
		})
	}
}
