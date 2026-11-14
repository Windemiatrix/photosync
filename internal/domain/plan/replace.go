package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// ReplaceOps returns the operations that replace asset target in iCloud with
// an asset imported from its files in the cloud, and the notice of a
// replacement held back. target, decisions and components are as
// DecideLayout and DecideComponents give them and links as DecideLayout
// takes them; ops are the operations that the run gives the asset so far,
// files gives by link ID the entry of the scan where the file of a link lies
// now, as ReadScan gives it, and albums are the albums of the run. A link
// lies where the last operation of ops that holds it leaves it, else where
// its file lies. The file to import is that of a decision with
// ActionReplaceAsset or ActionMoveFileReplaceAsset, or the main file under
// which components hold a decision with one; of several, the one with the
// lowest link ID, under the rule of its own decision when that replaces,
// else of the component. Nothing is replaced while a decision deletes the
// asset or imports a file of it as a new asset. A hidden target, which a
// sync cannot find in Photos, and a file that does not stay active, as a
// needless copy that goes to quarantine, give instead the notice
// "plan.hidden_not_replaced" or "plan.quarantined_not_replaced" with the
// name and UUID of target. Otherwise an OpImport with target.ID as its Asset
// reads the file where it lies; its links are that of the file, then each
// Live Photo video and RAW file that ComponentBeside finds beside it, in the
// order of ID, each where it lies, at the level imported and with the size,
// hashes, ETag and MTime of its entry. An OpAlbumAdd under the same rule puts
// the asset into each album of target.Memberships that albums hold outside
// Recently Deleted and that no operation of ops names, and an OpAssetDelete
// by R-STATUS-08 takes the old asset to Recently Deleted. The operations are
// in the order of domain.Op.Compare, and the inputs are not changed.
func ReplaceOps(target domain.Asset, decisions map[string][]domain.LinkDecision, components map[domain.LinkID][]domain.LinkDecision, links []domain.Link, ops []domain.Op, files map[domain.LinkID]domain.RemoteEntry, albums []domain.Album) ([]domain.Op, []domain.Notice) {
	main, rule := domain.LinkID(0), ""
	pick := func(id domain.LinkID, d domain.LinkDecision) {
		if (d.Action == domain.ActionReplaceAsset || d.Action == domain.ActionMoveFileReplaceAsset) && (main == 0 || id < main) {
			main, rule = id, d.Rule
		}
	}
	for _, ds := range decisions {
		for _, d := range ds {
			if d.Action == domain.ActionDeleteAsset || d.Action == domain.ActionImport {
				return nil, nil
			}
			pick(d.Link.ID, d)
		}
	}
	for id, ds := range components {
		for _, d := range ds {
			pick(id, d)
		}
	}
	links = slices.SortedFunc(slices.Values(links), func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) })
	i := slices.IndexFunc(links, func(l domain.Link) bool { return l.ID == main })
	if i < 0 {
		return nil, nil
	}
	ends := func(l domain.Link) domain.Link { return mainEnd(l, l.ID, ops, files) }
	end := ends(links[i])
	switch {
	case target.Hidden:
		return nil, []domain.Notice{{Code: "plan.hidden_not_replaced", Params: []string{target.OriginalName, target.Key.UUID}}}
	case end.Status != domain.StatusActive:
		return nil, []domain.Notice{{Code: "plan.quarantined_not_replaced", Params: []string{target.OriginalName, target.Key.UUID}}}
	}
	imported := func(l domain.Link) domain.Link {
		e := files[l.ID]
		l = ends(l)
		l.Level, l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = domain.LevelImported, e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
		return l
	}
	o := domain.Op{
		Kind: domain.OpImport, Remote: end.Remote, Root: end.Root, Src: end.Path, Asset: target.ID, Rule: rule,
		Links: []domain.Link{imported(links[i])},
	}
	for _, l := range links {
		if ComponentBeside(end, ends(l)) {
			o.Links = append(o.Links, imported(l))
		}
	}
	out := []domain.Op{o, {Kind: domain.OpAssetDelete, Asset: target.ID, Rule: "R-STATUS-08"}}
	for _, m := range target.Memberships {
		named := slices.ContainsFunc(ops, func(x domain.Op) bool { return x.AlbumID == m.AlbumID })
		if !named && slices.ContainsFunc(albums, func(a domain.Album) bool { return a.ID == m.AlbumID && !a.InTrash }) {
			out = append(out, domain.Op{Kind: domain.OpAlbumAdd, Asset: target.ID, AlbumID: m.AlbumID, Rule: rule})
		}
	}
	slices.SortFunc(out, domain.Op.Compare)
	return out, nil
}
