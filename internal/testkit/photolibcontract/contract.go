package photolibcontract

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/app"
	"github.com/Windemiatrix/photosync/internal/domain"
)

// Library is a photo library under test with hooks, all set, that act for its
// user and for iCloud. Photo is the absolute path of a photo file that it can
// import. Hold makes the next change by the method of app.PhotoLibrary named
// op wait for an outcome that comes too late: waiting is called while the
// change waits, and only Answer gives the outcome; with no change waiting,
// Answer answers the next dialog of op. Answer, SyncCloud and Restore, which
// takes the asset back from Recently Deleted, return once snapshots show what
// they did. A hook that cannot act skips the test.
type Library struct {
	app.PhotoLibrary
	Photo     string
	Answer    func(op string, yes bool)
	Hold      func(op string, waiting func())
	SyncCloud func()
	Restore   func(uuid string)
}

// Run runs the cases one by one, for the user of a real library answers its
// dialogs one at a time, each on the Library that open gives for the test of
// the case. A case changes only the assets and albums that it made.
func Run(t *testing.T, open func(t *testing.T) Library) {
	t.Run("import", func(t *testing.T) { testImport(t, open(t)) })
	t.Run("cloud", func(t *testing.T) { testCloud(t, open(t)) })
	t.Run("delete", func(t *testing.T) { testDelete(t, open(t)) })
	t.Run("hide", func(t *testing.T) { testHide(t, open(t)) })
}

func testImport(t *testing.T, l Library) {
	ctx := t.Context()
	info, err := os.Stat(l.Photo)
	if err != nil {
		t.Fatalf("os.Stat(Photo) error = %v", err)
	}
	album, err := l.CreateAlbum(ctx, "05/01 Holidays", "")
	if err != nil {
		t.Fatalf("CreateAlbum() error = %v", err)
	}
	name := "IMG_0101" + filepath.Ext(l.Photo)
	uuid, err := l.Import(ctx, domain.ImportRequest{Photo: l.Photo, OriginalName: name, AlbumUUIDs: []string{album}})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	res := []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: info.Size()}}
	if a := assetOf(t, l, uuid); a.OriginalName != name || a.Library != domain.LibraryPersonal ||
		!slices.Equal(a.Resources, res) || !slices.Equal(a.AlbumUUIDs, []string{album}) {
		t.Errorf("the imported asset has OriginalName %q, Library %q, Resources %+v and %d albums; want %q, %q, %+v and the new album",
			a.OriginalName, a.Library, a.Resources, len(a.AlbumUUIDs), name, domain.LibraryPersonal, res)
	}
	if err := l.AddToAlbum(ctx, album, []string{uuid}); err != nil {
		t.Errorf("AddToAlbum() of a member error = %v, want nil", err)
	}
	if !slices.Equal(assetOf(t, l, uuid).AlbumUUIDs, []string{album}) {
		t.Error("AddToAlbum() of a member changed its albums")
	}
}

func testCloud(t *testing.T, l Library) {
	uuid := importPhoto(t, l)
	l.SyncCloud()
	if assetOf(t, l, uuid).Key.CloudAssetGUID == "" {
		t.Error("the asset has no CloudAssetGUID after SyncCloud")
	}
}

func testDelete(t *testing.T, l Library) {
	uuid := importPhoto(t, l)
	l.Answer("DeleteAssets", false)
	if err := l.DeleteAssets(t.Context(), []string{uuid}); !errors.Is(err, domain.ErrUserCanceled) {
		t.Errorf("DeleteAssets() refused by the user error = %v, want %v", err, domain.ErrUserCanceled)
	}
	if assetOf(t, l, uuid).InTrash {
		t.Error("DeleteAssets() changed the library although the user refused")
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	l.Hold("DeleteAssets", cancel)
	if err := l.DeleteAssets(ctx, []string{uuid}); !errors.Is(err, domain.ErrOutcomeUnknown) || !errors.Is(err, context.Canceled) {
		t.Errorf("DeleteAssets() ended before its outcome error = %v, want %v with %v", err, domain.ErrOutcomeUnknown, context.Canceled)
	}
	if assetOf(t, l, uuid).InTrash {
		t.Error("DeleteAssets() took effect before its outcome")
	}
	l.Answer("DeleteAssets", true)
	if !assetOf(t, l, uuid).InTrash {
		t.Fatal("a late yes did not move the asset to Recently Deleted")
	}
	l.Restore(uuid)
	if assetOf(t, l, uuid).InTrash {
		t.Error("the asset stays in Recently Deleted after Restore")
	}
}

func testHide(t *testing.T, l Library) {
	ctx := t.Context()
	uuid := importPhoto(t, l)
	l.Answer("UpdateAsset", true)
	if err := l.UpdateAsset(ctx, uuid, domain.AssetPatch{Hidden: new(true)}); err != nil {
		t.Fatalf("UpdateAsset() hiding error = %v, want nil", err)
	}
	if !assetOf(t, l, uuid).Hidden {
		t.Error("the snapshot does not show the asset as Hidden")
	}
	if _, err := l.Download(ctx, uuid, t.TempDir(), nil); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("Download() of a hidden asset error = %v, want %v", err, domain.ErrNotFound)
	}
	if err := l.UpdateAsset(ctx, uuid, domain.AssetPatch{Hidden: new(false)}); !errors.Is(err, domain.ErrNotFound) {
		t.Errorf("UpdateAsset() showing a hidden asset error = %v, want %v", err, domain.ErrNotFound)
	}
}

func importPhoto(t *testing.T, l Library) string {
	t.Helper()
	uuid, err := l.Import(t.Context(), domain.ImportRequest{Photo: l.Photo})
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}
	return uuid
}

func assetOf(t *testing.T, l Library, uuid string) domain.Asset {
	t.Helper()
	path, err := l.SystemLibraryPath()
	if err != nil {
		t.Fatalf("SystemLibraryPath() error = %v", err)
	}
	s, err := l.Snapshot(t.Context(), path)
	if err != nil {
		t.Fatalf("Snapshot() error = %v", err)
	}
	i := slices.IndexFunc(s.Assets, func(a domain.Asset) bool { return a.Key.UUID == uuid })
	if i < 0 {
		t.Fatal("the snapshot lacks the asset that the case made")
	}
	return s.Assets[i]
}
