package plan

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Syncs reports whether asset a takes part in a sync under p. It does not
// when it is hidden and p.Hidden is skip, when it is a burst shot that the
// user did not pick and p.Burst is selected_only, or when its original is
// neither on this Mac nor in iCloud. An asset that does not sync is neither
// new nor changed: its links stay as they are, whatever happens on either
// side. An asset in Recently Deleted syncs, so that its links see the
// deletion.
func Syncs(a domain.Asset, p domain.Policies) bool {
	skipped := a.Hidden && p.Hidden == domain.HiddenSkip ||
		a.BurstUnselected && p.Burst == domain.BurstSelectedOnly ||
		a.Missing && (a.InCloud == nil || !*a.InCloud)
	return !skipped
}

// IsNew reports whether asset a, which has no link in a remote, is new
// there: it syncs and is not in Recently Deleted.
func IsNew(a domain.Asset, p domain.Policies) bool {
	return !a.InTrash && Syncs(a, p)
}

// ICloudSide returns how the asset of active link l changed in iCloud since
// the last applied run, which left it in baseLibrary. now is the asset of
// the snapshot reconciled with the state, found by the AssetID of l, with
// the albums it is in now as Memberships; found reports that the snapshot
// holds it, and albums are the albums of the library with the IDs of the
// state. The asset is deleted when it is gone or in Recently Deleted;
// otherwise it moved when it left baseLibrary for a library other than the
// root of l, or when l lies in the directory of an album that it is no
// longer in: an album missing from now.Memberships or from albums, or in
// Recently Deleted. A library that differs only from the root of l is no
// move: the asset waits to be moved there in Photos. The policies do not
// matter here: see Syncs.
func ICloudSide(l domain.Link, baseLibrary domain.LibraryKind, now domain.Asset, found bool, albums []domain.Album) domain.ChangeKind {
	switch {
	case !found || now.InTrash:
		return domain.ChangeDeleted
	case now.Library != baseLibrary && now.Library != l.Root,
		l.AlbumID != 0 && !stillInAlbum(now, l.AlbumID, albums):
		return domain.ChangeMoved
	}
	return domain.ChangeUnchanged
}

func stillInAlbum(a domain.Asset, id domain.AlbumID, albums []domain.Album) bool {
	return slices.ContainsFunc(a.Memberships, func(m domain.Membership) bool { return m.AlbumID == id }) &&
		slices.ContainsFunc(albums, func(b domain.Album) bool { return b.ID == id && !b.InTrash })
}

// RemoteSide returns how the file of active link l changed in the cloud
// since the last applied run. e is the entry of the scan at the root and
// path of l, and found reports that the scan has one. The file is deleted
// when there is none or e is a directory, unchanged when e has the content
// of the base, else replaced. The base keeps the size, hashes and ETag that
// the listing of the remote gave. With hash md5 or sha256 the content
// compares by that hash when l and e both have it; otherwise, and always
// with HashNone, by size and by an ETag that both have. A file that turns up
// at another path is deleted here; the caller makes it a move (R-OBJECT-08).
func RemoteSide(l domain.Link, e domain.RemoteEntry, found bool, hash domain.HashKind) domain.ChangeKind {
	switch {
	case !found || e.IsDir:
		return domain.ChangeDeleted
	case sameContent(l, e, hash):
		return domain.ChangeUnchanged
	}
	return domain.ChangeReplaced
}

// A value that one side lacks proves no change: a remote that begins to
// report a hash or an ETag must not replace every asset it holds.
func sameContent(l domain.Link, e domain.RemoteEntry, hash domain.HashKind) bool {
	switch {
	case hash == domain.HashMD5 && l.MD5 != "" && e.MD5 != "":
		return l.MD5 == e.MD5
	case hash == domain.HashSHA256 && l.SHA256 != "" && e.SHA256 != "":
		return l.SHA256 == e.SHA256
	}
	return l.Size == e.Size && (l.ETag == "" || e.ETag == "" || l.ETag == e.ETag)
}
