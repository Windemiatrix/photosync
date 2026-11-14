package plan

import (
	"cmp"
	"path"
	"slices"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Input is what a run plans from. State is what the last applied run left,
// Snapshot the photo library as the run reads it, and Scans hold by its name
// the whole listing of each remote of Remotes. Remotes are those of the
// configuration in its order, the priority of remotes while
// Policies.RemotePriority is empty. OnlyRemote, when not empty, names the
// only remote that the run syncs, and the scans of the others are not read;
// a name that Remotes lack leaves no remote to sync. Template names the
// files that the run uploads, and Fields gives the fields of an asset by its
// key, an empty template.Photo for a key that it does not know; neither is
// nil. Now, never zero, is when the run plans, and RunID is the ID of the
// run.
type Input struct {
	State      domain.State
	Snapshot   domain.LibrarySnapshot
	Scans      map[string][]domain.RemoteEntry
	Remotes    []domain.RemoteSpec
	OnlyRemote string
	Policies   domain.Policies
	Template   *template.Template
	Fields     func(domain.AssetKey) template.Photo
	Now        time.Time
	RunID      string
}

// Build returns the plan of a run and the delta that Reconcile gives, which
// the run writes to the state before the operations; the error, with a zero
// plan and delta, is that of Reconcile or of FileOps. The run syncs the
// remotes of Remotes, or the one that OnlyRemote names: ReadScans reads their
// scans, and RunOps gives the operations that come from them before any asset
// takes a name. Then each asset of the delta, and each asset that the delta
// lacks and a link of the state names, found false, is decided on in the
// order of ID. In a remote that the run does not sync, or whose scan holds
// the asset as Settling, its links stay as the state has them; elsewhere
// DecideRemoteLinks decides on them, and, unless that freezes the asset
// there, DecideRemoteObject with the conflict that the state keeps for the
// asset there, else one with only AssetID and Remote. The conflict that it
// returns goes to Conflicts unless its Status is empty. Where it takes the
// asset into the plan, the albums that matching makes the asset join there
// are added to Join, and the new links that matching makes there are added
// to its links as they lie now: so DecideLayout takes them, the asset
// counting as linked when the state or those new links name it, and
// DecideComponents its links as the state holds them. AssetOps, then
// FileOps, ComponentOps and SidecarOps in each remote that the run syncs, by
// name, where the asset takes part in the plan, give its operations: FileOps
// with the decisions of matching there, each replacement turned into
// ActionLink when a decision of DecideLayout deletes the asset, and with the
// pairs of the state and those that the run made before. A remote takes its
// names from its whole listing and its links in the state, and its sidecars
// by those links. ExpireOps, with Policies.QuarantineKeepDays, gives the
// operations on the quarantine of each remote that the run syncs, from its
// whole listing.
//
// The delta gives each asset found the TargetLibrary of its target. A link of
// a decision of DecideLayout or DecideComponents with ActionMoveAsset,
// ActionUpdateBase or ActionLink lies where DecideRemoteLinks has it now,
// with the size, hashes, ETag and MTime of its file, and one with ActionNone,
// ActionForget or ActionUpload is as in the state; each takes the status
// that the run leaves it in, except that the link of an upload that
// DecideLayout decides leaves the state, the upload making a new one. Each
// such link that differs from the state, and each new link of matching with
// ActionLink where the asset takes part in the plan, goes, unless an
// operation of the asset holds it, with the OpAlbumAdd or OpAlbumRemove of
// the asset on its album, else on the album that its link leaves, else to
// Links; a decision with another action leaves its link to the operations.
// Once every asset is decided on, an operation of a remote that the run
// syncs, other than an import, whose source, or whose destination or a
// directory above it, is among the paths that Names.Doubles finds there
// leaves the plan, and with it every operation there on its asset, if it has
// one, and the links and the conflict of that asset there, but not its
// operations in iCloud; a link alone holds nothing back. Ops are in the order
// of domain.Op.Compare; Albums are what ReadScans adds, Pairs what ReadScans
// and then FileOps make, and Notices hold those of Reconcile, ReadScans, the
// operations and Names.Doubles in the order of Code, then Params. The inputs
// are not changed.
func Build(in Input) (domain.Plan, domain.SnapshotDelta, error) {
	delta, notices, err := Reconcile(in.State, in.Snapshot, in.RunID)
	if err != nil {
		return domain.Plan{}, domain.SnapshotDelta{}, err
	}
	aside := func(r domain.RemoteSpec) bool { return in.OnlyRemote != "" && r.Name != in.OnlyRemote }
	remotes := slices.DeleteFunc(slices.Clone(in.Remotes), aside)
	files, read, added, made, ns := ReadScans(in.State, delta, in.Scans, remotes, in.Policies, in.Now)
	albums, pairs := slices.Concat(delta.Albums, added), slices.Concat(in.State.AlbumPairs, made)
	notices = append(notices, ns...)
	names, sides := map[string]*Names{}, map[string]map[domain.LinkID][]domain.RemoteEntry{}
	fresh := map[domain.AssetID][]domain.LinkDecision{}
	for _, r := range remotes {
		names[r.Name], sides[r.Name] = NewNames(r.Name, in.Scans[r.Name], in.State.Links), Sidecars(in.State.Links, r.Name, in.Scans[r.Name])
		for _, d := range read[r.Name].Linked {
			fresh[d.Link.AssetID] = append(fresh[d.Link.AssetID], d)
		}
	}
	out := domain.Plan{Ops: RunOps(read, added, albums, names, remotes, in.Policies, in.Now), Albums: added, Pairs: made}
	out.Ops = append(out.Ops, ExpireOps(in.Scans, remotes, in.Policies.QuarantineKeepDays, in.Now)...)
	assets, base, owned := map[domain.AssetID]domain.Asset{}, map[domain.AssetID]domain.Asset{}, map[domain.AssetID][]domain.Link{}
	stored, index := map[domain.Conflict]domain.Conflict{}, map[domain.AssetID]int{}
	var ids []domain.AssetID
	var kept []domain.Link
	left := map[domain.LinkID]domain.AlbumID{}
	var conflicts []domain.Conflict
	for i, a := range delta.Assets {
		assets[a.ID], index[a.ID], ids = a, i, append(ids, a.ID)
	}
	for _, a := range in.State.Assets {
		base[a.ID] = a
	}
	for _, l := range in.State.Links {
		owned[l.AssetID], ids = append(owned[l.AssetID], l), append(ids, l.AssetID)
	}
	for _, c := range in.State.Conflicts {
		stored[domain.Conflict{AssetID: c.AssetID, Remote: c.Remote}] = c
	}
	byName := slices.SortedFunc(slices.Values(remotes), func(x, y domain.RemoteSpec) int { return cmp.Compare(x.Name, y.Name) })
	slices.Sort(ids)
	for _, id := range slices.Compact(ids) {
		asset, found := assets[id]
		linked, frozen, decided := len(owned[id]) > 0, map[string]bool{}, map[string][]domain.LinkDecision{}
		var current []domain.Link
		for _, r := range in.Remotes {
			if frozen[r.Name] = aside(r) || read[r.Name].Settling[id]; frozen[r.Name] {
				current = append(current, slices.DeleteFunc(slices.Clone(owned[id]), func(l domain.Link) bool { return l.Remote != r.Name })...)
				continue
			}
			var cur []domain.Link
			decided[r.Name], cur, frozen[r.Name] = DecideRemoteLinks(asset, found, base[id].Library, owned[id], r, files, pairs, albums, in.Policies)
			current = append(current, cur...)
		}
		objects, matched := map[string]domain.ObjectDecision{}, map[string][]domain.LinkDecision{}
		for _, r := range byName {
			if frozen[r.Name] {
				continue
			}
			c := domain.Conflict{AssetID: id, Remote: r.Name}
			o, conflict, ok := DecideRemoteObject(asset, found, decided[r.Name], current, r, albums, cmp.Or(stored[c], c), in.Policies)
			if conflict.Status != "" {
				conflicts = append(conflicts, conflict)
			}
			if ok {
				o.Join = objectChanges(append(o.Join, read[r.Name].Join[id]...))
				objects[r.Name] = o
			}
		}
		for _, d := range fresh[id] {
			if _, ok := objects[d.Link.Remote]; ok {
				matched[d.Link.Remote], current, linked = append(matched[d.Link.Remote], d), append(current, d.Link), true
			}
		}
		decisions, status, target, join, leave, placements := DecideLayout(asset, found, linked, objects, current, in.Remotes, albums, in.Policies, in.RunID)
		fates, components, copies, after := DecideComponents(decisions, status, placements, owned[id], files, in.Remotes, in.Policies.Deletes)
		start := len(out.Ops)
		out.Ops = append(out.Ops, AssetOps(id, decisions, status, join, leave, components, files)...)
		for _, ds := range decisions {
			if slices.ContainsFunc(ds, func(d domain.LinkDecision) bool { return d.Action == domain.ActionDeleteAsset }) {
				for _, ms := range matched {
					for i := range ms {
						ms[i].Action = domain.ActionLink
					}
				}
			}
		}
		fields := in.Fields(target.Key)
		for _, r := range byName {
			main, mk, more, err := FileOps(names[r.Name], r, target, slices.Concat(decisions[r.Name], matched[r.Name]), placements[r.Name], files, albums, pairs, in.Template, fields, in.Policies, in.Now)
			if err != nil {
				return domain.Plan{}, domain.SnapshotDelta{}, err
			}
			comp, most := ComponentOps(names[r.Name], target, main, components, copies, after, files, in.Now)
			side, last := SidecarOps(names[r.Name], id, main, fates, sides[r.Name], files, r.Mode, in.Policies.Deletes, in.Now)
			out.Ops, notices = append(out.Ops, slices.Concat(main, comp, side)...), append(notices, slices.Concat(more, most, last)...)
			pairs, out.Pairs = append(pairs, mk...), append(out.Pairs, mk...)
		}
		if found {
			delta.Assets[index[id]].TargetLibrary = target.TargetLibrary
		}
		carried, cur := map[domain.LinkID]bool{}, map[domain.LinkID]domain.Link{}
		for _, o := range out.Ops[start:] {
			for _, l := range o.Links {
				carried[l.ID] = true
			}
		}
		for _, l := range current {
			cur[l.ID] = l
		}
		keep := func(l domain.Link, changed bool) {
			if changed && !carried[l.ID] {
				kept = append(kept, l)
			}
		}
		record := func(d domain.LinkDecision, end domain.LinkStatus) {
			l := d.Link
			switch d.Action {
			case domain.ActionMoveAsset, domain.ActionUpdateBase, domain.ActionLink:
				e := files[l.ID]
				l = cur[l.ID]
				l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
			case domain.ActionNone, domain.ActionForget, domain.ActionUpload:
			default:
				return
			}
			l.Status = end
			left[l.ID] = d.Link.AlbumID
			keep(l, l != d.Link)
		}
		for _, ds := range decisions {
			for _, d := range ds {
				st := status[d.Link.ID]
				if d.Action == domain.ActionUpload {
					st = ""
				}
				record(d, st)
			}
		}
		for _, ds := range components {
			for _, d := range ds {
				record(d, after[d.Link.ID])
			}
		}
		for _, ds := range matched {
			for _, d := range ds {
				keep(d.Link, d.Action == domain.ActionLink)
			}
		}
	}
	excluded := map[string]map[domain.AssetID]bool{}
	for _, r := range remotes {
		doubled, ns := names[r.Name].Doubles()
		hit := func(o domain.Op) bool {
			if o.Remote != r.Name || o.Kind == domain.OpImport {
				return false
			}
			for p := o.Dst; p != path.Dir(p); p = path.Dir(p) {
				if doubled[domain.RemoteEntry{Root: o.DstRoot, Path: p}] {
					return true
				}
			}
			return doubled[domain.RemoteEntry{Root: o.Root, Path: o.Src}]
		}
		held := map[domain.AssetID]bool{}
		excluded[r.Name] = held
		for _, o := range out.Ops {
			if hit(o) && o.Asset != 0 {
				held[o.Asset] = true
			}
		}
		out.Ops = slices.DeleteFunc(out.Ops, func(o domain.Op) bool { return hit(o) || o.Remote == r.Name && held[o.Asset] })
		notices = append(notices, ns...)
	}
	albumOps := map[domain.Link]int{}
	for i, o := range out.Ops {
		if o.Kind == domain.OpAlbumAdd || o.Kind == domain.OpAlbumRemove {
			albumOps[domain.Link{AssetID: o.Asset, AlbumID: o.AlbumID}] = i
		}
	}
	slices.SortFunc(kept, func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) })
	for _, l := range kept {
		i, joins := albumOps[domain.Link{AssetID: l.AssetID, AlbumID: l.AlbumID}]
		if !joins {
			i, joins = albumOps[domain.Link{AssetID: l.AssetID, AlbumID: left[l.ID]}]
		}
		switch {
		case excluded[l.Remote][l.AssetID]:
		case joins:
			out.Ops[i].Links = append(out.Ops[i].Links, l)
		default:
			out.Links = append(out.Links, l)
		}
	}
	for _, c := range conflicts {
		if !excluded[c.Remote][c.AssetID] {
			out.Conflicts = append(out.Conflicts, c)
		}
	}
	slices.SortFunc(out.Ops, domain.Op.Compare)
	slices.SortFunc(notices, func(x, y domain.Notice) int {
		return cmp.Or(cmp.Compare(x.Code, y.Code), slices.Compare(x.Params, y.Params))
	})
	out.Notices = notices
	return out, delta, nil
}
