package testkit

import (
	"context"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DeleteAssets asks the user once for the whole batch. An asset that is
// hidden, trashed or unknown fails the call with domain.ErrNotFound, nothing
// changed and nothing asked.
func (f *FakePhotoLibrary) DeleteAssets(ctx context.Context, uuids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "DeleteAssets"); err != nil {
		return err
	}
	found, err := f.findAssets(uuids)
	if err != nil || len(found) == 0 {
		return err
	}
	if err := f.ask("DeleteAssets"); err != nil {
		return err
	}
	return f.commit(ctx, "DeleteAssets", func() {
		for _, i := range found {
			f.assets[i].InTrash = true
		}
	})
}

// UpdateAsset asks the user only to hide the asset: a p that hides nothing
// succeeds with nothing changed and nothing asked. An asset that is hidden,
// trashed or unknown fails the call with domain.ErrNotFound whatever p is.
func (f *FakePhotoLibrary) UpdateAsset(ctx context.Context, uuid string, p domain.AssetPatch) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "UpdateAsset"); err != nil {
		return err
	}
	i, err := f.findAsset(uuid)
	if err != nil || p.Hidden == nil || !*p.Hidden {
		return err
	}
	if err := f.ask("UpdateAsset"); err != nil {
		return err
	}
	return f.commit(ctx, "UpdateAsset", func() { f.assets[i].Hidden = true })
}

// DeleteCollections asks the user once, for the library may ask, and then
// puts each folder and album of uuids and every one under them in the trash,
// where snapshots keep them. An empty uuids asks nothing, and a uuid that is
// not a folder or an album outside the trash fails the call with
// domain.ErrNotFound, nothing changed and nothing asked.
func (f *FakePhotoLibrary) DeleteCollections(ctx context.Context, uuids []string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "DeleteCollections"); err != nil {
		return err
	}
	for _, u := range uuids {
		if _, err := f.findCollection(u, domain.KindFolder, ""); err != nil {
			return err
		}
	}
	var found []int
	for i, a := range f.albums {
		if f.within(a.UUID, uuids) {
			found = append(found, i)
		}
	}
	if len(found) == 0 {
		return nil
	}
	if err := f.ask("DeleteCollections"); err != nil {
		return err
	}
	return f.commit(ctx, "DeleteCollections", func() {
		for _, i := range found {
			f.albums[i].InTrash = true
		}
	})
}

func (f *FakePhotoLibrary) ask(op string) error {
	f.counters.Dialogs++
	if f.refused[op] {
		delete(f.refused, op)
		return domain.ErrUserCanceled
	}
	return nil
}

func (f *FakePhotoLibrary) within(uuid string, roots []string) bool {
	if slices.Contains(roots, uuid) {
		return true
	}
	i := slices.IndexFunc(f.albums, func(a domain.Album) bool { return a.UUID == uuid })
	return i >= 0 && f.within(f.albums[i].ParentUUID, roots)
}
