package testkit_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

func photoLibSeed() domain.LibrarySnapshot {
	return domain.LibrarySnapshot{
		Assets: []domain.Asset{
			{
				Key:          domain.AssetKey{CloudAssetGUID: "guid-1", UUID: "uuid-1"},
				Library:      domain.LibraryPersonal,
				OriginalName: "IMG_0001.HEIC",
				Taken:        time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC),
				Resources: []domain.Resource{
					{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 1 << 20},
					{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 2 << 20},
				},
				AlbumUUIDs: []string{"album-1"},
				InCloud:    new(true),
				Missing:    false,
			},
			{
				Key:          domain.AssetKey{UUID: "uuid-2"},
				Library:      domain.LibraryShared,
				OriginalName: "IMG_0002.JPG",
				Hidden:       true,
				Resources: []domain.Resource{
					{Role: domain.ResourceOriginal, Filename: "IMG_0002.JPG", Size: 3 << 20},
				},
				InCloud: nil,
				Missing: false,
			},
			{
				Key:          domain.AssetKey{CloudAssetGUID: "guid-3", UUID: "uuid-3"},
				Library:      domain.LibraryPersonal,
				OriginalName: "IMG_0003.MOV",
				IsVideo:      true,
				InTrash:      true,
				Resources: []domain.Resource{
					{Role: domain.ResourceOriginal, Filename: "IMG_0003.MOV", Size: 4 << 20},
				},
				InCloud: new(true),
				Missing: true,
			},
		},
		Albums: []domain.Album{
			{UUID: "folder-1", Kind: domain.KindFolder, Title: "Trips"},
			{UUID: "folder-2", Kind: domain.KindFolder, Title: "2024", ParentUUID: "folder-1", FolderTitles: []string{"Old"}},
			{UUID: "album-1", Key: "album-guid-1", KeySource: domain.KeyCloudGUID, Title: "05/01 Holidays", ParentUUID: "folder-2",
				Start: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)},
			{UUID: "album-2", Title: "Отпуск", InTrash: true, FolderTitles: []string{"Old"}},
			{UUID: "album-3", Title: "Lost", ParentUUID: "folder-9"},
		},
	}
}

func photoLibWant() domain.LibrarySnapshot {
	want := photoLibSeed()
	want.Assets[0].Missing = true
	want.Albums[1].FolderTitles = []string{"Trips"}
	want.Albums[2].FolderTitles = []string{"Trips", "2024"}
	want.Albums[3].FolderTitles = nil
	return want
}

func photoLibSnapshot(t *testing.T, lib *testkit.FakePhotoLibrary) domain.LibrarySnapshot {
	t.Helper()
	got, err := lib.Snapshot(t.Context(), "/Volumes/Photos/Photos Library.photoslibrary")
	if err != nil {
		t.Fatalf("Snapshot() error = %v, want nil", err)
	}
	if len(got.Assets) != 3 || len(got.Albums) != 5 {
		t.Fatalf("Snapshot() has %d assets and %d albums, want 3 and 5", len(got.Assets), len(got.Albums))
	}
	return got
}

func TestFakePhotoLibrarySnapshot(t *testing.T) {
	t.Parallel()

	got := photoLibSnapshot(t, testkit.NewFakePhotoLibrary(photoLibSeed()))
	if want := photoLibWant(); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}

	empty, err := testkit.NewFakePhotoLibrary(domain.LibrarySnapshot{}).Snapshot(t.Context(), "")
	if err != nil {
		t.Fatalf("Snapshot() of an empty library error = %v, want nil", err)
	}
	if len(empty.Assets) != 0 || len(empty.Albums) != 0 {
		t.Errorf("Snapshot() of an empty library = %+v, want no assets and no albums", empty)
	}
}

func TestFakePhotoLibraryCopies(t *testing.T) {
	t.Parallel()

	seed := photoLibSeed()
	lib := testkit.NewFakePhotoLibrary(seed)
	if seed.Assets[0].Missing {
		t.Errorf("NewFakePhotoLibrary wrote into its argument: Missing = true, want false")
	}

	seed.Assets[0].OriginalName = "IMG_9999.HEIC"
	seed.Assets[0].Resources[0].Size = 1
	seed.Assets[0].AlbumUUIDs[0] = "album-9"
	seed.Albums[0].Title = "Changed"

	first := photoLibSnapshot(t, lib)
	first.Assets[0].OriginalName = "IMG_9999.HEIC"
	first.Assets[0].Resources[0].SHA256 = "00"
	first.Assets[0].AlbumUUIDs[0] = "album-9"
	first.Albums[0].Title = "Changed"
	first.Albums[2].FolderTitles[0] = "Changed"

	if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after changes of the seed and of a snapshot = %+v, want %+v", got, want)
	}
}

func TestFakePhotoLibraryFailNext(t *testing.T) {
	t.Parallel()

	calls := []struct {
		op   string
		call func(context.Context, *testkit.FakePhotoLibrary) error
	}{
		{"Snapshot", func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			_, err := l.Snapshot(ctx, "")
			return err
		}},
		{"SystemLibraryPath", func(_ context.Context, l *testkit.FakePhotoLibrary) error {
			_, err := l.SystemLibraryPath()
			return err
		}},
		{"LibraryIdentity", func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			_, err := l.LibraryIdentity(ctx, "")
			return err
		}},
	}
	for _, c := range calls {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			boom := errors.New("boom")
			lib.FailNext(c.op, boom)

			for _, other := range calls {
				if other.op == c.op {
					continue
				}
				if err := other.call(ctx, lib); err != nil {
					t.Errorf("%s() after FailNext(%q) error = %v, want nil", other.op, c.op, err)
				}
			}
			if err := c.call(ctx, lib); !errors.Is(err, boom) {
				t.Errorf("%s() error = %v, want %v", c.op, err, boom)
			}
			if err := c.call(ctx, lib); err != nil {
				t.Errorf("second %s() error = %v, want nil", c.op, err)
			}
			if got := lib.Counters().Calls[c.op]; got != 2 {
				t.Errorf("Counters().Calls[%q] = %d, want 2", c.op, got)
			}
		})
	}
}

func TestFakePhotoLibraryCanceled(t *testing.T) {
	t.Parallel()

	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if _, err := lib.Snapshot(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("Snapshot() error = %v, want %v", err, context.Canceled)
	}
	if _, err := lib.LibraryIdentity(ctx, ""); !errors.Is(err, context.Canceled) {
		t.Errorf("LibraryIdentity() error = %v, want %v", err, context.Canceled)
	}
	want := map[string]int{"Snapshot": 1, "LibraryIdentity": 1}
	if got := lib.Counters().Calls; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryCounters(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	if got := lib.Counters().Calls; len(got) != 0 {
		t.Errorf("Counters().Calls of a new fake = %v, want empty", got)
	}

	for range 2 {
		if _, err := lib.Snapshot(ctx, ""); err != nil {
			t.Fatalf("Snapshot() error = %v, want nil", err)
		}
	}
	if _, err := lib.SystemLibraryPath(); err != nil {
		t.Fatalf("SystemLibraryPath() error = %v, want nil", err)
	}

	c := lib.Counters()
	want := map[string]int{"Snapshot": 2, "SystemLibraryPath": 1}
	if !maps.Equal(c.Calls, want) {
		t.Fatalf("Counters().Calls = %v, want %v", c.Calls, want)
	}
	c.Calls["Snapshot"] = 100
	if got := lib.Counters().Calls["Snapshot"]; got != 2 {
		t.Errorf("Counters shares its map: Calls[%q] = %d, want 2", "Snapshot", got)
	}
}

func TestFakePhotoLibraryIdentity(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	a := testkit.NewFakePhotoLibrary(photoLibSeed())
	b := testkit.NewFakePhotoLibrary(photoLibSeed())

	idA, err := a.LibraryIdentity(ctx, "")
	if err != nil || idA == "" {
		t.Errorf("LibraryIdentity() = %q, %v; want a non-empty identity, nil", idA, err)
	}
	idOther, err := a.LibraryIdentity(ctx, "/Volumes/Other/Other.photoslibrary")
	if err != nil || idOther != idA {
		t.Errorf("LibraryIdentity() of another path = %q, %v; want %q, nil", idOther, err, idA)
	}
	idB, err := b.LibraryIdentity(ctx, "")
	if err != nil || idB == idA {
		t.Errorf("two fakes share the identity %q (error %v)", idB, err)
	}

	pathA, err := a.SystemLibraryPath()
	if err != nil || pathA == "" {
		t.Errorf("SystemLibraryPath() = %q, %v; want a non-empty path, nil", pathA, err)
	}
	pathB, err := b.SystemLibraryPath()
	if err != nil || pathB != pathA {
		t.Errorf("SystemLibraryPath() of another fake = %q, %v; want %q, nil", pathB, err, pathA)
	}
}

func TestFakePhotoLibraryConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := lib.Snapshot(ctx, ""); err != nil {
				t.Errorf("Snapshot() error = %v, want nil", err)
			}
			lib.Counters()
		})
	}
	wg.Wait()

	if got := lib.Counters().Calls["Snapshot"]; got != 8 {
		t.Errorf("Counters().Calls[%q] = %d, want 8", "Snapshot", got)
	}
}
