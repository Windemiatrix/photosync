package domain

// Plan is what a run does. Ops are its operations in the order of
// Op.Compare, no two of them equal by it. Albums are the albums and folders
// that the run adds to iCloud, each by an OpAlbumCreate, and Pairs the album
// pairs that it makes. Notices are in the order of Code, then Params.
type Plan struct {
	Ops     []Op
	Albums  []Album
	Pairs   []AlbumPair
	Notices []Notice
}
