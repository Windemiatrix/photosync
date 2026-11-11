package domain

import "time"

// HashKind is a hash of file content that a remote reports in its listing.
type HashKind string

// The hashes. HashNone is the zero value: a remote that reports no hash is
// never trusted with one, or a stranger's file of the same size would pass for
// the one photosync wrote.
const (
	HashNone   HashKind = ""
	HashMD5    HashKind = "md5"
	HashSHA256 HashKind = "sha256"
)

// Capabilities is what a remote tells about itself: after a write, its listing
// gives the ContentHash of every file.
type Capabilities struct {
	ContentHash HashKind
}

// RemoteEntry is a file or a directory in the listing of a remote. Root is set
// by the caller of the remote: a remote knows paths, not libraries. Path is
// relative to the root of that library in the remote. MD5 and SHA256 are
// lowercase hex. ETag changes whenever the content does. A hash, ETag or MTime
// that the remote does not report is zero. Meta is zero until the scan reads
// the metadata of the file.
type RemoteEntry struct {
	Root              LibraryKind
	Path              string
	IsDir             bool
	Size              int64
	ETag, MD5, SHA256 string
	MTime             time.Time
	Meta              MediaMeta
}

// MediaMeta is what the metadata of a media file tells. A zero Taken: the file
// holds no capture time. With a nil TZOffset, Taken is the wall clock of an
// unknown zone in time.UTC; otherwise it is the moment, and TZOffset is the
// offset of the place in seconds east of UTC. Taken has whole seconds; SubSec
// holds the digits of the fraction as the file writes them ("075" is not
// "75"). Width and Height are the pixels as stored, before the orientation;
// zero when unknown.
type MediaMeta struct {
	Taken         time.Time
	TZOffset      *int
	SubSec        string
	Width, Height int
}

// PutHints describes the content of a Put. MD5 is its lowercase hex MD5; a
// remote that can check it after the write fails the Put on a mismatch.
type PutHints struct {
	MD5 string
}
