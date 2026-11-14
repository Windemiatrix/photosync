package plan

import (
	"cmp"
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
// name, where the asset takes part in the plan, give its operations: FileOps with the decisions of matching there
// unless a decision of DecideLayout deletes the asset, and with the pairs of
// the state and those that the run made before. A remote takes its names from
// its whole listing and its links in the state, and its sidecars by those
// links. Ops are in the order of domain.Op.Compare; Albums are what ReadScans
// adds, Pairs what ReadScans and then FileOps make, and Notices hold those of
// Reconcile, ReadScans and the operations in the order of Code, then Params.
// The inputs are not changed.
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
	assets, base, owned := map[domain.AssetID]domain.Asset{}, map[domain.AssetID]domain.Asset{}, map[domain.AssetID][]domain.Link{}
	stored := map[domain.Conflict]domain.Conflict{}
	var ids []domain.AssetID
	for _, a := range delta.Assets {
		assets[a.ID], ids = a, append(ids, a.ID)
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
				out.Conflicts = append(out.Conflicts, conflict)
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
		out.Ops = append(out.Ops, AssetOps(id, decisions, status, join, leave, components, files)...)
		for _, ds := range decisions {
			if slices.ContainsFunc(ds, func(d domain.LinkDecision) bool { return d.Action == domain.ActionDeleteAsset }) {
				clear(matched)
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
	}
	slices.SortFunc(out.Ops, domain.Op.Compare)
	slices.SortFunc(notices, func(x, y domain.Notice) int {
		return cmp.Or(cmp.Compare(x.Code, y.Code), slices.Compare(x.Params, y.Params))
	})
	out.Notices = notices
	return out, delta, nil
}
