package domain_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Explanations show these strings: two equal ones would pass one decision
// for another.
func TestDecisionValues(t *testing.T) {
	t.Parallel()
	checkStored(t, []domain.ChangeKind{
		domain.ChangeNone, domain.ChangeNew, domain.ChangeUnchanged,
		domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted,
	},
		"none", "new", "unchanged", "moved", "replaced", "deleted")
	checkStored(t, []domain.LinkAction{
		domain.ActionNone, domain.ActionUpload, domain.ActionImport, domain.ActionLink,
		domain.ActionMoveFile, domain.ActionMoveAsset, domain.ActionReplaceAsset,
		domain.ActionMoveFileReplaceAsset, domain.ActionConflict, domain.ActionDeleteFile,
		domain.ActionDeleteAsset, domain.ActionForget, domain.ActionUpdateBase,
		domain.ActionCopyFile, domain.ActionQuarantineFile,
	},
		"none", "upload", "import", "link",
		"move_file", "move_asset", "replace_asset",
		"move_file_replace_asset", "conflict", "delete_file",
		"delete_asset", "forget", "update_base",
		"copy_file", "quarantine_file")
}
