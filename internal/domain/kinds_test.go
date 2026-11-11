package domain_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// The state and the configuration store these strings: changing one breaks
// the states and configurations already written.
func TestStoredValues(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.LibraryKind{domain.LibraryShared, domain.LibraryPersonal},
		"shared", "personal")
	checkStored(t, []domain.ResourceRole{domain.ResourceOriginal, domain.ResourceLiveVideo, domain.ResourceRAWPair},
		"original", "live_video", "raw_pair")
	checkStored(t, []domain.LinkRole{domain.RoleOriginal, domain.RoleLiveVideo, domain.RoleRAWPair, domain.RoleSource, domain.RoleDerived},
		"original", "live_video", "raw_pair", "source", "derived")
	checkStored(t, []domain.LinkStatus{domain.StatusActive, domain.StatusQuarantinedICloud, domain.StatusQuarantinedRemote, domain.StatusDetached},
		"active", "quarantined_icloud", "quarantined_remote", "detached")
	checkStored(t, []domain.MatchLevel{
		domain.LevelExact, domain.LevelSameShot, domain.LevelSameShotWeak, domain.LevelSameTimeSize, domain.LevelProbable,
		domain.LevelPairComponent, domain.LevelUploaded, domain.LevelImported, domain.LevelImportedState,
	},
		"exact", "same_shot", "same_shot_weak", "same_time_size", "probable",
		"pair_component", "uploaded", "imported", "imported_state")
	checkStored(t, []domain.AlbumKind{domain.KindFolder, domain.KindEvent, domain.KindThematic},
		"folder", "event", "thematic")
	checkStored(t, []domain.KeySource{domain.KeyCloudGUID, domain.KeyCloudID, domain.KeyPath},
		"zcloudguid", "phcloudid", "path")
	checkStored(t, []domain.PairOrigin{domain.OriginICloud, domain.OriginRemote, domain.OriginImport},
		"icloud", "remote", "import")
	checkStored(t, []domain.DateSource{domain.DateICloud, domain.DateMetadata, domain.DateFilename, domain.DateFolder, domain.DateMTime},
		"icloud", "metadata", "filename", "folder", "mtime")
	checkStored(t, []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly},
		"two-way", "upload-only", "download-only")
}

func checkStored[T ~string](t *testing.T, got []T, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%T: %d constants, want %d", got, len(got), len(want))
		return
	}
	seen := make(map[T]bool, len(got))
	for i, v := range got {
		if string(v) != want[i] {
			t.Errorf("%T[%d] = %q, want %q", got, i, v, want[i])
		}
		if seen[v] {
			t.Errorf("%T: two constants store %q", got, v)
		}
		seen[v] = true
	}
}
