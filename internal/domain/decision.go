package domain

// ChangeKind is how one side of a link differs from its base: the asset in
// iCloud or the file in the cloud. A side with no base yet is ChangeNone or
// ChangeNew, and then so is the other side.
type ChangeKind string

// The changes. An asset moves when it changes album or library, and is
// deleted when it is gone or in Recently Deleted; an edit of an asset is no
// change, because only originals sync, so only a file is replaced. A file
// moves when its content turns up under another path.
const (
	ChangeNone      ChangeKind = "none"
	ChangeNew       ChangeKind = "new"
	ChangeUnchanged ChangeKind = "unchanged"
	ChangeMoved     ChangeKind = "moved"
	ChangeReplaced  ChangeKind = "replaced"
	ChangeDeleted   ChangeKind = "deleted"
)

// LinkAction is what a sync does with a link, or with a new asset or a new
// file that has no link yet. A file is in the cloud and an asset in iCloud.
type LinkAction string

// The link actions. ActionLink ties a new asset to the new file that
// matching paired with it, without a transfer. ActionDeleteFile and
// ActionDeleteAsset leave the way to delete to the deletes policy, and
// ActionConflict the winner to the conflict policy. ActionReplaceAsset
// imports the file in place of the asset. ActionForget drops the link from
// the state and ActionUpdateBase takes both sides into it, both without
// operations. ActionCopyFile copies a file of the remote into another
// directory as a new link, and ActionQuarantineFile moves a needless copy to
// the quarantine whatever the deletes policy.
const (
	ActionNone                 LinkAction = "none"
	ActionUpload               LinkAction = "upload"
	ActionImport               LinkAction = "import"
	ActionLink                 LinkAction = "link"
	ActionMoveFile             LinkAction = "move_file"
	ActionMoveAsset            LinkAction = "move_asset"
	ActionReplaceAsset         LinkAction = "replace_asset"
	ActionMoveFileReplaceAsset LinkAction = "move_file_replace_asset"
	ActionConflict             LinkAction = "conflict"
	ActionDeleteFile           LinkAction = "delete_file"
	ActionDeleteAsset          LinkAction = "delete_asset"
	ActionForget               LinkAction = "forget"
	ActionUpdateBase           LinkAction = "update_base"
	ActionCopyFile             LinkAction = "copy_file"
	ActionQuarantineFile       LinkAction = "quarantine_file"
)

// LinkDecision is what a sync does with one link, or with a new asset or
// file of a remote, and why: the change on each side and the ID of the rule
// in Rules that chose the action, empty for ActionNone.
type LinkDecision struct {
	ICloud, Remote ChangeKind
	Action         LinkAction
	Rule           string
}

// Placement is a change that makes the files of an asset in a remote lie in
// the directories of its albums: ActionMoveFile, ActionCopyFile or
// ActionUpload into the directory of the album AlbumID, zero for the day
// folder, or ActionQuarantineFile. Link is the file moved, copied or
// quarantined as the remote has it now, zero for an upload. Rule is the ID
// of the rule in Rules that chose the change.
type Placement struct {
	Action  LinkAction
	Link    Link
	AlbumID AlbumID
	Rule    string
}
