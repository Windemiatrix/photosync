package domain_test

import (
	"reflect"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// The configuration stores these strings: changing one breaks the
// configurations already written.
func TestPolicyValues(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip},
		"remote_wins", "icloud_wins", "ask", "skip")
	checkStored(t, []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate},
		"none", "quarantine", "propagate")
	checkStored(t, []domain.DerivedCopyPolicy{domain.DerivedReplaceWithOriginal, domain.DerivedKeepBoth, domain.DerivedKeepRemote},
		"replace_with_original", "keep_both", "keep_remote")
	checkStored(t, []domain.MultiAlbumPolicy{domain.MultiAlbumKeepOthers, domain.MultiAlbumRemoveOthers},
		"closest_date_keep_others", "closest_date_remove_others")
	checkStored(t, []domain.UnsupportedFormatsPolicy{domain.UnsupportedConvertMP4, domain.UnsupportedRemoteOnly},
		"convert_mp4", "remote_only")
	checkStored(t, []domain.EditedFilesPolicy{domain.EditedIgnore, domain.EditedQuarantine},
		"ignore", "quarantine")
	checkStored(t, []domain.HiddenPolicy{domain.HiddenSync, domain.HiddenSkip},
		"sync", "skip")
	checkStored(t, []domain.BurstPolicy{domain.BurstAll, domain.BurstSelectedOnly},
		"all", "selected_only")
}

// The oracle spells the values as the configuration stores them, not with
// the constants, so that a wrong constant cannot hide a wrong default.
func TestDefaultPolicies(t *testing.T) {
	t.Parallel()
	want := domain.Policies{
		Conflict:           "remote_wins",
		DerivedCopy:        "replace_with_original",
		MultiAlbum:         "closest_date_remove_others",
		UnsupportedFormats: "convert_mp4",
		EditedFiles:        "ignore",
		Hidden:             "sync",
		Burst:              "all",
		AlbumsForPersonal:  true,
		EditedDirs:         []string{"отредактировано", "edited"},
		SettleMinutes:      10,
	}
	got := domain.DefaultPolicies()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DefaultPolicies() = %+v, want %+v", got, want)
	}
	got.EditedDirs[0] = "changed"
	if next := domain.DefaultPolicies(); next.EditedDirs[0] != "отредактировано" {
		t.Errorf("DefaultPolicies().EditedDirs[0] = %q after a change to an earlier result, want %q", next.EditedDirs[0], "отредактировано")
	}
}
