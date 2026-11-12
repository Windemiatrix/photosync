package testkit

import (
	"cmp"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Import creates the asset under a UUID such as "fake-uuid-1" and reads the
// files of req now, as the library copies them. The asset gets a
// CloudAssetGUID and a cloud record (InCloud) only from SyncCloud. The fake
// reads no metadata: Taken is req.Taken in UTC, zero when req.Taken is, and
// a resource other than the original keeps the name of its file. Both or
// neither of Photo and Video, LiveVideo or RAWPair without Photo, an
// OriginalName that holds a NUL or is not UTF-8, and a file that cannot be
// read fail the Import with nothing created, as they fail the library; so
// does a UUID in AlbumUUIDs that is not an album outside the trash, with
// domain.ErrNotFound.
func (f *FakePhotoLibrary) Import(ctx context.Context, req domain.ImportRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "Import"); err != nil {
		return "", err
	}
	if (req.Photo == "") == (req.Video == "") || (req.Photo == "" && (req.LiveVideo != "" || req.RAWPair != "")) ||
		strings.ContainsRune(req.OriginalName, 0) || !utf8.ValidString(req.OriginalName) {
		return "", errors.New("testkit: the import request breaks its contract")
	}
	for _, u := range req.AlbumUUIDs {
		if _, err := f.findCollection(u, ""); err != nil {
			return "", err
		}
	}
	original := cmp.Or(req.Photo, req.Video)
	a := domain.Asset{
		Library:      domain.LibraryPersonal,
		OriginalName: cmp.Or(req.OriginalName, filepath.Base(original)),
		Taken:        req.Taken.UTC(),
		DateSource:   domain.DateICloud,
		IsVideo:      req.Video != "",
		AlbumUUIDs:   slices.Clone(req.AlbumUUIDs),
	}
	var data [][]byte
	for _, file := range []struct {
		role       domain.ResourceRole
		path, name string
	}{
		{domain.ResourceOriginal, original, a.OriginalName},
		{domain.ResourceLiveVideo, req.LiveVideo, filepath.Base(req.LiveVideo)},
		{domain.ResourceRAWPair, req.RAWPair, filepath.Base(req.RAWPair)},
	} {
		if file.path == "" {
			continue
		}
		b, err := os.ReadFile(file.path) //nolint:gosec // G304: the caller names the file, as it names it to the library
		if err != nil {
			return "", err
		}
		a.Resources = append(a.Resources, domain.Resource{Role: file.role, Filename: file.name, Size: int64(len(b))})
		data = append(data, b)
	}
	a.Key.UUID = f.newID("fake-uuid")
	f.imported[len(f.assets)] = data
	f.assets = append(f.assets, a)
	return a.Key.UUID, nil
}

// SyncCloud is iCloud syncing the library: every asset gets a cloud record
// (InCloud true), and every asset, album and folder without a cloud
// identifier gets one such as "fake-guid-1", an album or a folder with
// KeyCloudGUID. The assets are numbered first, then the albums, in the order
// of a snapshot. The original of an asset stays where it was.
func (f *FakePhotoLibrary) SyncCloud() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.assets {
		a := &f.assets[i]
		// The seed and earlier snapshots share the old pointer.
		a.InCloud = new(true)
		if a.Key.CloudAssetGUID == "" {
			a.Key.CloudAssetGUID = f.newID("fake-guid")
		}
	}
	for i := range f.albums {
		if a := &f.albums[i]; a.Key == "" {
			a.Key, a.KeySource = f.newID("fake-guid"), domain.KeyCloudGUID
		}
	}
}
