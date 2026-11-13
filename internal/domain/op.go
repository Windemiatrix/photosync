package domain

import "cmp"

// OpKind is what an operation of a plan does.
type OpKind string

// The operations. OpDelete removes a file from the cloud, OpCopy and OpMove
// copy and move one there, OpQuarantine moves one to the quarantine of its
// root, and OpPut uploads the file of an asset with the role of its link.
// OpAlbumAdd and OpAlbumRemove put an asset into an album of iCloud and take
// it out of one, and OpAssetDelete moves an asset to Recently Deleted.
const (
	OpDelete      OpKind = "delete"
	OpCopy        OpKind = "copy"
	OpMove        OpKind = "move"
	OpQuarantine  OpKind = "quarantine"
	OpPut         OpKind = "put"
	OpAlbumAdd    OpKind = "album_add"
	OpAlbumRemove OpKind = "album_remove"
	OpAssetDelete OpKind = "asset_delete"
)

// Phase is the phase of a run that runs the operations of kind k. A run
// takes its phases in increasing order and starts one only once the one
// before it is done: the files of the cloud change in phase 1 and new ones
// go up in phase 2, the albums of assets change in phase 4 and the assets
// that iCloud loses go in phase 5.
func (k OpKind) Phase() int {
	switch k {
	case OpPut:
		return 2
	case OpAlbumAdd, OpAlbumRemove:
		return 4
	case OpAssetDelete:
		return 5
	}
	return 1
}

// Op is one operation of a plan. Remote names the remote of an operation on
// the cloud and is empty for one on iCloud. An operation on the cloud reads,
// moves or removes the file at Src under its Root and writes the one at Dst
// under DstRoot; a put has no Root and Src and a delete no DstRoot and Dst,
// and those fields stay empty. Asset is the asset whose file, album or self
// the operation changes, and AlbumID the album that the asset joins or
// leaves. Rule is the ID of the rule in Rules that chose the operation, and
// Irreversible reports that undo cannot bring back what the operation
// removes. Links are the links that the operation changes, in the order of
// ID, each as the state keeps it once the operation is done; a link with an
// empty Status leaves the state, and one with a zero ID is new and takes its
// ID from the state.
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
