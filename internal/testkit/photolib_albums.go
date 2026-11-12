package testkit

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// CreateFolder makes the folder under a UUID such as "fake-uuid-1" and keeps
// its title in NFC, as snapshots give titles; the folder gets a cloud
// identifier (Key) only from SyncCloud. A title that is empty, holds a NUL or
// is not UTF-8, and a parentUUID that is not a folder outside the trash fail
// the call with nothing created, the parentUUID with domain.ErrNotFound.
func (f *FakePhotoLibrary) CreateFolder(ctx context.Context, title, parentUUID string) (string, error) {
	return f.create(ctx, "CreateFolder", title, parentUUID, domain.KindFolder)
}

// CreateAlbum is CreateFolder for an album.
func (f *FakePhotoLibrary) CreateAlbum(ctx context.Context, title, parentUUID string) (string, error) {
	return f.create(ctx, "CreateAlbum", title, parentUUID, "")
}

// RenameCollection keeps the title in NFC and checks it as CreateFolder
// does; a uuid that is not a folder or an album outside the trash is
// domain.ErrNotFound.
func (f *FakePhotoLibrary) RenameCollection(ctx context.Context, uuid, title string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "RenameCollection"); err != nil {
		return err
	}
	if err := checkTitle(title); err != nil {
		return err
	}
	i, err := f.findCollection(uuid, domain.KindFolder, "")
	if err != nil {
		return err
	}
	return f.commit(ctx, "RenameCollection", func() { f.albums[i].Title = norm.NFC.String(title) })
}

func (f *FakePhotoLibrary) create(ctx context.Context, op, title, parentUUID string, kind domain.AlbumKind) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, op); err != nil {
		return "", err
	}
	if err := checkTitle(title); err != nil {
		return "", err
	}
	if parentUUID != "" {
		if _, err := f.findCollection(parentUUID, domain.KindFolder); err != nil {
			return "", err
		}
	}
	a := domain.Album{Kind: kind, Title: norm.NFC.String(title), ParentUUID: parentUUID}
	if err := f.commit(ctx, op, func() {
		a.UUID = f.newID("fake-uuid")
		f.albums = append(f.albums, a)
	}); err != nil {
		return "", err
	}
	return a.UUID, nil
}

// kinds holds "" to find an album, as snapshots leave the Kind of an album
// empty. A trashed folder or album is not found: live runs have not shown
// whether PhotoKit finds them, and app must not come to rely on it.
func (f *FakePhotoLibrary) findCollection(uuid string, kinds ...domain.AlbumKind) (int, error) {
	for i, a := range f.albums {
		if a.UUID == uuid && !a.InTrash && slices.Contains(kinds, a.Kind) {
			return i, nil
		}
	}
	return 0, domain.ErrNotFound
}

func checkTitle(title string) error {
	if title == "" || strings.ContainsRune(title, 0) || !utf8.ValidString(title) {
		return errors.New("testkit: the title is empty, holds a NUL or is not UTF-8")
	}
	return nil
}

// AddToAlbum puts the album last in the AlbumUUIDs of each asset that is not
// in it yet. An albumUUID that is not an album outside the trash, even with
// no assetUUIDs, and an asset that is hidden, trashed or unknown fail the
// call with domain.ErrNotFound and nothing changed.
func (f *FakePhotoLibrary) AddToAlbum(ctx context.Context, albumUUID string, assetUUIDs []string) error {
	return f.changeMembers(ctx, "AddToAlbum", albumUUID, assetUUIDs, func(albums []string) []string {
		if slices.Contains(albums, albumUUID) {
			return albums
		}
		return append(albums, albumUUID)
	})
}

// RemoveFromAlbum is AddToAlbum that takes the album out. An asset that
// leaves its last album gets nil AlbumUUIDs, not an empty slice, as one
// never in an album has, so that reflect.DeepEqual finds snapshots equal.
func (f *FakePhotoLibrary) RemoveFromAlbum(ctx context.Context, albumUUID string, assetUUIDs []string) error {
	return f.changeMembers(ctx, "RemoveFromAlbum", albumUUID, assetUUIDs, func(albums []string) []string {
		albums = slices.DeleteFunc(albums, func(u string) bool { return u == albumUUID })
		if len(albums) == 0 {
			return nil
		}
		return albums
	})
}

func (f *FakePhotoLibrary) changeMembers(ctx context.Context, op, albumUUID string, assetUUIDs []string, change func([]string) []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, op); err != nil {
		return err
	}
	if _, err := f.findCollection(albumUUID, ""); err != nil {
		return err
	}
	found, err := f.findAssets(assetUUIDs)
	if err != nil {
		return err
	}
	return f.commit(ctx, op, func() {
		for _, i := range found {
			f.assets[i].AlbumUUIDs = change(f.assets[i].AlbumUUIDs)
		}
	})
}

func (f *FakePhotoLibrary) findAssets(uuids []string) ([]int, error) {
	var found []int
	for _, u := range uuids {
		i, err := f.findAsset(u)
		if err != nil {
			return nil, err
		}
		found = append(found, i)
	}
	return found, nil
}

// Rebuild is the library made anew from iCloud, as on a new Mac. It gets a
// new LibraryIdentity, and its albums and folders, then its assets, each in
// the order of a snapshot, get new UUIDs such as "fake-uuid-1". A ParentUUID
// or an entry of AlbumUUIDs takes the new UUID, or stays as it is when it
// names nothing in the library. With keepGUIDs false, each nonempty
// CloudAssetGUID and Key changes too, to one such as "fake-guid-2" numbered
// right after the new UUID of its owner: live runs have not shown which of
// the two Photos does. The rest stays, the files that Import read included.
func (f *FakePhotoLibrary) Rebuild(keepGUIDs bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.id = photoLibIDs.Add(1)
	uuids := map[string]string{}
	for i := range f.albums {
		a := &f.albums[i]
		uuids[a.UUID] = f.newID("fake-uuid")
		if !keepGUIDs && a.Key != "" {
			a.Key = f.newID("fake-guid")
		}
	}
	// A folder may come after the albums in it, so no ParentUUID changes
	// before every new UUID is known.
	for i := range f.albums {
		a := &f.albums[i]
		a.UUID, a.ParentUUID = uuids[a.UUID], cmp.Or(uuids[a.ParentUUID], a.ParentUUID)
	}
	for i := range f.assets {
		a := &f.assets[i]
		a.Key.UUID = f.newID("fake-uuid")
		if !keepGUIDs && a.Key.CloudAssetGUID != "" {
			a.Key.CloudAssetGUID = f.newID("fake-guid")
		}
		for j, u := range a.AlbumUUIDs {
			a.AlbumUUIDs[j] = cmp.Or(uuids[u], u)
		}
	}
}
