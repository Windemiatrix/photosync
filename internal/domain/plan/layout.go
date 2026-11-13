package plan

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DecideLayout returns what a run makes of an asset in iCloud and in each
// remote: decisions, by remote, those of objects once DecideFate decides the
// fate of the asset and StatusDeletes applies p.Deletes; status, by the link
// ID of each decision, the status that it leaves its link in, "" for none;
// target, join and leave as DecideTarget gives them; and placements, by
// remote, those of DecidePlacement that the mode of the remote runs, a remote
// without any being absent. now and found are as DecideRemoteLinks takes
// them, links are its current for every remote with the links that matching
// makes in the run, and linked reports that the asset has a link in some
// remote and status, in the state or made by matching. objects are, by
// remote, what DecideRemoteObject gives with in true for each remote where
// the asset takes part in the plan, a remote with none of its files too, with
// the albums that matching makes the asset join added to Join; the decisions
// of matching are not among them. remotes are those of the configuration in
// its order, which is the priority of remotes while p.RemotePriority is
// empty. albums are as DecidePlacement takes them; a Join or a Leave of an
// album that they lack or hold in Recently Deleted is dropped.
//
// An asset that is gone, that IsNew does not hold for, or that a decision
// still deletes with ActionDeleteAsset is not laid out: target is now with
// its albums in the order of AlbumID, and there are no join, leave or
// placements. Otherwise an asset without a link gets the first placement
// that DecideFirstPlacement gives under p.MultiAlbum when a remote of
// objects is not upload-only, a remote for which StatusFrozen holds on the
// statuses after the run gets no placements, and the uploads of a remote
// with a decision by R-LINK-10, which holds no file of the asset, take that
// rule. The inputs are not changed.
func DecideLayout(now domain.Asset, found, linked bool, objects map[string]domain.ObjectDecision, links []domain.Link, remotes []domain.RemoteSpec, albums []domain.Album, p domain.Policies, runID string) (decisions map[string][]domain.LinkDecision, status map[domain.LinkID]domain.LinkStatus, target domain.Asset, join, leave []domain.AlbumChange, placements map[string][]domain.Placement) {
	modes, names := map[string]domain.Mode{}, []string(nil)
	for _, r := range remotes {
		modes[r.Name], names = r.Mode, append(names, r.Name)
	}
	priority := p.RemotePriority
	if len(priority) == 0 {
		priority = names
	}
	dead := func(c domain.AlbumChange) bool {
		return !slices.ContainsFunc(albums, func(a domain.Album) bool { return a.ID == c.AlbumID && !a.InTrash })
	}
	fate, deleting, frozen := DecideFate(objects, modes, priority, p.Deletes), false, map[string]bool{}
	decisions, status = map[string][]domain.LinkDecision{}, map[domain.LinkID]domain.LinkStatus{}
	for name, o := range fate {
		var after []domain.Link
		for i, d := range o.Links {
			l := d.Link
			o.Links[i], l.Status = StatusDeletes(d, p.Deletes)
			status[l.ID], after = l.Status, append(after, l)
			deleting = deleting || o.Links[i].Action == domain.ActionDeleteAsset
		}
		o.Join, o.Leave = slices.DeleteFunc(slices.Clone(o.Join), dead), slices.DeleteFunc(slices.Clone(o.Leave), dead)
		fate[name], decisions[name], frozen[name] = o, o.Links, StatusFrozen(after, name)
	}
	if !found || !IsNew(now, p) || deleting {
		target, _, _ = DecideTarget(now, domain.ObjectDecision{}, nil, nil, nil, runID)
		return decisions, status, target, nil, nil, nil
	}
	var first domain.ObjectDecision
	if !linked && slices.ContainsFunc(remotes, func(r domain.RemoteSpec) bool {
		_, in := objects[r.Name]
		return in && r.Mode != domain.ModeUploadOnly
	}) {
		first.Join, first.Leave = DecideFirstPlacement(now, albums, p.MultiAlbum)
	}
	target, join, leave = DecideTarget(now, first, fate, links, priority, runID)
	placements = map[string][]domain.Placement{}
	for name, o := range fate {
		again := slices.ContainsFunc(o.Links, func(d domain.LinkDecision) bool { return d.Rule == "R-LINK-10" })
		for _, pl := range DecidePlacement(target, links, name, albums) {
			if again {
				pl.Rule = "R-LINK-10"
			}
			if !frozen[name] && ModeAllows(modes[name], pl.Action) {
				placements[name] = append(placements[name], pl)
			}
		}
	}
	return decisions, status, target, join, leave, placements
}
