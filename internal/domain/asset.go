package domain

import (
	"cmp"
	"time"
)

// AssetKey is the address of an asset of a snapshot in a plan, before the
// state gives the asset an AssetID. CloudAssetGUID is empty while iCloud has
// not given the asset one.
type AssetKey struct {
	CloudAssetGUID string
	UUID           string
}

// Compare orders keys as a run gives new assets their IDs: keys with a
// CloudAssetGUID first, by it, then keys without one; ties go by UUID.
// Strings compare byte by byte. The result is negative, zero or positive, as
// with cmp.Compare.
func (k AssetKey) Compare(o AssetKey) int {
	switch {
	case k.CloudAssetGUID == "" && o.CloudAssetGUID != "":
		return 1
	case k.CloudAssetGUID != "" && o.CloudAssetGUID == "":
		return -1
	}
	return cmp.Or(cmp.Compare(k.CloudAssetGUID, o.CloudAssetGUID), cmp.Compare(k.UUID, o.UUID))
}

// Asset is an asset of the photo library, as a snapshot reads it or as the
// state keeps it after the last applied run. Only the state fills ID,
// TargetLibrary and Memberships; only a snapshot fills AlbumUUIDs.
type Asset struct {
	ID  AssetID
	Key AssetKey

	Library       LibraryKind
	TargetLibrary LibraryKind // differs from Library while the asset waits to be moved
	OriginalName  string
	Taken         time.Time
	TZOffset      *int   // seconds east of UTC; nil when unknown, and Taken is then the wall clock in time.UTC, as in MediaMeta
	SubSec        string // digits of the fraction of a second as written, so "075" is not "75"; empty when unknown
	DateSource    DateSource

	Width, Height                             int // of the original file, as stored, before the orientation
	IsVideo, Hidden, BurstUnselected, InTrash bool
	Missing                                   bool  // the original is not on this Mac
	InCloud                                   *bool // nil when the library has no cloud record of the asset
	HasCaptionOrKeywords                      bool

	Resources   []Resource
	AlbumUUIDs  []string // the albums the asset is in; for an unselected frame of a burst, those of its key frame
	Memberships []Membership
}

// Resource is a file of an asset in the library. Filename is the name of the
// file with its extension, such as "IMG_0001.MOV". SHA256 and MD5 are
// lowercase hex and stay empty until photosync reads the file, so a
// snapshot leaves them empty.
type Resource struct {
	Role        ResourceRole
	Filename    string
	Size        int64
	SHA256, MD5 string
}

// Membership is an album that an asset of the state is in. FirstSeenRun is
// the ID of the run that first saw the asset in that album.
type Membership struct {
	AlbumID      AlbumID
	FirstSeenRun string
}
