package app

import (
	"context"
	"io"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// PhotoLibrary is the Photos library of this Mac. It names assets, albums and
// folders by their UUID in the library, as snapshots do. Each change is one
// change of the library, all or none. A change fails with
// domain.ErrUserCanceled when the user refuses its dialog, and then nothing
// was changed. When ctx ends before the library reports the outcome, a change
// fails with domain.ErrOutcomeUnknown wrapped together with the error of ctx:
// the change may still take effect, by a later answer to its dialog too, so
// the library is read again before the next change. A missing or hidden asset
// and a missing album or folder fail a call with domain.ErrNotFound, before
// any change and without a dialog.
type PhotoLibrary interface {
	// Snapshot reads the library at libraryPath without changing it. It
	// leaves out what is not the user's own: the assets of shared albums and
	// the items shared with the user that are not saved to the library. Every
	// other asset is in it, or Snapshot fails: an asset left out would read
	// as deleted.
	Snapshot(ctx context.Context, libraryPath string) (domain.LibrarySnapshot, error)

	// Download writes the files of the asset into dir, an existing empty
	// directory, and returns one LocalFile for each resource that a snapshot
	// lists for the asset. Every file is inside dir, whatever the names in the
	// library. An original that is only in iCloud is fetched from there and
	// then also kept in the library, on its volume. After an error dir may
	// hold some of the files. progress, when not nil, gets the bytes received
	// so far, maybe from another goroutine, and is not called after Download
	// returns.
	Download(ctx context.Context, uuid, dir string, progress func(received int64)) ([]domain.LocalFile, error)

	// Import creates one asset from req in the personal library, without a
	// dialog, and returns its UUID. Snapshots may give the asset an empty
	// CloudAssetGUID until iCloud has synced it. req.OriginalName is a name
	// from the cloud and goes to the library as it is, not normalized; one
	// that holds a NUL or is not UTF-8 fails the Import with nothing created.
	Import(ctx context.Context, req domain.ImportRequest) (uuid string, err error)

	// DeleteAssets moves the assets to Recently Deleted after one dialog for
	// the whole batch; snapshots keep them, with InTrash set. An empty uuids
	// changes nothing and asks nothing.
	DeleteAssets(ctx context.Context, uuids []string) error

	// UpdateAsset changes the asset as p says; a zero p changes nothing.
	// Hiding asks the user. A hidden asset stays in snapshots, but the other
	// methods no longer find it, so it cannot be shown again.
	UpdateAsset(ctx context.Context, uuid string, p domain.AssetPatch) error

	// CreateFolder makes a folder titled title in the folder parentUUID, or at
	// the top level when parentUUID is empty, and returns its UUID. A
	// parentUUID that is not a folder is domain.ErrNotFound. A title that is
	// empty, holds a NUL or is not UTF-8 fails with nothing created.
	CreateFolder(ctx context.Context, title, parentUUID string) (uuid string, err error)
	// CreateAlbum is CreateFolder for an album.
	CreateAlbum(ctx context.Context, title, parentUUID string) (uuid string, err error)
	// RenameCollection sets the title of a folder or an album; the title is
	// checked as for CreateFolder.
	RenameCollection(ctx context.Context, uuid, title string) error
	// DeleteCollections removes the folders and albums, not their assets; an
	// album in a removed folder goes with it. It may ask the user once. An
	// empty uuids changes nothing.
	DeleteCollections(ctx context.Context, uuids []string) error
	// AddToAlbum adds the assets to the album. An asset already in it is no
	// change, so a repeated call succeeds.
	AddToAlbum(ctx context.Context, albumUUID string, assetUUIDs []string) error
	// RemoveFromAlbum takes the assets out of the album; an asset that is not
	// in it is no change.
	RemoveFromAlbum(ctx context.Context, albumUUID string, assetUUIDs []string) error

	// SystemLibraryPath gives the path of the system library, the only one
	// that the changes of PhotoLibrary reach.
	SystemLibraryPath() (string, error)
	// LibraryIdentity gives a string that stays the same for the library at
	// libraryPath and differs for any other library, one made later at the
	// same path included.
	LibraryIdentity(ctx context.Context, libraryPath string) (string, error)
}

// SpaceManager reports and makes free space on the disks of this Mac. Sizes
// are bytes.
type SpaceManager interface {
	// Free gives the space that macOS reports free for an important use on
	// the volume that holds path.
	Free(ctx context.Context, path string) (int64, error)
	// LibrarySize gives the space that the library at libraryPath takes.
	LibrarySize(ctx context.Context, libraryPath string) (int64, error)
	// OptimizeStorageEnabled reports whether Photos keeps only smaller
	// versions on this Mac, so that a download brings originals from iCloud
	// into the library.
	OptimizeStorageEnabled(ctx context.Context) (bool, error)
	// Pressure fills a new file in dir with up to bytes, never leaving less
	// than keepFree free on its volume, so that macOS evicts the originals
	// that Photos keeps. The caller calls release, which removes the file, on
	// every way out, a canceled ctx included. After an error release is nil
	// and nothing is left in dir.
	Pressure(ctx context.Context, dir string, bytes, keepFree int64) (release func(), err error)
}

// NetworkMonitor tells whether the network in use costs the user money or
// data, as a phone hotspot or Low Data Mode does, so that transfers can wait
// for another.
type NetworkMonitor interface {
	IsExpensive(ctx context.Context) (bool, error)
}

// Converter makes MP4 from a video that Photos does not import. Both methods
// fail with domain.ErrNotSupported when no converter is installed.
type Converter interface {
	// ToMP4 writes the MP4 of the file src to dst, where no file is; both
	// are absolute local paths. After an error dst does not exist.
	ToMP4(ctx context.Context, src, dst string) error
	// Version gives the version of the converter, for the reports.
	Version(ctx context.Context) (string, error)
}

// Remote is a store of files that holds the roots of the libraries. Its
// paths use package path, are relative to the top of the store and are
// passed and returned as the store keeps them, not normalized: names that
// differ only in Unicode form or in case stay apart. A call fails with
// domain.ErrNotFound when a path that it reads, moves or removes is
// missing, and a call that writes a path needs its directory made first. A
// call that would change a path outside the roots that the remote was made
// with, or move or remove a root, fails with nothing changed; so does a
// path argument that starts with "/", has an empty, "." or ".." element, or
// holds a NUL. Its methods may be called from several goroutines at once.
type Remote interface {
	Capabilities() domain.Capabilities

	// Walk calls fn for every file and directory below root, in any order,
	// never two calls at once, and reads listings, not files. Path of an
	// entry is relative to root. Walk leaves nothing out, for excluding
	// paths is up to the caller, and fn cannot prune a subtree: an error from
	// fn ends the Walk with that error. An entry whose path is not below
	// root, as one that starts with "/" or has an empty, "." or ".."
	// element, or holds a NUL fails the whole Walk, and so does an error on
	// any branch: a Walk that succeeds has listed everything.
	Walk(ctx context.Context, root string, fn func(domain.RemoteEntry) error) error
	// Stat gives the entry at path, as Walk would, with an empty Path.
	Stat(ctx context.Context, path string) (domain.RemoteEntry, error)
	// Get reads the file at path from byte offset on: length bytes, or up to
	// the end when length is negative.
	Get(ctx context.Context, path string, offset, length int64) (io.ReadCloser, error)

	// Put writes the file path from size bytes of r, replacing a file that
	// is there; an r that gives more or fewer than size bytes fails the Put.
	// After an error path may hold a part of the file.
	Put(ctx context.Context, path string, r io.Reader, size int64, h domain.PutHints) error
	// Mkdir makes the directory path and its missing parents; a directory
	// that is there already is no change, and a file on the way is
	// domain.ErrExists.
	Mkdir(ctx context.Context, path string) error
	// Move moves the file src to dst in one step. A dst that is taken is
	// domain.ErrExists, and then nothing is moved; so is a directory at src.
	Move(ctx context.Context, src, dst string) error
	// Copy is Move that keeps src.
	Copy(ctx context.Context, src, dst string) error
	// Delete removes the file at path; a directory there fails the Delete
	// with nothing removed.
	Delete(ctx context.Context, path string) error
	// RemoveEmptyDir removes the directory path; one that holds anything is
	// domain.ErrNotEmpty, and then nothing is removed.
	RemoveEmptyDir(ctx context.Context, path string) error

	// Quota gives the bytes used and the bytes still free in the store; a
	// store that does not tell fails with domain.ErrNotSupported.
	Quota(ctx context.Context) (used, available int64, err error)
}

// MediaProbe reads the metadata of a photo or a video by its content alone:
// the extension of a name in the cloud may lie.
type MediaProbe interface {
	// Probe reads src, a file of size bytes, only within its first 4 MiB and
	// its last 4 MiB, so that a long video is not downloaded whole. What it
	// cannot read is left zero, a file of a format it does not know included:
	// Probe fails only when src or ctx does.
	Probe(ctx context.Context, src io.ReaderAt, size int64) (domain.MediaMeta, error)
}

// UI asks the user and shows the progress of a run on the standard error,
// apart from the result of a command on the standard output. Its methods may
// be called from several goroutines at once. With no terminal to ask in, a
// question fails at once with domain.ErrNoTTY, so that a run never waits for
// an answer that cannot come.
type UI interface {
	// Confirm reports whether the user answers prompt with yes in any
	// language of the interface; any other answer, an empty line and the end
	// of input are no.
	Confirm(ctx context.Context, prompt string) (bool, error)
	// Input gives the line that the user types after prompt, without the line
	// break, and io.EOF at the end of input.
	Input(ctx context.Context, prompt string) (string, error)
	// Print shows msg on lines of its own, never inside the progress line.
	Print(msg string)

	// ProgressStart shows the progress of stage out of totalItems items and
	// totalBytes bytes until ProgressDone.
	ProgressStart(stage string, totalItems, totalBytes int64)
	// ProgressAdd counts items and bytes more as done.
	ProgressAdd(items, bytes int64)
	ProgressDone()
}
