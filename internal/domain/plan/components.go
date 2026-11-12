package plan

import (
	"path"

	"github.com/Windemiatrix/go-libs/osxphotos/export"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

type componentCase struct {
	remote domain.ChangeKind
	main   domain.LinkAction
}

// A component goes where its main file goes and leaves the cloud by the same
// action, so that the modes and the deletes policy treat both alike.
var componentTable = map[componentCase]outcome{
	{domain.ChangeUnchanged, domain.ActionNone}:           {domain.ActionNone, ""},
	{domain.ChangeUnchanged, domain.ActionMoveFile}:       {domain.ActionMoveFile, "R-COMPONENT-04"},
	{domain.ChangeUnchanged, domain.ActionCopyFile}:       {domain.ActionCopyFile, "R-OBJECT-03"},
	{domain.ChangeUnchanged, domain.ActionQuarantineFile}: {domain.ActionQuarantineFile, "R-COMPONENT-04"},
	{domain.ChangeUnchanged, domain.ActionDeleteFile}:     {domain.ActionDeleteFile, "R-COMPONENT-04"},
	{domain.ChangeMoved, domain.ActionNone}:               {domain.ActionMoveFile, "R-COMPONENT-02"},
	{domain.ChangeMoved, domain.ActionMoveFile}:           {domain.ActionMoveFile, "R-COMPONENT-02"},
	{domain.ChangeMoved, domain.ActionCopyFile}:           {domain.ActionCopyFile, "R-OBJECT-03"},
	{domain.ChangeMoved, domain.ActionQuarantineFile}:     {domain.ActionQuarantineFile, "R-COMPONENT-04"},
	{domain.ChangeMoved, domain.ActionDeleteFile}:         {domain.ActionDeleteFile, "R-COMPONENT-04"},
	{domain.ChangeDeleted, domain.ActionNone}:             {domain.ActionUpload, "R-COMPONENT-01"},
	{domain.ChangeDeleted, domain.ActionMoveFile}:         {domain.ActionUpload, "R-COMPONENT-01"},
	{domain.ChangeDeleted, domain.ActionCopyFile}:         {domain.ActionUpload, "R-COMPONENT-01"},
	{domain.ChangeDeleted, domain.ActionQuarantineFile}:   {domain.ActionForget, "R-COMPONENT-04"},
	{domain.ChangeDeleted, domain.ActionDeleteFile}:       {domain.ActionForget, "R-COMPONENT-04"},
	{domain.ChangeReplaced, domain.ActionNone}:            {domain.ActionReplaceAsset, "R-COMPONENT-03"},
	{domain.ChangeReplaced, domain.ActionMoveFile}:        {domain.ActionMoveFileReplaceAsset, "R-COMPONENT-03"},
	{domain.ChangeReplaced, domain.ActionCopyFile}:        {domain.ActionCopyFile, "R-OBJECT-03"},
	{domain.ChangeReplaced, domain.ActionQuarantineFile}:  {domain.ActionQuarantineFile, "R-COMPONENT-04"},
	{domain.ChangeReplaced, domain.ActionDeleteFile}:      {domain.ActionDeleteFile, "R-COMPONENT-04"},
}

// ComponentAction returns what a sync does with the link of a Live Photo
// video or a RAW file whose file changed in the cloud as remote says, and the
// ID of the rule in Rules behind it, empty for ActionNone. main is what
// becomes of the main file that the component lay beside in the base:
// ActionNone when it stays at its path and root; ActionMoveFile when it ends
// up elsewhere, moved by the user or by the plan; ActionCopyFile for each
// copy of it that the plan makes; ActionQuarantineFile when the plan moves it
// to quarantine whatever the deletes policy; ActionDeleteFile when it leaves
// the cloud under the deletes policy, deleted by the plan or by the user.
// same reports that a moved component already lies where the main file ends
// up; it counts only with ActionNone and ActionMoveFile. A move, an upload or
// a move with a replacement puts the component beside the main file where
// that ends up, and a copy beside the copy, at ComponentPath. ok is false for
// a component without a base or a main file with another action.
func ComponentAction(remote domain.ChangeKind, main domain.LinkAction, same bool) (action domain.LinkAction, rule string, ok bool) {
	o, found := componentTable[componentCase{remote, main}]
	if same && o.rule == "R-COMPONENT-02" {
		o = outcome{domain.ActionUpdateBase, "R-COMPONENT-04"}
	}
	return o.action, o.rule, found
}

// ComponentBeside reports whether c lies beside main as a component of its
// file: an active link of a Live Photo video or a RAW file of the same asset,
// remote and root, in the directory of main byte for byte and with the
// export.StemKey of its name, the key by which the cloud files of one shot go
// together for matching. With main where it ends up and c where it lies now,
// it gives same for ComponentAction.
func ComponentBeside(main, c domain.Link) bool {
	return c.AssetID == main.AssetID && c.Remote == main.Remote && c.Root == main.Root &&
		(c.Role == domain.RoleLiveVideo || c.Role == domain.RoleRAWPair) && c.Status == domain.StatusActive &&
		path.Dir(c.Path) == path.Dir(main.Path) && export.StemKey(path.Base(c.Path)) == export.StemKey(path.Base(main.Path))
}

// ComponentPath returns the path of the component file with role beside the
// main file at path main: in the directory of main, with the name that
// naming.Companion gives from the name of main and from the name of the
// resource of asset with that role, which asset holds. A Live Photo video so
// ends in .mov whatever the cloud called it, once it moves, is copied or is
// uploaded; a component that stays keeps its name.
func ComponentPath(main string, role domain.LinkRole, asset domain.Asset) string {
	var name string
	// The roles of a component link and of its resource store the same strings.
	for _, r := range asset.Resources {
		if r.Role == domain.ResourceRole(role) {
			name = r.Filename
		}
	}
	return path.Join(path.Dir(main), naming.Companion(path.Base(main), domain.ResourceRole(role), name))
}
