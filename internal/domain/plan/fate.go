package plan

import (
	"cmp"
	"maps"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DecideFate returns objects with the fate of their asset decided across the
// remotes. objects are as DecideTarget takes them but before StatusDeletes,
// and modes give the mode of each of their remotes. A change of the asset in
// any remote, which is a decision whose iCloud side is moved or new or whose
// action is ActionMoveAsset or ActionReplaceAsset, beats a deletion: each
// ActionDeleteAsset becomes what ModeDecide gives for its file with the
// asset moved in iCloud, an upload by R-LINK-10 two-way, while a
// download-only remote, which does not see iCloud, still deletes. A deletion
// that stands reaches every remote through iCloud in the same run unless
// deletes is none: every other decision on an active link of the original or
// the source role becomes what ModeDecide, or ModeKeepAlbums for a decision
// taken by R-MODE-04, gives for its file with the asset deleted in iCloud. Of
// the decisions that replace the asset, ActionReplaceAsset and
// ActionMoveFileReplaceAsset, the first stands and each other becomes
// ActionUpdateBase by R-OBJECT-09, the base taking its file as it is: the
// remotes in the order of priority, those that it does not name last in the
// order of their names, and the decisions of a remote in their order. Each
// decision keeps its Link and Remote side, Join and Leave are as given, and
// objects are not changed.
func DecideFate(objects map[string]domain.ObjectDecision, modes map[string]domain.Mode, priority []string, deletes domain.DeletesPolicy) map[string]domain.ObjectDecision {
	names := slices.Sorted(maps.Keys(objects))
	rank := func(name string) int {
		if i := slices.Index(priority, name); i >= 0 {
			return i
		}
		return len(priority)
	}
	slices.SortStableFunc(names, func(x, y string) int { return cmp.Compare(rank(x), rank(y)) })
	redo := func(name string, d domain.LinkDecision, icloud domain.ChangeKind) domain.LinkDecision {
		r, _ := ModeDecide(modes[name], icloud, d.Remote, false)
		if d.Rule == "R-MODE-04" {
			r, _ = ModeKeepAlbums(modes[name], icloud, d.Remote, false)
		}
		r.Link = d.Link
		return r
	}
	changed := false
	for _, o := range objects {
		changed = changed || slices.ContainsFunc(o.Links, func(d domain.LinkDecision) bool {
			return d.ICloud == domain.ChangeMoved || d.ICloud == domain.ChangeNew ||
				d.Action == domain.ActionMoveAsset || d.Action == domain.ActionReplaceAsset
		})
	}
	fate, deleted, replaced := map[string]domain.ObjectDecision{}, false, false
	for _, name := range names {
		o := objects[name]
		o.Links = slices.Clone(o.Links)
		for i, d := range o.Links {
			replaces := d.Action == domain.ActionReplaceAsset || d.Action == domain.ActionMoveFileReplaceAsset
			switch {
			case d.Action == domain.ActionDeleteAsset && changed:
				o.Links[i] = redo(name, d, domain.ChangeMoved)
			case replaces && replaced:
				o.Links[i].Action, o.Links[i].Rule = domain.ActionUpdateBase, "R-OBJECT-09"
			}
			replaced = replaced || replaces
			deleted = deleted || o.Links[i].Action == domain.ActionDeleteAsset
		}
		fate[name] = o
	}
	if !deleted || deletes == domain.DeletesNone {
		return fate
	}
	for name, o := range fate {
		for i, d := range o.Links {
			if d.Action != domain.ActionDeleteAsset && objectFile(d.Link) {
				o.Links[i] = redo(name, d, domain.ChangeDeleted)
			}
		}
	}
	return fate
}
