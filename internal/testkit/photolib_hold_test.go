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

func TestFakePhotoLibraryHold(t *testing.T) {
	t.Parallel()

	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")
	for _, c := range []struct {
		op   string
		call func(context.Context, *testkit.FakePhotoLibrary) (string, error)
		edit func(*domain.LibrarySnapshot)
	}{
		{
			op: "Import",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.Import(ctx, domain.ImportRequest{Photo: heic})
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Assets = append(s.Assets, domain.Asset{
					Key:          domain.AssetKey{UUID: "fake-uuid-1"},
					Library:      domain.LibraryPersonal,
					OriginalName: "IMG_0004.HEIC",
					DateSource:   domain.DateICloud,
					Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.HEIC", Size: 7}},
				})
			},
		},
		{
			op: "CreateFolder",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateFolder(ctx, "New", "folder-1")
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Albums = append(s.Albums, domain.Album{
					UUID: "fake-uuid-1", Kind: domain.KindFolder, Title: "New", ParentUUID: "folder-1", FolderTitles: []string{"Trips"},
				})
			},
		},
		{
			op: "CreateAlbum",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return l.CreateAlbum(ctx, "New", "")
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Albums = append(s.Albums, domain.Album{UUID: "fake-uuid-1", Title: "New"})
			},
		},
		{
			op: "RenameCollection",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RenameCollection(ctx, "folder-1", "Journeys")
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Albums[0].Title = "Journeys"
				s.Albums[1].FolderTitles = []string{"Journeys"}
				s.Albums[2].FolderTitles = []string{"Journeys", "2024"}
			},
		},
		{
			op: "AddToAlbum",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.AddToAlbum(ctx, "album-3", []string{"uuid-1"})
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Assets[0].AlbumUUIDs = []string{"album-1", "album-3"}
				s.Albums[4].Start = time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
			},
		},
		{
			op: "RemoveFromAlbum",
			call: func(ctx context.Context, l *testkit.FakePhotoLibrary) (string, error) {
				return "", l.RemoveFromAlbum(ctx, "album-1", []string{"uuid-1"})
			},
			edit: func(s *domain.LibrarySnapshot) {
				s.Assets[0].AlbumUUIDs = nil
				s.Albums[2].Start = time.Time{}
			},
		},
	} {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			for _, a := range []struct {
				name string
				yes  bool
			}{
				{name: "yes", yes: true},
				{name: "no", yes: false},
			} {
				t.Run(a.name, func(t *testing.T) {
					t.Parallel()

					lib := testkit.NewFakePhotoLibrary(photoLibSeed())
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					lib.Hold(c.op, func() {
						if s, err := lib.Snapshot(t.Context(), ""); err != nil || !reflect.DeepEqual(s, photoLibWant()) {
							t.Errorf("%s took effect before its outcome: Snapshot() = %+v, %v; want %+v, nil", c.op, s, err, photoLibWant())
						}
						cancel()
					})

					uuid, err := c.call(ctx, lib)
					if !errors.Is(err, domain.ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
						t.Errorf("%s() error = %v, want %v with %v", c.op, err, domain.ErrOutcomeUnknown, context.Canceled)
					}
					if uuid != "" {
						t.Errorf("%s() = %q, want an empty UUID", c.op, uuid)
					}

					lib.Answer(c.op, a.yes)
					lib.Answer(c.op, true)
					want := photoLibWant()
					if a.yes {
						c.edit(&want)
					}
					got, err := lib.Snapshot(t.Context(), "")
					if err != nil {
						t.Fatalf("Snapshot() error = %v, want nil", err)
					}
					if !reflect.DeepEqual(got, want) {
						t.Errorf("Snapshot() after Answer(%q, %t) = %+v, want %+v", c.op, a.yes, got, want)
					}
					if got, want := lib.Counters().Calls, map[string]int{c.op: 1, "Snapshot": 2}; !maps.Equal(got, want) {
						t.Errorf("Counters().Calls = %v, want %v", got, want)
					}
				})
			}
		})
	}
}

func TestFakePhotoLibraryHoldOnce(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	heic := photoLibFile(t, t.TempDir(), "IMG_0004.HEIC", "photo 4")
	importCtx, cancelImport := context.WithCancel(ctx)
	defer cancelImport()
	albumCtx, cancelAlbum := context.WithCancel(ctx)
	defer cancelAlbum()
	lib.Hold("Import", cancelImport)
	lib.Hold("CreateAlbum", cancelAlbum)

	uuid, err := lib.CreateAlbum(albumCtx, "", "")
	if err == nil || errors.Is(err, domain.ErrOutcomeUnknown) || uuid != "" {
		t.Errorf("CreateAlbum() of an empty title = %q, %v; want an empty UUID and an error of the title", uuid, err)
	}
	uuid, err = lib.Import(importCtx, domain.ImportRequest{Photo: heic})
	if uuid != "" || !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Errorf("held Import() = %q, %v; want an empty UUID, %v", uuid, err, domain.ErrOutcomeUnknown)
	}
	uuid, err = lib.CreateAlbum(albumCtx, "Held", "")
	if uuid != "" || !errors.Is(err, domain.ErrOutcomeUnknown) {
		t.Errorf("held CreateAlbum() = %q, %v; want an empty UUID, %v", uuid, err, domain.ErrOutcomeUnknown)
	}
	if uuid, err := lib.CreateAlbum(ctx, "Next", ""); err != nil || uuid != "fake-uuid-1" {
		t.Fatalf("CreateAlbum() after the held one = %q, %v; want %q, nil", uuid, err, "fake-uuid-1")
	}

	lib.Answer("CreateAlbum", true)
	lib.Answer("Import", true)
	lib.Answer("Import", true)

	lib.Hold("Import", cancelImport)
	lib.Hold("Import", nil)
	if uuid, err := lib.Import(ctx, domain.ImportRequest{Photo: heic}); err != nil || uuid != "fake-uuid-4" {
		t.Fatalf("Import() after Hold(%q, nil) = %q, %v; want %q, nil", "Import", uuid, err, "fake-uuid-4")
	}

	got, err := lib.Snapshot(ctx, "")
	if err != nil || len(got.Assets) != 5 || len(got.Albums) != 7 {
		t.Fatalf("Snapshot() = %d assets and %d albums, %v; want 5 and 7, nil", len(got.Assets), len(got.Albums), err)
	}
	want := photoLibWant()
	want.Albums = append(want.Albums, domain.Album{UUID: "fake-uuid-1", Title: "Next"}, domain.Album{UUID: "fake-uuid-2", Title: "Held"})
	for _, u := range []string{"fake-uuid-3", "fake-uuid-4"} {
		want.Assets = append(want.Assets, domain.Asset{
			Key:          domain.AssetKey{UUID: u},
			Library:      domain.LibraryPersonal,
			OriginalName: "IMG_0004.HEIC",
			DateSource:   domain.DateICloud,
			Resources:    []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.HEIC", Size: 7}},
		})
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() = %+v, want %+v", got, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"CreateAlbum": 3, "Import": 2, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}

func TestFakePhotoLibraryHoldConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			lib.Hold("CreateAlbum", nil)
			lib.Answer("CreateAlbum", true)
			if uuid, err := lib.CreateAlbum(ctx, "Day", ""); err != nil || uuid == "" {
				t.Errorf("CreateAlbum() = %q, %v; want a UUID, nil", uuid, err)
			}
		})
	}
	wg.Wait()

	s, err := lib.Snapshot(ctx, "")
	if err != nil || len(s.Albums) != 13 {
		t.Fatalf("Snapshot() = %d albums, %v; want 13 albums, nil", len(s.Albums), err)
	}
	var uuids []string
	for _, a := range s.Albums[5:] {
		uuids = append(uuids, a.UUID)
	}
	slices.Sort(uuids)
	want := []string{"fake-uuid-1", "fake-uuid-2", "fake-uuid-3", "fake-uuid-4", "fake-uuid-5", "fake-uuid-6", "fake-uuid-7", "fake-uuid-8"}
	if !slices.Equal(uuids, want) {
		t.Errorf("UUIDs of the created albums = %v, want %v", uuids, want)
	}
	if got, want := lib.Counters().Calls, map[string]int{"CreateAlbum": 8, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}
