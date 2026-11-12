package testkit

import (
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
	f.albums[i].Title = norm.NFC.String(title)
	return nil
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
	a := domain.Album{UUID: f.newID("fake-uuid"), Kind: kind, Title: norm.NFC.String(title), ParentUUID: parentUUID}
	f.albums = append(f.albums, a)
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
