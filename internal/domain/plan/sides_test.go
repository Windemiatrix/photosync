package plan_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Recently Deleted keeps an asset in sync, so that its links see the
// deletion, but never makes it new.
func TestSideSyncs(t *testing.T) {
	t.Parallel()
	inCloud, notInCloud := true, false
	defaults, hiddenSkip, selectedOnly := domain.DefaultPolicies(), domain.DefaultPolicies(), domain.DefaultPolicies()
	hiddenSkip.Hidden, selectedOnly.Burst = domain.HiddenSkip, domain.BurstSelectedOnly
	for _, c := range []struct {
		name         string
		asset        domain.Asset
		p            domain.Policies
		syncs, isNew bool
	}{
		{"a plain asset", domain.Asset{}, defaults, true, true},
		{"hidden under hidden: sync", domain.Asset{Hidden: true}, defaults, true, true},
		{"hidden under hidden: skip", domain.Asset{Hidden: true}, hiddenSkip, false, false},
		{"hidden under burst: selected_only", domain.Asset{Hidden: true}, selectedOnly, true, true},
		{"an unpicked burst shot under burst: all", domain.Asset{BurstUnselected: true}, defaults, true, true},
		{"an unpicked burst shot under burst: selected_only", domain.Asset{BurstUnselected: true}, selectedOnly, false, false},
		{"an unpicked burst shot under hidden: skip", domain.Asset{BurstUnselected: true}, hiddenSkip, true, true},
		{"the original only in iCloud", domain.Asset{Missing: true, InCloud: &inCloud}, defaults, true, true},
		{"the original nowhere, no cloud record", domain.Asset{Missing: true}, defaults, false, false},
		{"the original nowhere, not in iCloud", domain.Asset{Missing: true, InCloud: &notInCloud}, defaults, false, false},
		{"the original on this Mac, not in iCloud yet", domain.Asset{InCloud: &notInCloud}, defaults, true, true},
		{"in Recently Deleted", domain.Asset{InTrash: true}, defaults, true, false},
		{"hidden in Recently Deleted under hidden: skip", domain.Asset{Hidden: true, InTrash: true}, hiddenSkip, false, false},
	} {
		if got := plan.Syncs(c.asset, c.p); got != c.syncs {
			t.Errorf("%s: Syncs = %v; want %v", c.name, got, c.syncs)
		}
		if got := plan.IsNew(c.asset, c.p); got != c.isNew {
			t.Errorf("%s: IsNew = %v; want %v", c.name, got, c.isNew)
		}
	}
}

// Each row starts from a link that nothing touched since the last applied
// run. The base library is personal unless a row says otherwise: an asset
// imported from the shared root stays in the personal one, waiting to be
// moved to the library of its file.
func TestSideICloud(t *testing.T) {
	t.Parallel()
	unchanged, moved, deleted := domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted
	const event, trip, party, gone domain.AlbumID = 10, 11, 12, 13
	albums := []domain.Album{
		{ID: event, UUID: "album-10", Title: "05/01 Holidays", FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", FolderTitles: []string{"2024"}, InTrash: true},
	}
	in := func(ids ...domain.AlbumID) []domain.Membership {
		var m []domain.Membership
		for _, id := range ids {
			m = append(m, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return m
	}
	for _, c := range []struct {
		name  string
		found bool
		edit  func(l *domain.Link, baseLibrary *domain.LibraryKind, now *domain.Asset)
		want  domain.ChangeKind
	}{
		{"no change", true, func(*domain.Link, *domain.LibraryKind, *domain.Asset) {}, unchanged},
		{"gone from the library", false, func(*domain.Link, *domain.LibraryKind, *domain.Asset) {}, deleted},
		{"in Recently Deleted", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) { now.InTrash = true }, deleted},
		{"in Recently Deleted after leaving its album and library", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			now.InTrash, now.Library, now.Memberships = true, domain.LibraryShared, in(trip)
		}, deleted},
		{"hidden, an unpicked burst shot, its original nowhere", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			now.Hidden, now.BurstUnselected, now.Missing = true, true, true
		}, unchanged},
		{"shared in Photos", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) { now.Library = domain.LibraryShared }, moved},
		{"unshared in Photos", true, func(l *domain.Link, baseLibrary *domain.LibraryKind, _ *domain.Asset) {
			l.Root, *baseLibrary = domain.LibraryShared, domain.LibraryShared
		}, moved},
		{"waits in the personal library for the shared one of its file", true, func(l *domain.Link, _ *domain.LibraryKind, _ *domain.Asset) {
			l.Root = domain.LibraryShared
		}, unchanged},
		{"reaches the shared library of its file", true, func(l *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			l.Root, now.Library = domain.LibraryShared, domain.LibraryShared
		}, unchanged},
		{"moved to another album", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) { now.Memberships = in(trip) }, moved},
		{"added to another album", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) { now.Memberships = in(event, trip) }, unchanged},
		{"removed from every album", true, func(_ *domain.Link, _ *domain.LibraryKind, now *domain.Asset) { now.Memberships = nil }, moved},
		{"the album of the link in Recently Deleted", true, func(l *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			l.AlbumID, now.Memberships = party, in(party)
		}, moved},
		{"the album of the link gone from the library", true, func(l *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			l.AlbumID, now.Memberships = gone, in(gone)
		}, moved},
		{"a link outside albums, the asset is in one", true, func(l *domain.Link, _ *domain.LibraryKind, _ *domain.Asset) { l.AlbumID = 0 }, unchanged},
		{"a link outside albums, the asset is in none", true, func(l *domain.Link, _ *domain.LibraryKind, now *domain.Asset) {
			l.AlbumID, now.Memberships = 0, nil
		}, unchanged},
	} {
		l := domain.Link{
			ID: 1, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal,
			Path: "2024/05-01 - Holidays/IMG_0001.HEIC", Role: domain.RoleOriginal, AlbumID: event, Status: domain.StatusActive,
		}
		baseLibrary := domain.LibraryPersonal
		now := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: domain.LibraryPersonal, OriginalName: "IMG_0001.HEIC", Memberships: in(event),
		}
		c.edit(&l, &baseLibrary, &now)
		if got := plan.ICloudSide(l, baseLibrary, now, c.found, albums); got != c.want {
			t.Errorf("%s: ICloudSide = %q; want %q", c.name, got, c.want)
		}
	}
}

// Every row runs under each kind of hash, so that one table shows which
// values decide under which kind; a value that one side lacks proves
// nothing.
func TestSideRemote(t *testing.T) {
	t.Parallel()
	type byHash = [3]domain.ChangeKind
	unchanged, replaced, deleted := domain.ChangeUnchanged, domain.ChangeReplaced, domain.ChangeDeleted
	for _, c := range []struct {
		name  string
		found bool
		edit  func(l *domain.Link, e *domain.RemoteEntry)
		want  byHash
	}{
		{"the same file", true, func(*domain.Link, *domain.RemoteEntry) {}, byHash{unchanged, unchanged, unchanged}},
		{"no file at the path", false, func(*domain.Link, *domain.RemoteEntry) {}, byHash{deleted, deleted, deleted}},
		{"a directory at the path", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.IsDir = true }, byHash{deleted, deleted, deleted}},
		{"new content", true, func(_ *domain.Link, e *domain.RemoteEntry) {
			e.Size, e.MD5, e.SHA256, e.ETag = 101, "ac", "bc", "e2"
		}, byHash{replaced, replaced, replaced}},
		{"new content of the same size", true, func(_ *domain.Link, e *domain.RemoteEntry) {
			e.MD5, e.SHA256, e.ETag = "ac", "bc", "e2"
		}, byHash{replaced, replaced, replaced}},
		{"a new ETag only", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.ETag = "e2" }, byHash{replaced, unchanged, unchanged}},
		{"another MD5 only", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.MD5 = "ac" }, byHash{unchanged, replaced, unchanged}},
		{"another SHA-256 only", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.SHA256 = "bc" }, byHash{unchanged, unchanged, replaced}},
		{"the entry without hashes", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.MD5, e.SHA256 = "", "" }, byHash{unchanged, unchanged, unchanged}},
		{"the base without hashes", true, func(l *domain.Link, _ *domain.RemoteEntry) { l.MD5, l.SHA256 = "", "" }, byHash{unchanged, unchanged, unchanged}},
		{"no hashes and another size", true, func(_ *domain.Link, e *domain.RemoteEntry) {
			e.MD5, e.SHA256, e.Size = "", "", 101
		}, byHash{replaced, replaced, replaced}},
		{"no hashes and another ETag", true, func(_ *domain.Link, e *domain.RemoteEntry) {
			e.MD5, e.SHA256, e.ETag = "", "", "e2"
		}, byHash{replaced, replaced, replaced}},
		{"the entry without an ETag", true, func(_ *domain.Link, e *domain.RemoteEntry) { e.ETag = "" }, byHash{unchanged, unchanged, unchanged}},
		{"the base without an ETag", true, func(l *domain.Link, _ *domain.RemoteEntry) { l.ETag = "" }, byHash{unchanged, unchanged, unchanged}},
		{"no ETag and new content", true, func(_ *domain.Link, e *domain.RemoteEntry) {
			e.ETag, e.Size, e.MD5, e.SHA256 = "", 101, "ac", "bc"
		}, byHash{replaced, replaced, replaced}},
	} {
		l := domain.Link{
			ID: 1, AssetID: 7, Remote: "disk", Root: domain.LibraryPersonal,
			Path: "2024/05-01 - Holidays/IMG_0001.HEIC", Role: domain.RoleOriginal, Status: domain.StatusActive,
			Size: 100, MD5: "aa", SHA256: "bb", ETag: "e1",
		}
		e := domain.RemoteEntry{Root: l.Root, Path: l.Path, Size: 100, MD5: "aa", SHA256: "bb", ETag: "e1"}
		c.edit(&l, &e)
		var got byHash
		for i, hash := range []domain.HashKind{domain.HashNone, domain.HashMD5, domain.HashSHA256} {
			got[i] = plan.RemoteSide(l, e, c.found, hash)
		}
		if got != c.want {
			t.Errorf("%s: RemoteSide with no hash, MD5, SHA-256 = %q; want %q", c.name, got, c.want)
		}
	}
}
