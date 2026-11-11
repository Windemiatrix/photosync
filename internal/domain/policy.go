package domain

// ConflictPolicy is what a sync does with an asset that iCloud and a remote
// changed in different ways.
type ConflictPolicy string

// The conflict policies.
const (
	ConflictRemoteWins ConflictPolicy = "remote_wins"
	ConflictICloudWins ConflictPolicy = "icloud_wins"
	ConflictAsk        ConflictPolicy = "ask"
	ConflictSkip       ConflictPolicy = "skip"
)

// DeletesPolicy is what a sync does on one side with an asset or a file that
// the user deleted on the other.
type DeletesPolicy string

// The deletes policies.
const (
	DeletesNone       DeletesPolicy = "none"
	DeletesQuarantine DeletesPolicy = "quarantine"
	DeletesPropagate  DeletesPolicy = "propagate"
)

// DerivedCopyPolicy is what a sync does when a remote holds another copy of
// the shot of an asset, such as a JPEG exported from its RAW.
type DerivedCopyPolicy string

// The derived copy policies.
const (
	DerivedReplaceWithOriginal DerivedCopyPolicy = "replace_with_original"
	DerivedKeepBoth            DerivedCopyPolicy = "keep_both"
	DerivedKeepRemote          DerivedCopyPolicy = "keep_remote"
)

// MultiAlbumPolicy is what the first placement of an asset in several event
// albums does with the events other than the one closest to its date.
type MultiAlbumPolicy string

// The multi-album policies.
const (
	MultiAlbumKeepOthers   MultiAlbumPolicy = "closest_date_keep_others"
	MultiAlbumRemoveOthers MultiAlbumPolicy = "closest_date_remove_others"
)

// UnsupportedFormatsPolicy is what a sync does with a cloud video in a format
// that Photos does not take.
type UnsupportedFormatsPolicy string

// The unsupported formats policies.
const (
	UnsupportedConvertMP4 UnsupportedFormatsPolicy = "convert_mp4"
	UnsupportedRemoteOnly UnsupportedFormatsPolicy = "remote_only"
)

// EditedFilesPolicy is what a sync does with the cloud files of edits.
type EditedFilesPolicy string

// The edited files policies.
const (
	EditedIgnore     EditedFilesPolicy = "ignore"
	EditedQuarantine EditedFilesPolicy = "quarantine"
)

// HiddenPolicy says whether the assets hidden in Photos sync.
type HiddenPolicy string

// The hidden policies.
const (
	HiddenSync HiddenPolicy = "sync"
	HiddenSkip HiddenPolicy = "skip"
)

// BurstPolicy says whether the burst shots that the user did not pick sync.
type BurstPolicy string

// The burst policies.
const (
	BurstAll          BurstPolicy = "all"
	BurstSelectedOnly BurstPolicy = "selected_only"
)

// Policies are the rules of a sync that the user chooses; one set serves all
// remotes.
type Policies struct {
	Conflict           ConflictPolicy
	Deletes            DeletesPolicy
	DerivedCopy        DerivedCopyPolicy
	MultiAlbum         MultiAlbumPolicy
	UnsupportedFormats UnsupportedFormatsPolicy
	EditedFiles        EditedFilesPolicy
	Hidden             HiddenPolicy
	Burst              BurstPolicy
	DeleteEmptyAlbums  bool
	AlbumsForPersonal  bool
	// RemotePriority names remotes; when they change an asset in different
	// ways, the one listed first wins.
	RemotePriority []string
	// EditedDirs names the cloud directories whose files are edits.
	EditedDirs []string
	// SettleMinutes is how long a cloud file must stay unchanged before a sync
	// touches it.
	SettleMinutes int
	// UntitledEvent stands for the missing title in the cloud directory of an
	// event album without one: "MM-DD - " followed by it.
	UntitledEvent string
}

// DefaultPolicies returns the policies for the keys that a configuration
// leaves out, with new slices on each call. Deletes stays empty because the
// user must choose it, UntitledEvent because it is in the language chosen at
// setup, and RemotePriority because it is the order of the remotes in the
// configuration.
func DefaultPolicies() Policies {
	return Policies{
		Conflict:           ConflictRemoteWins,
		DerivedCopy:        DerivedReplaceWithOriginal,
		MultiAlbum:         MultiAlbumRemoveOthers,
		UnsupportedFormats: UnsupportedConvertMP4,
		EditedFiles:        EditedIgnore,
		Hidden:             HiddenSync,
		Burst:              BurstAll,
		AlbumsForPersonal:  true,
		EditedDirs:         []string{"отредактировано", "edited"},
		SettleMinutes:      10,
	}
}
