package domain

// State is what the last applied run left: the assets and albums as Photos
// had them, the album pairs, the links with the base of every cloud file, the
// earlier identities of replaced assets and the conflicts not closed yet, at
// most one for an asset in a remote. Memberships, parents, pairs and links
// refer to albums of the same State by AlbumID, and links, history and
// conflicts refer to its assets by AssetID.
type State struct {
	Assets     []Asset
	Albums     []Album
	AlbumPairs []AlbumPair
	Links      []Link
	History    []AssetHistory
	Conflicts  []Conflict
}

// AssetHistory is an identity that an asset of the state had before photosync
// replaced it with an asset imported from the cloud. Deleted reports whether
// the old asset went to Recently Deleted; until it does, the replacement is
// not complete.
type AssetHistory struct {
	AssetID AssetID
	Key     AssetKey
	Deleted bool
}
