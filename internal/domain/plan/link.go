package plan

import "github.com/Windemiatrix/photosync/internal/domain"

type sides struct{ icloud, remote domain.ChangeKind }

type outcome struct {
	action domain.LinkAction
	rule   string
}

var linkTable = map[sides]outcome{
	{domain.ChangeNew, domain.ChangeNone}:            {domain.ActionUpload, "R-LINK-01"},
	{domain.ChangeNone, domain.ChangeNew}:            {domain.ActionImport, "R-LINK-02"},
	{domain.ChangeNew, domain.ChangeNew}:             {domain.ActionLink, "R-LINK-03"},
	{domain.ChangeUnchanged, domain.ChangeUnchanged}: {domain.ActionNone, ""},
	{domain.ChangeMoved, domain.ChangeUnchanged}:     {domain.ActionMoveFile, "R-LINK-04"},
	{domain.ChangeUnchanged, domain.ChangeMoved}:     {domain.ActionMoveAsset, "R-LINK-05"},
	{domain.ChangeUnchanged, domain.ChangeReplaced}:  {domain.ActionReplaceAsset, "R-LINK-06"},
	{domain.ChangeMoved, domain.ChangeMoved}:         {domain.ActionConflict, "R-LINK-07"},
	{domain.ChangeDeleted, domain.ChangeUnchanged}:   {domain.ActionDeleteFile, "R-LINK-08"},
	{domain.ChangeUnchanged, domain.ChangeDeleted}:   {domain.ActionDeleteAsset, "R-LINK-09"},
	{domain.ChangeMoved, domain.ChangeDeleted}:       {domain.ActionUpload, "R-LINK-10"},
	{domain.ChangeDeleted, domain.ChangeMoved}:       {domain.ActionImport, "R-LINK-11"},
	{domain.ChangeDeleted, domain.ChangeReplaced}:    {domain.ActionImport, "R-LINK-11"},
	{domain.ChangeDeleted, domain.ChangeDeleted}:     {domain.ActionForget, "R-LINK-12"},
	{domain.ChangeMoved, domain.ChangeReplaced}:      {domain.ActionMoveFileReplaceAsset, "R-LINK-14"},
}

// DecideLink returns the decision on a link whose asset changed in iCloud as
// icloud says and whose file changed in the cloud as remote says. same
// reports that the file moved to the place the asset moved to; it counts
// only when both sides moved. The result is false for changes that cannot
// meet on one link: a base on one side only, nothing on either side, or a
// replaced asset. The mode of the remote and the policies do not change the
// decision: they apply to it afterwards.
func DecideLink(icloud, remote domain.ChangeKind, same bool) (domain.LinkDecision, bool) {
	o, ok := linkTable[sides{icloud, remote}]
	if same && o.action == domain.ActionConflict {
		o = outcome{domain.ActionUpdateBase, "R-LINK-13"}
	}
	return domain.LinkDecision{ICloud: icloud, Remote: remote, Action: o.action, Rule: o.rule}, ok
}
