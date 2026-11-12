package testkit_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

func TestFakePhotoLibraryMembers(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	early := time.Date(2024, 4, 30, 8, 0, 0, 0, time.UTC)
	seed := photoLibSeed()
	seed.Assets[1].Hidden = false
	seed.Assets[1].Taken = early
	seed.Assets[1].AlbumUUIDs = []string{"album-3"}
	lib := testkit.NewFakePhotoLibrary(seed)

	for _, c := range []struct {
		album  string
		assets []string
	}{
		{album: "album-1", assets: []string{"uuid-2", "uuid-1"}},
		{album: "album-3", assets: []string{"uuid-2"}},
		{album: "album-1", assets: nil},
	} {
		if err := lib.AddToAlbum(ctx, c.album, c.assets); err != nil {
			t.Fatalf("AddToAlbum(%q, %q) error = %v, want nil", c.album, c.assets, err)
		}
	}
	want := photoLibWant()
	want.Assets[1].Hidden = false
	want.Assets[1].Taken = early
	want.Assets[1].AlbumUUIDs = []string{"album-3", "album-1"}
	want.Albums[2].Start = early
	want.Albums[4].Start = early
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after AddToAlbum() = %+v, want %+v", got, want)
	}

	for _, c := range []struct {
		album  string
		assets []string
	}{
		{album: "album-1", assets: []string{"uuid-2"}},
		{album: "album-3", assets: []string{"uuid-1", "uuid-2"}},
	} {
		if err := lib.RemoveFromAlbum(ctx, c.album, c.assets); err != nil {
			t.Fatalf("RemoveFromAlbum(%q, %q) error = %v, want nil", c.album, c.assets, err)
		}
	}
	want = photoLibWant()
	want.Assets[1].Hidden = false
	want.Assets[1].Taken = early
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after RemoveFromAlbum() = %+v, want %+v", got, want)
	}

	if got, want := lib.Counters().Calls, map[string]int{"AddToAlbum": 3, "RemoveFromAlbum": 2, "Snapshot": 2}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryMembersRejects(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, op, album string
		assets          []string
	}{
		{name: "add_to_folder", op: "AddToAlbum", album: "folder-1", assets: []string{"uuid-1"}},
		{name: "add_to_trashed_album", op: "AddToAlbum", album: "album-2", assets: []string{"uuid-1"}},
		{name: "add_to_unknown", op: "AddToAlbum", album: "album-9", assets: []string{"uuid-1"}},
		{name: "add_none_to_unknown", op: "AddToAlbum", album: "album-9", assets: nil},
		{name: "add_to_empty", op: "AddToAlbum", album: "", assets: []string{"uuid-1"}},
		{name: "add_hidden", op: "AddToAlbum", album: "album-3", assets: []string{"uuid-1", "uuid-2"}},
		{name: "add_trashed", op: "AddToAlbum", album: "album-3", assets: []string{"uuid-1", "uuid-3"}},
		{name: "add_unknown", op: "AddToAlbum", album: "album-3", assets: []string{"uuid-1", "uuid-9"}},
		{name: "remove_from_folder", op: "RemoveFromAlbum", album: "folder-2", assets: []string{"uuid-1"}},
		{name: "remove_from_trashed_album", op: "RemoveFromAlbum", album: "album-2", assets: []string{"uuid-1"}},
		{name: "remove_from_unknown", op: "RemoveFromAlbum", album: "album-9", assets: []string{"uuid-1"}},
		{name: "remove_hidden", op: "RemoveFromAlbum", album: "album-1", assets: []string{"uuid-1", "uuid-2"}},
		{name: "remove_trashed", op: "RemoveFromAlbum", album: "album-1", assets: []string{"uuid-1", "uuid-3"}},
		{name: "remove_unknown", op: "RemoveFromAlbum", album: "album-1", assets: []string{"uuid-1", "uuid-9"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			call := lib.AddToAlbum
			if c.op == "RemoveFromAlbum" {
				call = lib.RemoveFromAlbum
			}
			if err := call(t.Context(), c.album, c.assets); !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("%s(%q, %q) error = %v, want %v", c.op, c.album, c.assets, err, domain.ErrNotFound)
			}
			if got := lib.Counters().Calls[c.op]; got != 1 {
				t.Errorf("Counters().Calls[%q] = %d, want 1", c.op, got)
			}
			if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed the library: Snapshot() = %+v, want %+v", c.op, got, want)
			}
		})
	}
}

func TestFakePhotoLibraryMembersFail(t *testing.T) {
	t.Parallel()

	modes := []struct {
		name string
		arm  func(t *testing.T, lib *testkit.FakePhotoLibrary, op string) (context.Context, error)
	}{
		{name: "canceled", arm: func(t *testing.T, _ *testkit.FakePhotoLibrary, _ string) (context.Context, error) {
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			return ctx, context.Canceled
		}},
		{name: "fail_next", arm: func(t *testing.T, lib *testkit.FakePhotoLibrary, op string) (context.Context, error) {
			boom := errors.New("boom")
			lib.FailNext(op, boom)
			return t.Context(), boom
		}},
	}
	for _, c := range []struct {
		op, album string
	}{
		{op: "AddToAlbum", album: "album-3"},
		{op: "RemoveFromAlbum", album: "album-1"},
	} {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			for _, m := range modes {
				t.Run(m.name, func(t *testing.T) {
					t.Parallel()

					lib := testkit.NewFakePhotoLibrary(photoLibSeed())
					call := lib.AddToAlbum
					if c.op == "RemoveFromAlbum" {
						call = lib.RemoveFromAlbum
					}
					ctx, wantErr := m.arm(t, lib, c.op)
					if err := call(ctx, c.album, []string{"uuid-1"}); !errors.Is(err, wantErr) {
						t.Errorf("%s() error = %v, want %v", c.op, err, wantErr)
					}
					if got, want := lib.Counters().Calls, map[string]int{c.op: 1}; !maps.Equal(got, want) {
						t.Errorf("Counters().Calls = %v, want %v", got, want)
					}
					if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
						t.Errorf("%s changed the library: Snapshot() = %+v, want %+v", c.op, got, want)
					}
				})
			}
		})
	}
}

func TestFakePhotoLibraryRebuild(t *testing.T) {
	t.Parallel()

	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")
	for _, c := range []struct {
		name          string
		keep          bool
		albums        []string
		albumKey      string
		assets, guids []string
	}{
		{
			name:     "keep_guids",
			keep:     true,
			albums:   []string{"fake-uuid-2", "fake-uuid-3", "fake-uuid-4", "fake-uuid-5", "fake-uuid-6"},
			albumKey: "album-guid-1",
			assets:   []string{"fake-uuid-7", "fake-uuid-8", "fake-uuid-9", "fake-uuid-10"},
			guids:    []string{"guid-1", "", "guid-3", ""},
		},
		{
			name:     "new_guids",
			keep:     false,
			albums:   []string{"fake-uuid-2", "fake-uuid-4", "fake-uuid-5", "fake-uuid-6", "fake-uuid-7"},
			albumKey: "fake-guid-3",
			assets:   []string{"fake-uuid-8", "fake-uuid-10", "fake-uuid-11", "fake-uuid-13"},
			guids:    []string{"fake-guid-9", "", "fake-guid-12", ""},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			seed := photoLibSeed()
			seed.Albums[0], seed.Albums[2] = seed.Albums[2], seed.Albums[0]
			seed.Assets[1].AlbumUUIDs = []string{"album-9", "album-3"}
			seed.Assets[2].AlbumUUIDs = []string{"album-2"}
			lib := testkit.NewFakePhotoLibrary(seed)
			if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); err != nil || uuid != "fake-uuid-1" {
				t.Fatalf("Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
			}

			before, err := lib.LibraryIdentity(ctx, "")
			if err != nil {
				t.Fatalf("LibraryIdentity() error = %v, want nil", err)
			}
			lib.Rebuild(c.keep)
			after, err := lib.LibraryIdentity(ctx, "")
			if err != nil {
				t.Fatalf("LibraryIdentity() after Rebuild() error = %v, want nil", err)
			}
			if after == before {
				t.Errorf("Rebuild kept the identity %q", before)
			}

			got, err := lib.Snapshot(ctx, "")
			if err != nil || len(got.Assets) != 4 || len(got.Albums) != 5 {
				t.Fatalf("Snapshot() = %d assets and %d albums, %v; want 4 and 5, nil", len(got.Assets), len(got.Albums), err)
			}
			want := photoLibWant()
			want.Albums[0], want.Albums[2] = want.Albums[2], want.Albums[0]
			for i, u := range c.albums {
				want.Albums[i].UUID = u
			}
			want.Albums[0].ParentUUID = c.albums[1]
			want.Albums[1].ParentUUID = c.albums[2]
			want.Albums[0].Key = c.albumKey
			want.Assets = append(want.Assets, domain.Asset{
				Library:      domain.LibraryPersonal,
				OriginalName: "IMG_0004.HEIC",
				DateSource:   domain.DateICloud,
				Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.HEIC", Size: 7}},
			})
			for i := range want.Assets {
				want.Assets[i].Key = domain.AssetKey{CloudAssetGUID: c.guids[i], UUID: c.assets[i]}
			}
			want.Assets[0].AlbumUUIDs = []string{c.albums[0]}
			want.Assets[1].AlbumUUIDs = []string{"album-9", c.albums[4]}
			want.Assets[2].AlbumUUIDs = []string{c.albums[3]}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("Snapshot() after Rebuild(%t) = %+v, want %+v", c.keep, got, want)
			}

			dir := t.TempDir()
			if _, err := lib.Download(ctx, c.assets[3], dir, nil); err != nil {
				t.Fatalf("Download(%q) error = %v, want nil", c.assets[3], err)
			}
			photoLibDir(t, dir, map[string]string{"original": "photo 4"})

			if got, want := lib.Counters().Calls, map[string]int{"Import": 1, "LibraryIdentity": 2, "Snapshot": 1, "Download": 1}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}
		})
	}
}

func TestFakePhotoLibraryMembersConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if err := lib.AddToAlbum(ctx, "album-3", []string{"uuid-1"}); err != nil {
				t.Errorf("AddToAlbum() error = %v, want nil", err)
				return
			}
			if err := lib.RemoveFromAlbum(ctx, "album-1", []string{"uuid-1"}); err != nil {
				t.Errorf("RemoveFromAlbum() error = %v, want nil", err)
			}
		})
	}
	wg.Wait()

	want := photoLibWant()
	want.Assets[0].AlbumUUIDs = []string{"album-3"}
	want.Albums[2].Start = time.Time{}
	want.Albums[4].Start = time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"AddToAlbum": 8, "RemoveFromAlbum": 8, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryRebuildConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			lib.Rebuild(true)
			if _, err := lib.LibraryIdentity(ctx, ""); err != nil {
				t.Errorf("LibraryIdentity() error = %v, want nil", err)
			}
		})
	}
	wg.Wait()

	s := photoLibSnapshot(t, lib)
	var uuids []string
	for _, a := range s.Albums {
		uuids = append(uuids, a.UUID)
	}
	for _, a := range s.Assets {
		uuids = append(uuids, a.Key.UUID)
	}
	want := []string{"fake-uuid-57", "fake-uuid-58", "fake-uuid-59", "fake-uuid-60", "fake-uuid-61", "fake-uuid-62", "fake-uuid-63", "fake-uuid-64"}
	if !slices.Equal(uuids, want) {
		t.Errorf("UUIDs after eight Rebuild() = %v, want %v", uuids, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"LibraryIdentity": 8, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}
