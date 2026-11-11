package testkit_test

import (
	"context"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

func photoLibPadded(s string, size int) string {
	return s + string(make([]byte, size-len(s)))
}

func photoLibDir(t *testing.T, dir string, want map[string]string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v, want nil", err)
	}
	got := map[string]string{}
	for _, e := range entries {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q) error = %v, want nil", e.Name(), err)
		}
		got[e.Name()] = string(b)
	}
	gotNames, wantNames := slices.Sorted(maps.Keys(got)), slices.Sorted(maps.Keys(want))
	if !slices.Equal(gotNames, wantNames) {
		t.Errorf("files in dir = %v, want %v", gotNames, wantNames)
		return
	}
	for name, w := range want {
		if g := got[name]; g != w {
			t.Errorf("file %q has %d bytes starting %q, want %d bytes starting %q",
				name, len(g), g[:min(len(g), 40)], len(w), w[:min(len(w), 40)])
		}
	}
}

func TestFakePhotoLibraryDownload(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	dir := t.TempDir()
	var got []int64
	files, err := lib.Download(ctx, "uuid-1", dir, func(n int64) { got = append(got, n) })
	if err != nil {
		t.Fatalf("Download() error = %v, want nil", err)
	}
	wantFiles := []domain.LocalFile{
		{Role: domain.ResourceOriginal, Path: filepath.Join(dir, "original")},
		{Role: domain.ResourceLiveVideo, Path: filepath.Join(dir, "live_video")},
	}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("Download() = %+v, want %+v", files, wantFiles)
	}
	photoLibDir(t, dir, map[string]string{
		"original":   photoLibPadded("IMG_0001.HEIC 2024-05-01T10:00:00Z", 1<<20),
		"live_video": photoLibPadded("IMG_0001.MOV 2024-05-01T10:00:00Z", 2<<20),
	})
	if want := []int64{1 << 20, 3 << 20}; !slices.Equal(got, want) {
		t.Errorf("progress got %v, want %v", got, want)
	}
	c := lib.Counters()
	if want := map[string]int{"Download": 1}; !maps.Equal(c.Calls, want) {
		t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
	}
	if c.BytesDownloaded != 3<<20 {
		t.Errorf("Counters().BytesDownloaded = %d, want %d", c.BytesDownloaded, 3<<20)
	}
	want := photoLibWant()
	want.Assets[0].Missing = false
	if snap := photoLibSnapshot(t, lib); !reflect.DeepEqual(snap, want) {
		t.Errorf("Snapshot() after Download() = %+v, want %+v", snap, want)
	}

	var got2 []int64
	if _, err := lib.Download(ctx, "uuid-1", t.TempDir(), func(n int64) { got2 = append(got2, n) }); err != nil {
		t.Fatalf("second Download() error = %v, want nil", err)
	}
	if want := []int64{1 << 20, 3 << 20}; !slices.Equal(got2, want) {
		t.Errorf("progress of the second Download() got %v, want %v", got2, want)
	}
	if got := lib.Counters().BytesDownloaded; got != 6<<20 {
		t.Errorf("Counters().BytesDownloaded after two calls = %d, want %d", got, 6<<20)
	}
}

func TestFakePhotoLibraryDownloadContent(t *testing.T) {
	t.Parallel()

	taken := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		uuid     string
		taken    time.Time
		filename string
		size     int64
		want     string
	}{
		{"duplicate_1", "uuid-11", taken, "IMG_0005.JPG", 64, photoLibPadded("IMG_0005.JPG 2024-05-01T10:00:00Z", 64)},
		{"duplicate_2", "uuid-12", taken, "IMG_0005.JPG", 64, photoLibPadded("IMG_0005.JPG 2024-05-01T10:00:00Z", 64)},
		{"fraction", "uuid-13", taken.Add(500 * time.Millisecond), "IMG_0005.JPG", 64, photoLibPadded("IMG_0005.JPG 2024-05-01T10:00:00.5Z", 64)},
		{"zone", "uuid-14", time.Date(2024, 5, 1, 13, 0, 0, 0, time.FixedZone("", 3*60*60)), "IMG_0005.JPG", 64, photoLibPadded("IMG_0005.JPG 2024-05-01T10:00:00Z", 64)},
		{"zero_time", "uuid-15", time.Time{}, "IMG_0005.JPG", 64, photoLibPadded("IMG_0005.JPG 0001-01-01T00:00:00Z", 64)},
		{"short", "uuid-16", taken, "IMG_0005.JPG", 4, "IMG_"},
		{"empty", "uuid-17", taken, "IMG_0005.JPG", 0, ""},
		{"parent_name", "uuid-18", taken, "../IMG_0006.JPG", 64, photoLibPadded("../IMG_0006.JPG 2024-05-01T10:00:00Z", 64)},
	}
	var seed domain.LibrarySnapshot
	for _, tt := range tests {
		seed.Assets = append(seed.Assets, domain.Asset{
			Key:       domain.AssetKey{UUID: tt.uuid},
			Taken:     tt.taken,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: tt.filename, Size: tt.size}},
		})
	}
	lib := testkit.NewFakePhotoLibrary(seed)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			files, err := lib.Download(t.Context(), tt.uuid, dir, nil)
			if err != nil {
				t.Fatalf("Download() error = %v, want nil", err)
			}
			want := []domain.LocalFile{{Role: domain.ResourceOriginal, Path: filepath.Join(dir, "original")}}
			if !slices.Equal(files, want) {
				t.Fatalf("Download() = %+v, want %+v", files, want)
			}
			photoLibDir(t, dir, map[string]string{"original": tt.want})
		})
	}
}

func TestFakePhotoLibraryDownloadNotFound(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct{ name, uuid string }{
		{"hidden", "uuid-2"},
		{"trashed", "uuid-3"},
		{"unknown", "uuid-9"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			dir := t.TempDir()
			files, err := lib.Download(t.Context(), tt.uuid, dir, func(int64) { t.Errorf("progress called") })
			if !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("Download(%q) error = %v, want %v", tt.uuid, err, domain.ErrNotFound)
			}
			if len(files) != 0 {
				t.Errorf("Download(%q) = %+v, want no files", tt.uuid, files)
			}
			photoLibDir(t, dir, nil)
			c := lib.Counters()
			if want := map[string]int{"Download": 1}; !maps.Equal(c.Calls, want) {
				t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
			}
			if c.BytesDownloaded != 0 {
				t.Errorf("Counters().BytesDownloaded = %d, want 0", c.BytesDownloaded)
			}
			if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
				t.Errorf("Snapshot() after Download(%q) = %+v, want %+v", tt.uuid, got, want)
			}
		})
	}
}

func TestFakePhotoLibraryDownloadFails(t *testing.T) {
	t.Parallel()

	t.Run("canceled_before", func(t *testing.T) {
		t.Parallel()

		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		files, err := lib.Download(ctx, "uuid-1", dir, nil)
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Download() error = %v, want %v", err, context.Canceled)
		}
		if len(files) != 0 {
			t.Errorf("Download() = %+v, want no files", files)
		}
		photoLibDir(t, dir, nil)
		c := lib.Counters()
		if want := map[string]int{"Download": 1}; !maps.Equal(c.Calls, want) {
			t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
		}
		if c.BytesDownloaded != 0 {
			t.Errorf("Counters().BytesDownloaded = %d, want 0", c.BytesDownloaded)
		}
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
	})

	t.Run("fail_next", func(t *testing.T) {
		t.Parallel()

		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		dir := t.TempDir()
		boom := errors.New("boom")
		lib.FailNext("Download", boom)
		files, err := lib.Download(t.Context(), "uuid-1", dir, nil)
		if !errors.Is(err, boom) {
			t.Errorf("Download() error = %v, want %v", err, boom)
		}
		if len(files) != 0 {
			t.Errorf("Download() = %+v, want no files", files)
		}
		photoLibDir(t, dir, nil)
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
		files, err = lib.Download(t.Context(), "uuid-1", t.TempDir(), nil)
		if err != nil || len(files) != 2 {
			t.Errorf("second Download() = %d files, %v; want 2 files, nil", len(files), err)
		}
	})

	t.Run("canceled_between", func(t *testing.T) {
		t.Parallel()

		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		dir := t.TempDir()
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		files, err := lib.Download(ctx, "uuid-1", dir, func(int64) { cancel() })
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Download() error = %v, want %v", err, context.Canceled)
		}
		if len(files) != 0 {
			t.Errorf("Download() = %+v, want no files", files)
		}
		photoLibDir(t, dir, map[string]string{"original": photoLibPadded("IMG_0001.HEIC 2024-05-01T10:00:00Z", 1<<20)})
		if got := lib.Counters().BytesDownloaded; got != 1<<20 {
			t.Errorf("Counters().BytesDownloaded = %d, want %d", got, 1<<20)
		}
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
	})

	t.Run("missing_dir", func(t *testing.T) {
		t.Parallel()

		lib := testkit.NewFakePhotoLibrary(photoLibSeed())
		dir := filepath.Join(t.TempDir(), "missing")
		files, err := lib.Download(t.Context(), "uuid-1", dir, func(int64) { t.Errorf("progress called") })
		if !errors.Is(err, fs.ErrNotExist) {
			t.Errorf("Download() error = %v, want %v", err, fs.ErrNotExist)
		}
		if len(files) != 0 {
			t.Errorf("Download() = %+v, want no files", files)
		}
		if got := lib.Counters().BytesDownloaded; got != 0 {
			t.Errorf("Counters().BytesDownloaded = %d, want 0", got)
		}
		if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
			t.Errorf("Snapshot() = %+v, want %+v", got, want)
		}
	})
}

func TestFakePhotoLibraryRestore(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	lib.Restore("uuid-9")
	lib.Restore("uuid-2")
	if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
		t.Errorf("Restore changed an asset that is not in the trash: Snapshot() = %+v, want %+v", got, want)
	}

	lib.Restore("uuid-3")
	want := photoLibWant()
	want.Assets[2].InTrash = false
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after Restore(%q) = %+v, want %+v", "uuid-3", got, want)
	}

	dir := t.TempDir()
	files, err := lib.Download(ctx, "uuid-3", dir, nil)
	if err != nil {
		t.Fatalf("Download() of a restored asset error = %v, want nil", err)
	}
	wantFiles := []domain.LocalFile{{Role: domain.ResourceOriginal, Path: filepath.Join(dir, "original")}}
	if !slices.Equal(files, wantFiles) {
		t.Fatalf("Download() of a restored asset = %+v, want %+v", files, wantFiles)
	}
	photoLibDir(t, dir, map[string]string{"original": photoLibPadded("IMG_0003.MOV 0001-01-01T00:00:00Z", 4<<20)})

	want.Assets[2].Missing = false
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after Download() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"Snapshot": 3, "Download": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryFailNextNil(t *testing.T) {
	t.Parallel()

	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	boom := errors.New("boom")
	lib.FailNext("Download", boom)
	lib.FailNext("Download", nil)
	if _, err := lib.Download(t.Context(), "uuid-1", t.TempDir(), nil); err != nil {
		t.Errorf("Download() after FailNext(%q, nil) error = %v, want nil", "Download", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	lib.FailNext("Snapshot", nil)
	if _, err := lib.Snapshot(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("Snapshot() after FailNext(%q, nil) error = %v, want %v", "Snapshot", err, context.Canceled)
	}
}

func TestFakePhotoLibraryFilesConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	dirs := make([]string, 8)
	for i := range dirs {
		dirs[i] = t.TempDir()
	}
	var wg sync.WaitGroup
	for _, dir := range dirs {
		wg.Go(func() {
			lib.Restore("uuid-3")
			if _, err := lib.Download(ctx, "uuid-1", dir, nil); err != nil {
				t.Errorf("Download() error = %v, want nil", err)
			}
			lib.Counters()
		})
	}
	wg.Wait()

	c := lib.Counters()
	if want := map[string]int{"Download": 8}; !maps.Equal(c.Calls, want) {
		t.Errorf("Counters().Calls = %v, want %v", c.Calls, want)
	}
	if c.BytesDownloaded != 24<<20 {
		t.Errorf("Counters().BytesDownloaded = %d, want %d", c.BytesDownloaded, 24<<20)
	}
}
