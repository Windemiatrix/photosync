package plan

import (
	"maps"
	"slices"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// ComponentOps returns the operations of a run on the Live Photo videos and
// RAW files of an asset in the remote of names, which follow the files of the
// original or the source role that they lie beside, and notices of those that
// find their path taken. target is the target of the asset as DecideLayout
// gives it, mains are the operations that FileOps gives for the asset in that
// remote, components, copies and after are as DecideComponents gives them,
// files gives by link ID the entry of the scan where the file of a link lies
// now, as ReadScan gives it, names are the names of the remote in the run,
// which the operations take, and now is the day of quarantine. Only the
// components of that remote count, by increasing link ID of their main file.
//
// A component with ActionQuarantineFile moves from where it lies to
// quarantine (OpQuarantine), its link taking that place, the path in
// quarantine and the status that after gives it. One with ActionMoveFile or
// ActionMoveFileReplaceAsset moves from where it lies (OpMove), and one with
// ActionUpload puts the resource of target with its role (OpPut), to
// ComponentPath from where its main file ends up, its link taking that place:
// the link of that file as an operation of mains leaves it, with its album,
// else where that file lies, with the album of the component. Each placement
// in copies copies the component from where it lies (OpCopy), or puts its
// resource for ActionUpload, to ComponentPath from the new link of the OpCopy
// of mains into the album of the placement, as a new link in that album like
// the component. Each OpPut of mains with a new link puts the Live Photo
// video and the RAW file of target to ComponentPath from that link, as new
// links in its album under its rule. A put makes a link at the level uploaded
// with the size and hashes of the resource, and nothing while target lacks
// the resource. A destination at ComponentPath that names already hold gives
// the notice "plan.path_taken" with the remote, the root and the path instead
// of an operation, and names take every other. Other decisions give nothing
// here. The operations are in the order of domain.Op.Compare, the notices in
// the order of their parameters, and the inputs but names are not changed.
func ComponentOps(names *Names, target domain.Asset, mains []domain.Op, components map[domain.LinkID][]domain.LinkDecision, copies map[domain.LinkID][]domain.Placement, after map[domain.LinkID]domain.LinkStatus, files map[domain.LinkID]domain.RemoteEntry, now time.Time) (ops []domain.Op, notices []domain.Notice) {
	add := func(kind domain.OpKind, from, to domain.Link, dst, rule string) {
		ops = append(ops, domain.Op{
			Kind: kind, Remote: names.remote, Root: from.Root, Src: from.Path, DstRoot: to.Root, Dst: dst,
			Asset: to.AssetID, Rule: rule, Links: []domain.Link{to},
		})
	}
	beside := func(kind domain.OpKind, from, to, main domain.Link, rule string) {
		to.Root, to.Path, to.AlbumID = main.Root, ComponentPath(main.Path, to.Role, target), main.AlbumID
		// Free gives a main file that the run moves a stem that no file holds,
		// but beside one that stays a file of the user may hold the name.
		if names.paths[to.Root][to.Path] {
			notices = append(notices, domain.Notice{Code: "plan.path_taken", Params: []string{names.remote, string(to.Root), to.Path}})
			return
		}
		names.take(to.Root, to.Path)
		add(kind, from, to, to.Path, rule)
	}
	put := func(id domain.LinkID, role domain.LinkRole, main domain.Link, rule string) {
		for _, res := range target.Resources {
			// The roles of a link and of its resource store the same strings.
			if res.Role == domain.ResourceRole(role) {
				beside(domain.OpPut, domain.Link{}, domain.Link{
					ID: id, AssetID: target.ID, Remote: names.remote, Role: role,
					Size: res.Size, SHA256: res.SHA256, MD5: res.MD5, Level: domain.LevelUploaded, Status: domain.StatusActive,
				}, main, rule)
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(components)) {
		for _, d := range components[id] {
			c := d.Link
			c.Root, c.Path = files[c.ID].Root, files[c.ID].Path
			end := c
			end.Root, end.Path = files[id].Root, files[id].Path
			for _, o := range mains {
				for _, l := range o.Links {
					if l.ID == id {
						end = l
					}
				}
			}
			switch {
			case c.Remote != names.remote:
			case d.Action == domain.ActionQuarantineFile:
				to := c
				to.Status, to.QuarantinePath = after[c.ID], names.Quarantine(c.Root, now, c.Path)
				add(domain.OpQuarantine, c, to, to.QuarantinePath, d.Rule)
			case d.Action == domain.ActionMoveFile || d.Action == domain.ActionMoveFileReplaceAsset:
				beside(domain.OpMove, c, c, end, d.Rule)
			case d.Action == domain.ActionUpload:
				put(c.ID, c.Role, end, d.Rule)
			}
		}
		for _, pl := range copies[id] {
			for _, o := range mains {
				for _, l := range o.Links {
					switch to := pl.Link; {
					case pl.Link.Remote != names.remote || o.Kind != domain.OpCopy || l.AlbumID != pl.AlbumID:
					case pl.Action == domain.ActionUpload:
						put(0, to.Role, l, pl.Rule)
					default:
						to.ID = 0
						beside(domain.OpCopy, pl.Link, to, l, pl.Rule)
					}
				}
			}
		}
	}
	for _, o := range mains {
		for _, l := range o.Links {
			if o.Kind == domain.OpPut && l.ID == 0 {
				put(0, domain.RoleLiveVideo, l, o.Rule)
				put(0, domain.RoleRAWPair, l, o.Rule)
			}
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	slices.SortFunc(notices, func(x, y domain.Notice) int { return slices.Compare(x.Params, y.Params) })
	return ops, notices
}
