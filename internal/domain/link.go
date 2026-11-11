package domain

import "time"

// Link is a file of a remote root that stands for an asset, with the base of
// the three-way comparison: the file and its album as the last applied run
// left them. AlbumID is zero when the file lies outside the directory of an
// album, as in a day folder. Hashes are lowercase hex, empty when unknown.
type Link struct {
	ID      LinkID
	AssetID AssetID
	Remote  string
	Root    LibraryKind
	Path    string
	Role    LinkRole
	AlbumID AlbumID

	Size        int64
	SHA256, MD5 string
	ETag        string
	MTime       time.Time

	Level          MatchLevel
	Status         LinkStatus
	QuarantinePath string // where the file lies while in quarantine; empty otherwise
}
