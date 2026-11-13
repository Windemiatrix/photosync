package domain

import "cmp"

// OpKind is what an operation of a plan does.
type OpKind string

// The operations. OpDelete removes a file from the cloud, OpCopy and OpMove
// copy and move one there, OpQuarantine moves one to the quarantine of its
// root, and OpPut uploads the file of an asset with the role of its link.
// OpAlbumCreate adds to iCloud an album or a folder of albums that the run
// adds, and OpImport makes a new asset of iCloud from a file of the cloud.
// OpAlbumAdd and OpAlbumRemove put an asset into an album of iCloud and take
// it out of one, and OpAssetDelete moves an asset to Recently Deleted.
const (
	OpDelete      OpKind = "delete"
	OpCopy        OpKind = "copy"
	OpMove        OpKind = "move"
	OpQuarantine  OpKind = "quarantine"
	OpPut         OpKind = "put"
	OpAlbumCreate OpKind = "album_create"
	OpImport      OpKind = "import"
	OpAlbumAdd    OpKind = "album_add"
	OpAlbumRemove OpKind = "album_remove"
	OpAssetDelete OpKind = "asset_delete"
)

// Phase is the phase of a run that runs the operations of kind k. A run
// takes its phases in increasing order and starts one only once the one
// before it is done: the files of the cloud change and the albums that the
// run adds come to iCloud in phase 1, new files go up in phase 2, new assets
// come in from the cloud in phase 3, into albums that are there by then, the
// albums of assets change in phase 4 and the assets that iCloud loses go in
// phase 5.
func (k OpKind) Phase() int {
	switch k {
	case OpPut:
		return 2
	case OpImport:
		return 3
	case OpAlbumAdd, OpAlbumRemove:
		return 4
	case OpAssetDelete:
		return 5
	}
	return 1
}

// Op is one operation of a plan. Remote names the remote of an operation on
// the cloud, and of an import the remote whose file it reads; it is empty for
// any other operation on iCloud. An operation on the cloud reads, moves or
// removes the file at Src under its Root and writes the one at Dst under
// DstRoot, and an import reads the file at Src under Root; a put has no Root
// and Src, a delete and an import no DstRoot and Dst, and an album_create
// none of the four, and those fields stay empty. Asset is the asset whose
// file, album or self the operation changes, zero for none, as for an import,
// whose asset is new; AlbumID is the album that the asset joins or leaves, or
// that an album_create adds. Rule is the ID of the rule in Rules that chose
// the operation, and Irreversible reports that undo cannot bring back what
// the operation removes. Links are the links that the operation changes, each
// as the state keeps it once the operation is done, in the order of ID, and
// those of an import with the link of its main file first; a link with an
// empty Status leaves the state, one with a zero ID is new and takes its ID
// from the state, and one with a zero AssetID takes that of the asset that
// its import makes.
type Op struct {
	Kind         OpKind
	Remote       string
	Root         LibraryKind
	Src          string
	DstRoot      LibraryKind
	Dst          string
	Asset        AssetID
	AlbumID      AlbumID
	Rule         string
	Irreversible bool
	Links        []Link
}

// Compare orders the operations of a plan as a run takes them: by the Phase
// of Kind, then by Remote, those on iCloud first, then by Kind, Root, Src,
// Dst, Asset and AlbumID, strings byte by byte. The result is negative, zero
// or positive, as with cmp.Compare.
func (o Op) Compare(x Op) int {
	return cmp.Or(
		cmp.Compare(o.Kind.Phase(), x.Kind.Phase()), cmp.Compare(o.Remote, x.Remote), cmp.Compare(o.Kind, x.Kind),
		cmp.Compare(o.Root, x.Root), cmp.Compare(o.Src, x.Src), cmp.Compare(o.Dst, x.Dst),
		cmp.Compare(o.Asset, x.Asset), cmp.Compare(o.AlbumID, x.AlbumID),
	)
}
