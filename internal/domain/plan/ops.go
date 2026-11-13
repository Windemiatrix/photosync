package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// AssetOps returns the operations of a run on asset id that need no new path
// in the cloud: the deletions of its files and of the asset, and the albums
// that it joins and leaves. decisions, status, join and leave are as
// DecideLayout gives them, components as DecideComponents gives them, and
// files gives by link ID the entry of the scan where the file of a link lies
// now, as ReadScan gives it. Each decision and each component with
// ActionDeleteFile gives an OpDelete of its file there, and the decisions
// with ActionDeleteAsset give one OpAssetDelete. An operation takes the rule
// of its decisions, which StatusDeletes gives all alike, and holds their
// links, each with the status that status gives it; a component, which status
// does not list, loses its link. An OpDelete is irreversible under any rule,
// and an OpAssetDelete only under R-STATUS-06. join and leave give
// OpAlbumAdd and OpAlbumRemove with their rules, and other decisions give
// nothing here. The operations are in the order of domain.Op.Compare, and
// the inputs are not changed.
func AssetOps(id domain.AssetID, decisions map[string][]domain.LinkDecision, status map[domain.LinkID]domain.LinkStatus, join, leave []domain.AlbumChange, components map[domain.LinkID][]domain.LinkDecision, files map[domain.LinkID]domain.RemoteEntry) []domain.Op {
	var ops []domain.Op
	gone := domain.Op{Kind: domain.OpAssetDelete, Asset: id}
	end := func(d domain.LinkDecision) {
		l := d.Link
		l.Status = status[l.ID]
		switch d.Action {
		case domain.ActionDeleteFile:
			e := files[l.ID]
			ops = append(ops, domain.Op{
				Kind: domain.OpDelete, Remote: l.Remote, Root: e.Root, Src: e.Path, Asset: id,
				Rule: d.Rule, Irreversible: true, Links: []domain.Link{l},
			})
		case domain.ActionDeleteAsset:
			gone.Rule, gone.Irreversible, gone.Links = d.Rule, d.Rule == "R-STATUS-06", append(gone.Links, l)
		}
	}
	for _, ds := range decisions {
		for _, d := range ds {
			end(d)
		}
	}
	for _, ds := range components {
		for _, d := range ds {
			end(d)
		}
	}
	if gone.Links != nil {
		slices.SortFunc(gone.Links, func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) })
		ops = append(ops, gone)
	}
	for _, c := range join {
		ops = append(ops, domain.Op{Kind: domain.OpAlbumAdd, Asset: id, AlbumID: c.AlbumID, Rule: c.Rule})
	}
	for _, c := range leave {
		ops = append(ops, domain.Op{Kind: domain.OpAlbumRemove, Asset: id, AlbumID: c.AlbumID, Rule: c.Rule})
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops
}
