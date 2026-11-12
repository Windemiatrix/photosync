package testkit_test

import (
	"context"
	"errors"
	"maps"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

func TestFakePhotoLibraryCreate(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	for _, c := range []struct {
		op            string
		create        func(context.Context, string, string) (string, error)
		title, parent string
		want          string
	}{
		{op: "CreateFolder", create: lib.CreateFolder, title: "Café", want: "fake-uuid-1"},
		{op: "CreateFolder", create: lib.CreateFolder, title: "2025", parent: "fake-uuid-1", want: "fake-uuid-2"},
		{op: "CreateAlbum", create: lib.CreateAlbum, title: "06/01 Beach", parent: "fake-uuid-2", want: "fake-uuid-3"},
		{op: "CreateAlbum", create: lib.CreateAlbum, title: "Notes", want: "fake-uuid-4"},
		{op: "CreateAlbum", create: lib.CreateAlbum, title: "Day", parent: "folder-2", want: "fake-uuid-5"},
	} {
		if uuid, err := c.create(ctx, c.title, c.parent); err != nil || uuid != c.want {
			t.Fatalf("%s(%q, %q) = %q, %v; want %q, nil", c.op, c.title, c.parent, uuid, err, c.want)
		}
	}

	got, err := lib.Snapshot(ctx, "")
	if err != nil || len(got.Assets) != 3 || len(got.Albums) != 10 {
		t.Fatalf("Snapshot() = %d assets and %d albums, %v; want 3 and 10, nil", len(got.Assets), len(got.Albums), err)
	}
	want := photoLibWant()
	want.Albums = append(want.Albums,
		domain.Album{UUID: "fake-uuid-1", Kind: domain.KindFolder, Title: "Café"},
		domain.Album{UUID: "fake-uuid-2", Kind: domain.KindFolder, Title: "2025", ParentUUID: "fake-uuid-1", FolderTitles: []string{"Café"}},
		domain.Album{UUID: "fake-uuid-3", Title: "06/01 Beach", ParentUUID: "fake-uuid-2", FolderTitles: []string{"Café", "2025"}},
		domain.Album{UUID: "fake-uuid-4", Title: "Notes"},
		domain.Album{UUID: "fake-uuid-5", Title: "Day", ParentUUID: "folder-2", FolderTitles: []string{"Trips", "2024"}},
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after CreateFolder() and CreateAlbum() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"CreateFolder": 2, "CreateAlbum": 3, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryRename(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	if err := lib.RenameCollection(ctx, "folder-1", "Journeys"); err != nil {
		t.Fatalf("RenameCollection() of a folder error = %v, want nil", err)
	}
	if err := lib.RenameCollection(ctx, "album-1", "05/02 Café"); err != nil {
		t.Fatalf("RenameCollection() of an album error = %v, want nil", err)
	}

	want := photoLibWant()
	want.Albums[0].Title = "Journeys"
	want.Albums[1].FolderTitles = []string{"Journeys"}
	want.Albums[2].Title = "05/02 Café"
	want.Albums[2].FolderTitles = []string{"Journeys", "2024"}
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after RenameCollection() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"RenameCollection": 2, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryCollectionRejects(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, op string
		call     func(context.Context, *testkit.FakePhotoLibrary) (string, error)
		notFound bool
	}{
		{
			name: "create_folder_empty_title", op: "CreateFolder",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateFolder(ctx, "", "")
			},
		},
		{
			name: "create_album_nul_title", op: "CreateAlbum",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateAlbum(ctx, "New\x00", "")
			},
		},
		{
			name: "create_folder_invalid_utf8", op: "CreateFolder",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateFolder(ctx, "New\xff", "")
			},
		},
		{
			name: "create_folder_in_album", op: "CreateFolder", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateFolder(ctx, "New", "album-1")
			},
		},
		{
			name: "create_album_in_trashed_folder", op: "CreateAlbum", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateAlbum(ctx, "New", "folder-2")
			},
		},
		{
			name: "create_album_in_unknown", op: "CreateAlbum", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateAlbum(ctx, "New", "folder-9")
			},
		},
		{
			name: "rename_empty_title", op: "RenameCollection",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "album-1", "")
			},
		},
		{
			name: "rename_nul_title", op: "RenameCollection",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "folder-1", "New\x00")
			},
		},
		{
			name: "rename_invalid_utf8", op: "RenameCollection",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "album-3", "New\xff")
			},
		},
		{
			name: "rename_unknown", op: "RenameCollection", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "folder-9", "New")
			},
		},
		{
			name: "rename_trashed_album", op: "RenameCollection", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "album-2", "New")
			},
		},
		{
			name: "rename_trashed_folder", op: "RenameCollection", notFound: true,
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "folder-2", "New")
			},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			seed := photoLibSeed()
			seed.Albums[1].InTrash = true
			lib := testkit.NewFakePhotoLibrary(seed)

			uuid, err := c.call(ctx, lib)
			if err == nil || uuid != "" {
				t.Errorf("%s() = %q, %v; want an empty UUID and an error", c.op, uuid, err)
			}
			if c.notFound && !errors.Is(err, domain.ErrNotFound) {
				t.Errorf("%s() error = %v, want %v", c.op, err, domain.ErrNotFound)
			}
			if got := lib.Counters().Calls[c.op]; got != 1 {
				t.Errorf("Counters().Calls[%q] = %d, want 1", c.op, got)
			}
			want := photoLibWant()
			want.Albums[1].InTrash = true
			if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed the library: Snapshot() = %+v, want %+v", c.op, got, want)
			}
			if uuid, err := lib.CreateAlbum(ctx, "New", ""); err != nil || uuid != "fake-uuid-1" {
				t.Errorf("next CreateAlbum() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
			}
		})
	}
}

func TestFakePhotoLibraryCollectionsFail(t *testing.T) {
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
		op   string
		call func(context.Context, *testkit.FakePhotoLibrary) (string, error)
	}{
		{op: "CreateFolder", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
			return l.CreateFolder(ctx, "New", "")
		}},
		{op: "CreateAlbum", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
			return l.CreateAlbum(ctx, "New", "folder-1")
		}},
		{op: "RenameCollection", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
			return "", l.RenameCollection(ctx, "album-1", "New")
		}},
	} {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			for _, m := range modes {
				t.Run(m.name, func(t *testing.T) {
					t.Parallel()

					lib := testkit.NewFakePhotoLibrary(photoLibSeed())
					ctx, wantErr := m.arm(t, lib, c.op)
					uuid, err := c.call(ctx, lib)
					if !errors.Is(err, wantErr) {
						t.Errorf("%s() error = %v, want %v", c.op, err, wantErr)
					}
					if uuid != "" {
						t.Errorf("%s() = %q, want an empty UUID", c.op, uuid)
					}
					if got, want := lib.Counters().Calls, map[string]int{c.op: 1}; !maps.Equal(got, want) {
						t.Errorf("Counters().Calls = %v, want %v", got, want)
					}
					if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
						t.Errorf("%s changed the library: Snapshot() = %+v, want %+v", c.op, got, want)
					}
					if uuid, err := lib.CreateAlbum(t.Context(), "New", ""); err != nil || uuid != "fake-uuid-1" {
						t.Errorf("next CreateAlbum() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
					}
				})
			}
		})
	}
}

func TestFakePhotoLibraryImportAlbums(t *testing.T) {
	t.Parallel()

	src := t.TempDir()
	heic := photoLibFile(t, src, "IMG_0004.HEIC", "photo 4")
	missing := filepath.Join(src, "IMG_0009.HEIC")

	for _, c := range []struct {
		name string
		req  domain.ImportRequest
	}{
		{name: "folder", req: domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{"album-1", "folder-1"}}},
		{name: "trashed_album", req: domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{"album-2"}}},
		{name: "unknown_album", req: domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{"album-9"}}},
		{name: "before_files", req: domain.ImportRequest{Photo: missing, AlbumUUIDs: []string{"album-9"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			if uuid, err := lib.Import(ctx, c.req); !errors.Is(err, domain.ErrNotFound) || uuid != "" {
				t.Errorf("Import() = %q, %v; want an empty UUID, %v", uuid, err, domain.ErrNotFound)
			}
			if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
				t.Errorf("Import created an asset: Snapshot() = %+v, want %+v", got, want)
			}
			req := domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{"album-1", "album-3"}}
			if uuid, err := lib.Import(ctx, req); err != nil || uuid != "fake-uuid-1" {
				t.Errorf("next Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
			}
			if got, want := lib.Counters().Calls, map[string]int{"Import": 2, "Snapshot": 1}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}
		})
	}
}

func TestFakePhotoLibrarySnapshotAlbums(t *testing.T) {
	t.Parallel()

	seed := photoLibSeed()
	seed.Albums[1].InTrash = true
	seed.Albums[3].Start = time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	seed.Assets[1].Taken = time.Date(2024, 4, 30, 8, 0, 0, 0, time.UTC)
	seed.Assets[1].AlbumUUIDs = []string{"album-1", "album-3"}
	seed.Assets[2].Taken = time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	seed.Assets[2].AlbumUUIDs = []string{"album-3"}
	seed.Assets = append(seed.Assets, domain.Asset{
		Key:          domain.AssetKey{UUID: "uuid-4"},
		Library:      domain.LibraryPersonal,
		OriginalName: "IMG_0004.HEIC",
		AlbumUUIDs:   []string{"album-3"},
	})

	lib := testkit.NewFakePhotoLibrary(seed)
	got, err := lib.Snapshot(t.Context(), "")
	if err != nil || len(got.Assets) != 4 || len(got.Albums) != 5 {
		t.Fatalf("Snapshot() = %d assets and %d albums, %v; want 4 and 5, nil", len(got.Assets), len(got.Albums), err)
	}

	want := photoLibWant()
	want.Assets[1].Taken = time.Date(2024, 4, 30, 8, 0, 0, 0, time.UTC)
	want.Assets[1].AlbumUUIDs = []string{"album-1", "album-3"}
	want.Assets[2].Taken = time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	want.Assets[2].AlbumUUIDs = []string{"album-3"}
	want.Assets = append(want.Assets, domain.Asset{
		Key:          domain.AssetKey{UUID: "uuid-4"},
		Library:      domain.LibraryPersonal,
		OriginalName: "IMG_0004.HEIC",
		AlbumUUIDs:   []string{"album-3"},
	})
	want.Albums[1].InTrash = true
	want.Albums[2].Start = time.Date(2024, 4, 30, 8, 0, 0, 0, time.UTC)
	want.Albums[4].Start = time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
}

func TestFakePhotoLibraryNumbers(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")

	early := domain.ImportRequest{
		Photo:      heic,
		Taken:      time.Date(2024, 4, 1, 12, 0, 0, 0, time.FixedZone("", 3*60*60)),
		AlbumUUIDs: []string{"album-1"},
	}
	if uuid, err := lib.Import(ctx, early); err != nil || uuid != "fake-uuid-1" {
		t.Fatalf("Import() = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
	}
	lib.SyncCloud()
	if uuid, err := lib.CreateFolder(ctx, "2025", ""); err != nil || uuid != "fake-uuid-8" {
		t.Fatalf("CreateFolder() = %q, %v; want %q, nil", uuid, err, "fake-uuid-8")
	}
	if uuid, err := lib.CreateAlbum(ctx, "06/01 Beach", "fake-uuid-8"); err != nil || uuid != "fake-uuid-9" {
		t.Fatalf("CreateAlbum() = %q, %v; want %q, nil", uuid, err, "fake-uuid-9")
	}
	if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{"fake-uuid-9"}}); err != nil || uuid != "fake-uuid-10" {
		t.Fatalf("Import() into the new album = %q, %v; want %q, nil", uuid, err, "fake-uuid-10")
	}
	lib.SyncCloud()

	got, err := lib.Snapshot(ctx, "")
	if err != nil || len(got.Assets) != 5 || len(got.Albums) != 7 {
		t.Fatalf("Snapshot() = %d assets and %d albums, %v; want 5 and 7, nil", len(got.Assets), len(got.Albums), err)
	}
	want := photoLibWant()
	want.Assets[1].Key.CloudAssetGUID = "fake-guid-2"
	want.Assets[1].InCloud = new(true)
	for i, key := range map[int]string{0: "fake-guid-4", 1: "fake-guid-5", 3: "fake-guid-6", 4: "fake-guid-7"} {
		want.Albums[i].Key, want.Albums[i].KeySource = key, domain.KeyCloudGUID
	}
	want.Albums[2].Start = time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC)
	want.Assets = append(want.Assets,
		domain.Asset{
			Key:          domain.AssetKey{CloudAssetGUID: "fake-guid-3", UUID: "fake-uuid-1"},
			Library:      domain.LibraryPersonal,
			OriginalName: "IMG_0004.HEIC",
			Taken:        time.Date(2024, 4, 1, 9, 0, 0, 0, time.UTC),
			DateSource:   domain.DateICloud,
			InCloud:      new(true),
			Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.HEIC", Size: 7}},
			AlbumUUIDs:   []string{"album-1"},
		},
		domain.Asset{
			Key:          domain.AssetKey{CloudAssetGUID: "fake-guid-11", UUID: "fake-uuid-10"},
			Library:      domain.LibraryPersonal,
			OriginalName: "IMG_0004.HEIC",
			DateSource:   domain.DateICloud,
			InCloud:      new(true),
			Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.HEIC", Size: 7}},
			AlbumUUIDs:   []string{"fake-uuid-9"},
		},
	)
	want.Albums = append(want.Albums,
		domain.Album{UUID: "fake-uuid-8", Key: "fake-guid-12", KeySource: domain.KeyCloudGUID, Kind: domain.KindFolder, Title: "2025"},
		domain.Album{
			UUID: "fake-uuid-9", Key: "fake-guid-13", KeySource: domain.KeyCloudGUID, Title: "06/01 Beach",
			ParentUUID: "fake-uuid-8", FolderTitles: []string{"2025"},
		},
	)
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"Import": 2, "CreateFolder": 1, "CreateAlbum": 1, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryAlbumsConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			folder, err := lib.CreateFolder(ctx, "Trips", "")
			if err != nil || folder == "" {
				t.Errorf("CreateFolder() = %q, %v; want a UUID, nil", folder, err)
				return
			}
			album, err := lib.CreateAlbum(ctx, "Day", folder)
			if err != nil || album == "" {
				t.Errorf("CreateAlbum() = %q, %v; want a UUID, nil", album, err)
				return
			}
			if err := lib.RenameCollection(ctx, album, "Night"); err != nil {
				t.Errorf("RenameCollection() error = %v, want nil", err)
				return
			}
			if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic, AlbumUUIDs: []string{album}}); err != nil || uuid == "" {
				t.Errorf("Import() = %q, %v; want a UUID, nil", uuid, err)
			}
		})
	}
	wg.Wait()

	s, err := lib.Snapshot(ctx, "")
	if err != nil || len(s.Albums) != 21 || len(s.Assets) != 11 {
		t.Fatalf("Snapshot() = %d albums and %d assets, %v; want 21 and 11, nil", len(s.Albums), len(s.Assets), err)
	}
	uuids := map[string]bool{}
	night := 0
	for _, a := range s.Albums {
		uuids[a.UUID] = true
		if a.Title == "Night" && len(a.FolderTitles) == 1 && a.FolderTitles[0] == "Trips" {
			night++
		}
	}
	for _, a := range s.Assets {
		uuids[a.Key.UUID] = true
	}
	if len(uuids) != 32 || night != 8 {
		t.Errorf("Snapshot() has %d distinct UUIDs and %d albums Night in Trips; want 32 and 8", len(uuids), night)
	}
	if got, want := lib.Counters().Calls, map[string]int{
		"CreateFolder": 8, "CreateAlbum": 8, "RenameCollection": 8, "Import": 8, "Snapshot": 1,
	}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}
