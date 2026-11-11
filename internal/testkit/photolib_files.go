package testkit

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Download writes each resource of the asset into dir as a file named after
// its role, such as "original": Size bytes that begin with the Filename of
// the resource and the Taken of the asset, so that duplicates in iCloud,
// alike in all three, give equal files. ctx is checked before each file;
// progress is called after each one with the fake locked and must not call
// the fake.
func (f *FakePhotoLibrary) Download(ctx context.Context, uuid, dir string, progress func(received int64)) ([]domain.LocalFile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "Download"); err != nil {
		return nil, err
	}
	i, err := f.findAsset(uuid)
	if err != nil {
		return nil, err
	}
	var files []domain.LocalFile
	var received int64
	for _, res := range f.assets[i].Resources {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		b := make([]byte, res.Size)
		copy(b, res.Filename+" "+f.assets[i].Taken.UTC().Format(time.RFC3339Nano))
		path := filepath.Join(dir, string(res.Role))
		if err := os.WriteFile(path, b, 0o600); err != nil {
			return nil, err
		}
		received += int64(len(b))
		f.counters.BytesDownloaded += int64(len(b))
		if progress != nil {
			progress(received)
		}
		files = append(files, domain.LocalFile{Role: res.Role, Path: path})
	}
	f.assets[i].Missing = false
	return files, nil
}

// Restore is the user taking the asset back from Recently Deleted; an
// unknown uuid changes nothing.
func (f *FakePhotoLibrary) Restore(uuid string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.assets {
		if f.assets[i].Key.UUID == uuid {
			f.assets[i].InTrash = false
		}
	}
}

// A trashed asset is not found either: live runs have not shown whether
// PhotoKit finds the assets in Recently Deleted, and app must not come to
// rely on it.
func (f *FakePhotoLibrary) findAsset(uuid string) (int, error) {
	for i, a := range f.assets {
		if a.Key.UUID == uuid && !a.Hidden && !a.InTrash {
			return i, nil
		}
	}
	return 0, domain.ErrNotFound
}
