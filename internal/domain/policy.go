package domain

import (
	"errors"
	"fmt"
	"slices"
)

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
	// NestedSeparator divides an album title into the levels of its cloud
	// directory.
	NestedSeparator string
	// AlbumsRoot is the top-level cloud directory of the albums and folders
	// outside the year folders.
	AlbumsRoot string
}

// DefaultPolicies returns the policies for the keys that a configuration
// leaves out, with new slices on each call. Deletes stays empty because the
// user must choose it, UntitledEvent and AlbumsRoot because they are in the
// language chosen at setup, and RemotePriority because it is the order of
// the remotes in the configuration.
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
		NestedSeparator:    " / ",
	}
}

// Validate returns nil, or an errors.Join of one *PolicyError for each key
// with a value that photosync does not take, in the order of the fields. An
// enumeration takes only its constants, spelled exactly; Deletes,
// UntitledEvent, NestedSeparator and AlbumsRoot must not be empty, and
// SettleMinutes must not be negative. The caller checks the rest:
// RemotePriority against its remotes, EditedDirs, UntitledEvent and
// AlbumsRoot as cloud names with naming.IsValidPart, and NestedSeparator
// against the day form of an event title.
func (p Policies) Validate() error {
	var errs []error
	errs = checkPolicy(errs, "conflict", p.Conflict, ConflictRemoteWins, ConflictICloudWins, ConflictAsk, ConflictSkip)
	errs = checkPolicy(errs, "deletes", p.Deletes, DeletesNone, DeletesQuarantine, DeletesPropagate)
	errs = checkPolicy(errs, "derived_copy", p.DerivedCopy, DerivedReplaceWithOriginal, DerivedKeepBoth, DerivedKeepRemote)
	errs = checkPolicy(errs, "multi_album", p.MultiAlbum, MultiAlbumKeepOthers, MultiAlbumRemoveOthers)
	errs = checkPolicy(errs, "unsupported_formats", p.UnsupportedFormats, UnsupportedConvertMP4, UnsupportedRemoteOnly)
	errs = checkPolicy(errs, "edited_files", p.EditedFiles, EditedIgnore, EditedQuarantine)
	errs = checkPolicy(errs, "hidden", p.Hidden, HiddenSync, HiddenSkip)
	errs = checkPolicy(errs, "burst", p.Burst, BurstAll, BurstSelectedOnly)
	if p.SettleMinutes < 0 {
		errs = append(errs, &PolicyError{Key: "settle_minutes", Value: fmt.Sprint(p.SettleMinutes)})
	}
	if p.UntitledEvent == "" {
		errs = append(errs, &PolicyError{Key: "untitled_event"})
	}
	if p.NestedSeparator == "" {
		errs = append(errs, &PolicyError{Key: "nested_separator"})
	}
	if p.AlbumsRoot == "" {
		errs = append(errs, &PolicyError{Key: "albums_root"})
	}
	return errors.Join(errs...)
}

func checkPolicy[T ~string](errs []error, key string, v T, known ...T) []error {
	if slices.Contains(known, v) {
		return errs
	}
	return append(errs, &PolicyError{Key: key, Value: string(v)})
}

// PolicyError is a key of a configuration with a value that photosync does
// not take. Key is the key without its section; Value is the value as given,
// and "" stands for a missing or empty value.
type PolicyError struct {
	Key, Value string
}

func (e *PolicyError) Error() string {
	return fmt.Sprintf("domain: policy %s: invalid value %q", e.Key, e.Value)
}
