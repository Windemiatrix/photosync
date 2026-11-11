package testkit

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Two fakes never share an identity, as two libraries do not.
var photoLibIDs atomic.Int64

// FakePhotoLibrary is the system photo library of a test, held in memory;
// NewFakePhotoLibrary makes one. A call whose ctx has already ended fails
// with the error of ctx and does nothing else, as a call of the real library
// does. A trashed asset, like a hidden one, stays in snapshots, but the
// other methods do not find it. Its methods may be called from several
// goroutines.
type FakePhotoLibrary struct {
	mu       sync.Mutex
	id       int64
	assets   []domain.Asset
	albums   []domain.Album
	failNext map[string]error
	counters Counters
}

// Counters is what a FakePhotoLibrary has done: Calls counts the calls of
// each method of app.PhotoLibrary by its name, failed calls included, and
// BytesDownloaded the bytes of the files that Download wrote.
type Counters struct {
	Calls           map[string]int
	BytesDownloaded int64
}

// NewFakePhotoLibrary returns the library of s, which keeps the contract of
// a snapshot: the fake does not check it, and later changes of s do not
// reach it. An original that iCloud holds (InCloud true) starts only in
// iCloud, whatever Missing says, so that no test relies on it being on this
// Mac.
func NewFakePhotoLibrary(s domain.LibrarySnapshot) *FakePhotoLibrary {
	f := &FakePhotoLibrary{
		id:       photoLibIDs.Add(1),
		assets:   cloneAssets(s.Assets),
		albums:   slices.Clone(s.Albums),
		failNext: map[string]error{},
		counters: Counters{Calls: map[string]int{}},
	}
	for i := range f.assets {
		if a := &f.assets[i]; a.InCloud != nil && *a.InCloud {
			a.Missing = true
		}
	}
	return f
}

// Counters returns a copy of the counters, which later calls leave as it is.
func (f *FakePhotoLibrary) Counters() Counters {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := f.counters
	c.Calls = maps.Clone(c.Calls)
	return c
}

// FailNext makes the next call of the method of app.PhotoLibrary named op,
// such as "Snapshot", fail with err; the call is counted and does nothing
// else. A nil err takes back an earlier FailNext of op.
func (f *FakePhotoLibrary) FailNext(op string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failNext[op] = err
}

// Snapshot reads the library whatever path it is given: the fake is one
// library. The snapshot shares no slice with the library, and the
// FolderTitles of an album are the titles of the folders above it now,
// whatever NewFakePhotoLibrary got.
func (f *FakePhotoLibrary) Snapshot(ctx context.Context, _ string) (domain.LibrarySnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "Snapshot"); err != nil {
		return domain.LibrarySnapshot{}, err
	}
	s := domain.LibrarySnapshot{Assets: cloneAssets(f.assets), Albums: slices.Clone(f.albums)}
	for i := range s.Albums {
		s.Albums[i].FolderTitles = f.folderTitles(s.Albums[i].ParentUUID)
	}
	return s, nil
}

// SystemLibraryPath gives the same made-up path for every fake.
func (f *FakePhotoLibrary) SystemLibraryPath() (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(context.Background(), "SystemLibraryPath"); err != nil {
		return "", err
	}
	return "/fake/Photos Library.photoslibrary", nil
}

// LibraryIdentity gives the identity of the fake whatever path it is given.
// Each fake has an identity of its own, which tests compare but do not read.
func (f *FakePhotoLibrary) LibraryIdentity(ctx context.Context, _ string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, "LibraryIdentity"); err != nil {
		return "", err
	}
	return fmt.Sprintf("fake-library-%d", f.id), nil
}

func (f *FakePhotoLibrary) begin(ctx context.Context, op string) error {
	f.counters.Calls[op]++
	err := f.failNext[op]
	delete(f.failNext, op)
	if err != nil {
		return err
	}
	return ctx.Err()
}

func (f *FakePhotoLibrary) folderTitles(parent string) []string {
	var titles []string
	for parent != "" {
		i := slices.IndexFunc(f.albums, func(a domain.Album) bool { return a.UUID == parent })
		if i < 0 {
			break
		}
		titles = slices.Insert(titles, 0, f.albums[i].Title)
		parent = f.albums[i].ParentUUID
	}
	return titles
}

// A test and app may sort or fill these slices in place, as photosync fills
// the hashes of Resources, and must not change the library by that.
func cloneAssets(assets []domain.Asset) []domain.Asset {
	c := slices.Clone(assets)
	for i := range c {
		c[i].Resources = slices.Clone(c[i].Resources)
		c[i].AlbumUUIDs = slices.Clone(c[i].AlbumUUIDs)
	}
	return c
}
