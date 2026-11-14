package domain

// Plan is what a run does. Ops are its operations in the order of
// Op.Compare, no two of them equal by it. Albums are the albums and folders
// that the run adds to iCloud, each by an OpAlbumCreate, and Pairs the album
// pairs that it makes. Conflicts are those of the assets in each remote where
// the run decides on the asset, open, skipped or resolved as the run leaves
// them, in the order of AssetID, then Remote: a resolved one leaves the
// state, and one of the state that Conflicts lack stays there as it is.
// Notices are in the order of Code, then Params.
type Plan struct {
	Ops       []Op
	Albums    []Album
	Pairs     []AlbumPair
	Conflicts []Conflict
	Notices   []Notice
}
