package domain

import "time"

// LocalFile is a file of an asset copied to the local disk. Path is a path of
// the local file system, not of a remote.
type LocalFile struct {
	Role ResourceRole
	Path string
}

// ImportRequest is one asset to create in the photo library. Exactly one of
// Photo and Video is the original; LiveVideo makes a Live Photo of Photo, and
// RAWPair the RAW of a RAW+JPEG pair. Every file is an absolute local path,
// which the library copies. An empty OriginalName keeps the name of the file.
// A zero Taken leaves the capture time to the library; any other is a moment.
// The asset joins the albums of AlbumUUIDs in the same change.
type ImportRequest struct {
	Photo, Video, LiveVideo, RAWPair string
	OriginalName                     string
	Taken                            time.Time
	AlbumUUIDs                       []string
}

// AssetPatch is a change of an asset in the photo library; a nil field is left
// as it is.
type AssetPatch struct {
	Hidden *bool
}
