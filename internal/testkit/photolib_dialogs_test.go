package testkit_test

import (
	"context"
	"errors"
	"maps"
	"reflect"
	"sync"
	"testing"

	"github.com/Windemiatrix/photosync/internal/app"
	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

var _ app.PhotoLibrary = (*testkit.FakePhotoLibrary)(nil)

var photoLibDialogs = []struct {
	op   string
	call func(context.Context, *testkit.FakePhotoLibrary) error
	edit func(*domain.LibrarySnapshot)
}{
	{
		op: "DeleteAssets",
		call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteAssets(ctx, []string{"uuid-1"})
		},
		edit: func(s *domain.LibrarySnapshot) { s.Assets[0].InTrash = true },
	},
	{
		op: "UpdateAsset",
		call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{Hidden: new(true)})
		},
		edit: func(s *domain.LibrarySnapshot) { s.Assets[0].Hidden = true },
	},
	{
		op: "DeleteCollections",
		call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteCollections(ctx, []string{"folder-2"})
		},
		edit: func(s *domain.LibrarySnapshot) {
			s.Albums[1].InTrash = true
			s.Albums[2].InTrash = true
		},
	},
}

func TestFakePhotoLibraryDeleteAssets(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	seed := photoLibSeed()
	seed.Assets[1].Hidden = false
	lib := testkit.NewFakePhotoLibrary(seed)

	if err := lib.DeleteAssets(ctx, []string{"uuid-2", "uuid-1"}); err != nil {
		t.Fatalf("DeleteAssets() error = %v, want nil", err)
	}
	if err := lib.DeleteAssets(ctx, nil); err != nil {
		t.Fatalf("DeleteAssets(nil) error = %v, want nil", err)
	}

	cnt := lib.Counters()
	if cnt.Dialogs != 1 {
		t.Errorf("Counters().Dialogs = %d, want 1: one dialog for the batch and none for an empty one", cnt.Dialogs)
	}
	if got, want := cnt.Calls, map[string]int{"DeleteAssets": 2}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
	want := photoLibWant()
	want.Assets[1].Hidden = false
	want.Assets[0].InTrash = true
	want.Assets[1].InTrash = true
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after DeleteAssets() = %+v, want %+v", got, want)
	}
}

func TestFakePhotoLibraryDeleteCollections(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name   string
		before []int
		uuids  []string
		after  []int
	}{
		{name: "folder", before: nil, uuids: []string{"folder-1"}, after: []int{0, 1, 2}},
		{name: "album", before: nil, uuids: []string{"album-1"}, after: []int{2}},
		{name: "two", before: nil, uuids: []string{"folder-2", "album-3"}, after: []int{1, 2, 4}},
		{name: "under_trashed", before: []int{1}, uuids: []string{"folder-1"}, after: []int{0, 1, 2}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			seed := photoLibSeed()
			want := photoLibWant()
			for _, i := range c.before {
				seed.Albums[i].InTrash = true
				want.Albums[i].InTrash = true
			}
			for _, i := range c.after {
				want.Albums[i].InTrash = true
			}
			lib := testkit.NewFakePhotoLibrary(seed)

			if err := lib.DeleteCollections(t.Context(), c.uuids); err != nil {
				t.Errorf("DeleteCollections(%q) error = %v, want nil", c.uuids, err)
			}
			cnt := lib.Counters()
			if cnt.Dialogs != 1 {
				t.Errorf("Counters().Dialogs = %d, want 1", cnt.Dialogs)
			}
			if got, want := cnt.Calls, map[string]int{"DeleteCollections": 1}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}
			if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
				t.Errorf("Snapshot() after DeleteCollections(%q) = %+v, want %+v", c.uuids, got, want)
			}
		})
	}
}

func TestFakePhotoLibraryHide(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())

	if err := lib.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{Hidden: new(true)}); err != nil {
		t.Fatalf("UpdateAsset() hiding error = %v, want nil", err)
	}
	if err := lib.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{Hidden: new(false)}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateAsset() showing a hidden asset error = %v, want %v: a hidden asset cannot be shown again", err, domain.ErrNotFound)
	}
	if _, err := lib.Download(ctx, "uuid-1", t.TempDir(), nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Download() of a hidden asset error = %v, want %v", err, domain.ErrNotFound)
	}

	cnt := lib.Counters()
	if cnt.Dialogs != 1 {
		t.Errorf("Counters().Dialogs = %d, want 1", cnt.Dialogs)
	}
	if got, want := cnt.Calls, map[string]int{"UpdateAsset": 2, "Download": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
	want := photoLibWant()
	want.Assets[0].Hidden = true
	if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
		t.Errorf("Snapshot() after hiding = %+v, want %+v", got, want)
	}
}

func TestFakePhotoLibraryDialogRejects(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, op string
		call     func(context.Context, *testkit.FakePhotoLibrary) error
		err      error
	}{
		{name: "delete_assets_hidden", op: "DeleteAssets", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteAssets(ctx, []string{"uuid-1", "uuid-2"})
		}, err: domain.ErrNotFound},
		{name: "delete_assets_trashed", op: "DeleteAssets", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteAssets(ctx, []string{"uuid-1", "uuid-3"})
		}, err: domain.ErrNotFound},
		{name: "delete_assets_unknown", op: "DeleteAssets", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteAssets(ctx, []string{"uuid-1", "uuid-9"})
		}, err: domain.ErrNotFound},
		{name: "delete_assets_none", op: "DeleteAssets", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteAssets(ctx, nil)
		}, err: nil},
		{name: "update_hidden", op: "UpdateAsset", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-2", domain.AssetPatch{Hidden: new(true)})
		}, err: domain.ErrNotFound},
		{name: "update_trashed", op: "UpdateAsset", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-3", domain.AssetPatch{Hidden: new(true)})
		}, err: domain.ErrNotFound},
		{name: "update_unknown", op: "UpdateAsset", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-9", domain.AssetPatch{})
		}, err: domain.ErrNotFound},
		{name: "update_zero", op: "UpdateAsset", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{})
		}, err: nil},
		{name: "update_show", op: "UpdateAsset", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{Hidden: new(false)})
		}, err: nil},
		{name: "delete_trashed_collection", op: "DeleteCollections", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteCollections(ctx, []string{"folder-1", "album-2"})
		}, err: domain.ErrNotFound},
		{name: "delete_unknown_collection", op: "DeleteCollections", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteCollections(ctx, []string{"folder-1", "folder-9"})
		}, err: domain.ErrNotFound},
		{name: "delete_no_collections", op: "DeleteCollections", call: func(ctx context.Context, l *testkit.FakePhotoLibrary) error {
			return l.DeleteCollections(ctx, nil)
		}, err: nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			if err := c.call(t.Context(), lib); !errors.Is(err, c.err) {
				t.Errorf("%s() error = %v, want %v", c.op, err, c.err)
			}
			cnt := lib.Counters()
			if cnt.Dialogs != 0 {
				t.Errorf("Counters().Dialogs = %d, want 0", cnt.Dialogs)
			}
			if got, want := cnt.Calls, map[string]int{c.op: 1}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}
			if got, want := photoLibSnapshot(t, lib), photoLibWant(); !reflect.DeepEqual(got, want) {
				t.Errorf("%s changed the library: Snapshot() = %+v, want %+v", c.op, got, want)
			}
		})
	}
}

func TestFakePhotoLibraryDialogFail(t *testing.T) {
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
	for _, c := range photoLibDialogs {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			for _, m := range modes {
				t.Run(m.name, func(t *testing.T) {
					t.Parallel()

					lib := testkit.NewFakePhotoLibrary(photoLibSeed())
					ctx, wantErr := m.arm(t, lib, c.op)
					if err := c.call(ctx, lib); !errors.Is(err, wantErr) {
						t.Errorf("%s() error = %v, want %v", c.op, err, wantErr)
					}
					cnt := lib.Counters()
					if cnt.Dialogs != 0 {
						t.Errorf("Counters().Dialogs = %d, want 0", cnt.Dialogs)
					}
					if got, want := cnt.Calls, map[string]int{c.op: 1}; !maps.Equal(got, want) {
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

func TestFakePhotoLibraryDialogAnswer(t *testing.T) {
	t.Parallel()

	for _, c := range photoLibDialogs {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			ctx := t.Context()
			want := photoLibWant()
			c.edit(&want)

			lib := testkit.NewFakePhotoLibrary(photoLibSeed())
			lib.Answer(c.op, false)
			if err := c.call(ctx, lib); !errors.Is(err, domain.ErrUserCanceled) {
				t.Errorf("%s() after Answer(%q, false) error = %v, want %v", c.op, c.op, err, domain.ErrUserCanceled)
			}
			if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, photoLibWant()) {
				t.Errorf("%s changed the library although the user refused: Snapshot() = %+v, want %+v", c.op, got, photoLibWant())
			}
			if err := c.call(ctx, lib); err != nil {
				t.Errorf("second %s() error = %v, want nil: the no answered more than one dialog", c.op, err)
			}
			if got := photoLibSnapshot(t, lib); !reflect.DeepEqual(got, want) {
				t.Errorf("Snapshot() after the second %s() = %+v, want %+v", c.op, got, want)
			}
			cnt := lib.Counters()
			if cnt.Dialogs != 2 {
				t.Errorf("Counters().Dialogs = %d, want 2: the refused dialog counts too", cnt.Dialogs)
			}
			if got, want := cnt.Calls, map[string]int{c.op: 2, "Snapshot": 2}; !maps.Equal(got, want) {
				t.Errorf("Counters().Calls = %v, want %v", got, want)
			}

			back := testkit.NewFakePhotoLibrary(photoLibSeed())
			back.Answer(c.op, false)
			back.Answer(c.op, true)
			back.Answer("Import", false)
			if err := c.call(ctx, back); err != nil {
				t.Errorf("%s() error = %v, want nil: a yes did not take back the no, or the no of another method refused it", c.op, err)
			}
			if got := photoLibSnapshot(t, back); !reflect.DeepEqual(got, want) {
				t.Errorf("Snapshot() after %s() = %+v, want %+v", c.op, got, want)
			}
			if got := back.Counters().Dialogs; got != 1 {
				t.Errorf("Counters().Dialogs = %d, want 1", got)
			}
		})
	}
}

func TestFakePhotoLibraryDialogHold(t *testing.T) {
	t.Parallel()

	for _, c := range photoLibDialogs {
		t.Run(c.op, func(t *testing.T) {
			t.Parallel()

			for _, a := range []struct {
				name    string
				yes     bool
				again   error
				dialogs int
			}{
				{name: "yes", yes: true, again: domain.ErrNotFound, dialogs: 1},
				{name: "no", yes: false, again: nil, dialogs: 2},
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

					if err := c.call(ctx, lib); !errors.Is(err, domain.ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
						t.Errorf("%s() error = %v, want %v with %v", c.op, err, domain.ErrOutcomeUnknown, context.Canceled)
					}

					lib.Answer(c.op, a.yes)
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

					err = c.call(t.Context(), lib)
					if !errors.Is(err, a.again) {
						t.Errorf("after Answer(%q, %t) the next %s() error = %v, want %v", c.op, a.yes, c.op, err, a.again)
					}
					cnt := lib.Counters()
					if cnt.Dialogs != a.dialogs {
						t.Errorf("Counters().Dialogs = %d, want %d", cnt.Dialogs, a.dialogs)
					}
					if got, want := cnt.Calls, map[string]int{c.op: 2, "Snapshot": 2}; !maps.Equal(got, want) {
						t.Errorf("Counters().Calls = %v, want %v", got, want)
					}
				})
			}
		})
	}
}

func TestFakePhotoLibraryDialogConcurrent(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(photoLibSeed())
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			album, err := lib.CreateAlbum(ctx, "Day", "")
			if err != nil || album == "" {
				t.Errorf("CreateAlbum() = %q, %v; want a UUID, nil", album, err)
				return
			}
			if err := lib.DeleteCollections(ctx, []string{album}); err != nil {
				t.Errorf("DeleteCollections(%q) error = %v, want nil", album, err)
				return
			}
			if err := lib.UpdateAsset(ctx, "uuid-1", domain.AssetPatch{Hidden: new(false)}); err != nil {
				t.Errorf("UpdateAsset() error = %v, want nil", err)
				return
			}
			if err := lib.DeleteAssets(ctx, nil); err != nil {
				t.Errorf("DeleteAssets(nil) error = %v, want nil", err)
				return
			}
			lib.Answer("DeleteCollections", true)
			lib.Counters()
		})
	}
	wg.Wait()

	s, err := lib.Snapshot(ctx, "")
	if err != nil || len(s.Albums) != 13 {
		t.Fatalf("Snapshot() = %d albums, %v; want 13 albums, nil", len(s.Albums), err)
	}
	want := photoLibWant()
	for _, u := range []string{"fake-uuid-1", "fake-uuid-2", "fake-uuid-3", "fake-uuid-4", "fake-uuid-5", "fake-uuid-6", "fake-uuid-7", "fake-uuid-8"} {
		want.Albums = append(want.Albums, domain.Album{UUID: u, Title: "Day", InTrash: true})
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("Snapshot() = %+v, want %+v", s, want)
	}
	cnt := lib.Counters()
	if cnt.Dialogs != 8 {
		t.Errorf("Counters().Dialogs = %d, want 8", cnt.Dialogs)
	}
	if got, want := cnt.Calls, map[string]int{"CreateAlbum": 8, "DeleteCollections": 8, "UpdateAsset": 8, "DeleteAssets": 8, "Snapshot": 1}; !maps.Equal(got, want) {
		t.Errorf("Counters().Calls = %v, want %v", got, want)
	}
}
