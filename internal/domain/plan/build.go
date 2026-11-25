package plan

import (
	"cmp"
	"path"
	"slices"
	"strconv"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
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
// the run writes to the state before the operations, a new asset only with
// its first link; the error, with a zero
// plan and delta, is that of eventtpl.Validate for Policies.Templates,
// checked before all else, of Reconcile or of FileOps. The run syncs the
// remotes of Remotes, or the one that OnlyRemote names: ReadScans reads their
// scans, and RunOps gives the operations that come from them before any asset
// takes a name. The albums of the run are those of the delta, each as the
// Refiled, then the Renamed of ReadScans gives it, the remotes taken by name,
// or, inside a folder of Renamed, under the new title of that folder, and
// those that ReadScans adds; its pairs are those of the state for an album,
// remote and root for which ReadScans makes none, then those that ReadScans
// makes. MovePairs then moves the pairs of each remote, by name, in the
// personal root, then the shared one, with the albums and pairs of the run
// and its whole scan; a moved pair takes the place of its old one among the
// pairs of the run and in Pairs, and DecideLayout takes the old ones with the
// Moved of ReadScans.
// Then each asset of the delta, and each asset that the delta lacks and a
// link of the state names, found false, is decided on in the order of ID. In
// a remote that the run does not sync, or whose scan holds the asset as
// Settling, its links stay as the state has them; elsewhere DecideRemoteLinks
// decides on them, and, unless that freezes the asset there,
// DecideRemoteObject with the conflict that the state keeps for the asset
// there, else one with only AssetID and Remote. The conflict that it returns
// goes to Conflicts unless its Status is empty. Where it takes the asset into
// the plan, the albums that matching makes the asset join there are added to
// Join, and the new links that matching makes there are added to its links as
// they lie now: so DecideLayout takes them, the asset counting as linked when
// the state holds the asset or a link of it, or when those new links name
// it, and DecideComponents its links as the state holds them. A link of
// matching that a placement takes to quarantine is not made where it takes
// the asset into no album, in an upload-only remote or under the personal
// root while p.AlbumsForPersonal is false, nor are the links of its Live
// Photo video and RAW file; that placement is dropped, and the file counts
// in "plan.not_imported". AssetOps, ImportOps with its links as they lie now, then
// FileOps, ComponentOps and SidecarOps in each remote that the run syncs, by
// name, where the asset takes part in the plan, give its operations: FileOps
// with the decisions of matching there, each replacement turned into
// ActionLink when a decision of DecideLayout deletes the asset, and with the
// pairs of that remote in the root of the target of the asset, those of the
// state and those that the run made before. A remote takes its names from
// its whole listing, its links in the state and the path of each of those
// links whose asset its scan holds as Settling, and its sidecars by those
// links. ReplaceOps then gives the replacement of the asset from its
// links as they lie now and its operations so far. ExpireOps, with
// Policies.QuarantineKeepDays, gives the operations on the quarantine of each
// remote that the run syncs, from its whole listing.
//
// The delta gives each asset found the TargetLibrary of its target. A link of
// a decision of DecideLayout or DecideComponents with ActionMoveAsset,
// ActionUpdateBase or ActionLink lies where DecideRemoteLinks has it now,
// with the size, hashes, ETag and MTime of its file, and one with ActionNone,
// ActionForget or ActionUpload is as in the state, except that a detached
// link takes the root and path where its file lies now; each takes the status
// that the run leaves it in, except that the link of an upload that
// DecideLayout decides leaves the state, the upload making a new one. Each
// such link that differs from the state, and each new link of matching with
// ActionLink where the asset takes part in the plan, goes, unless an
// operation of the asset holds it, with the OpAlbumAdd or OpAlbumRemove of
// the asset on its album, else on the album that its link leaves, else to
// Links; a decision with another action leaves its link to the operations.
// Once every asset is decided on, an asset with an earlier identity that the
// History of the delta holds with Deleted false gets an OpAssetDelete by
// R-STATUS-08 unless an OpAssetDelete of the plan names it already; then an
// import of RunOps, whose links are new, leaves the plan when ImportOps or
// ReplaceOps imports a file of the same content by the hash of kind
// Capabilities.ContentHash of the remote of each, an empty hash matching
// none; then an operation of a remote that the run syncs, other
// than an import, whose source, or whose destination or a directory above
// it, is among the paths that Names.Doubles finds there leaves the plan, and
// with it every operation there on its asset, if it has one, and the links
// and the conflict of that asset there, but not its operations in iCloud; a
// link alone holds nothing back. Ops are in the order of domain.Op.Compare;
// Albums are what ReadScans adds, Pairs what ReadScans, MovePairs and then
// FileOps make, each with NamedAfter the path of its album when its DirPath
// is the directory that layout.DirFor gives the album, else empty, and
// Notices hold those of Reconcile, ReadScans, the operations, Names.Doubles
// and Build itself in the order of Code, then Params. Build adds
// "plan.conflict_settled", with the remote, the original name and the UUID
// of the asset and the conflict policy, wherever that policy settles a
// conflict that the state does not keep for an asset in a remote. It adds
// "plan.skipped", with "asset", the original name and the UUID, for each
// asset that IsNew holds for, with no link in the state and none that
// matching makes, that the run leaves in no event album. For each
// remote that the run syncs it adds, with the remote and a decimal count
// above zero, "plan.not_imported" for the new files that ReadScans does not
// decide to import, "plan.waiting_import" for those that it does but that
// wait for a later run, as the plan imports a file of the same content
// instead, "plan.edits_left" for the edits of ReadScans that no operation
// moves to quarantine, and "plan.albums_kept" for the decisions by
// R-MODE-04, each a move of a file that the base takes while the albums
// stay as they are. A content counts once among the new files of a remote.
// The inputs are not changed.
func Build(in Input) (domain.Plan, domain.SnapshotDelta, error) {
	if err := eventtpl.Validate(in.Policies.Templates); err != nil {
		return domain.Plan{}, domain.SnapshotDelta{}, err
	}
	delta, notices, err := Reconcile(in.State, in.Snapshot, in.RunID, in.Policies)
	if err != nil {
		return domain.Plan{}, domain.SnapshotDelta{}, err
	}
	aside := func(r domain.RemoteSpec) bool { return in.OnlyRemote != "" && r.Name != in.OnlyRemote }
	remotes := slices.DeleteFunc(slices.Clone(in.Remotes), aside)
	files, read, added, made, ns := ReadScans(in.State, delta, in.Scans, remotes, in.Policies, in.Now)
	albums, pairs := slices.Concat(delta.Albums, added), slices.DeleteFunc(slices.Clone(in.State.AlbumPairs), func(x domain.AlbumPair) bool {
		return slices.ContainsFunc(made, func(y domain.AlbumPair) bool {
			return y.AlbumID == x.AlbumID && y.Remote == x.Remote && y.Root == x.Root
		})
	})
	pairs = append(pairs, made...)
	byName := slices.SortedFunc(slices.Values(remotes), func(x, y domain.RemoteSpec) int { return cmp.Compare(x.Name, y.Name) })
	for _, r := range byName {
		for _, a := range slices.Concat(read[r.Name].Refiled, read[r.Name].Renamed) {
			for _, b := range underFolder(albums, a, in.Policies) {
				albums[slices.IndexFunc(albums, func(c domain.Album) bool { return c.ID == b.ID })] = b
			}
		}
	}
	events := layout.ReadEvents(albums, in.Policies)
	notices = append(notices, ns...)
	names, sides := map[string]*Names{}, map[string]map[domain.LinkID][]domain.RemoteEntry{}
	fresh := map[domain.AssetID][]domain.LinkDecision{}
	for _, r := range remotes {
		names[r.Name], sides[r.Name] = NewNames(r.Name, in.Scans[r.Name], in.State.Links), Sidecars(in.State.Links, r.Name, in.Scans[r.Name])
		for _, l := range in.State.Links {
			// A file put at the path of a frozen link would read on the next run
			// as that link's own file, changed.
			if l.Remote == r.Name && read[r.Name].Settling[l.AssetID] {
				names[r.Name].take(l.Root, l.Path)
			}
		}
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
	later := map[[2]string]int{}
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
	var moved []domain.AlbumPair
	for _, r := range byName {
		moved = append(moved, read[r.Name].Moved...)
		for _, root := range []domain.LibraryKind{domain.LibraryPersonal, domain.LibraryShared} {
			for _, x := range MovePairs(albums, pairs, in.Scans[r.Name], r, root, in.Policies) {
				same := func(y domain.AlbumPair) bool {
					return y.AlbumID == x.AlbumID && y.Remote == x.Remote && y.Root == x.Root
				}
				i := slices.IndexFunc(pairs, same)
				moved, pairs[i] = append(moved, pairs[i]), x
				// A pair that ReadScans moved already may move once more: the state
				// takes one pair of an album in a root of a remote.
				if j := slices.IndexFunc(out.Pairs, same); j >= 0 {
					out.Pairs[j] = x
				} else {
					out.Pairs = append(out.Pairs, x)
				}
			}
		}
	}
	rootPairs := map[domain.AlbumPair][]domain.AlbumPair{}
	for _, x := range pairs {
		k := domain.AlbumPair{Remote: x.Remote, Root: x.Root}
		rootPairs[k] = append(rootPairs[k], x)
	}
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
			// Only a conflict takes R-LINK-07, and SettleConflict keeps that rule on
			// the decision that it gives to the winner.
			if conflict.Status != "" {
				conflicts = append(conflicts, conflict)
			} else if slices.ContainsFunc(o.Links, func(d domain.LinkDecision) bool { return d.Rule == "R-LINK-07" }) {
				notices = append(notices, domain.Notice{Code: "plan.conflict_settled", Params: []string{r.Name, asset.OriginalName, asset.Key.UUID, string(in.Policies.Conflict)}})
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
		_, seen := base[id]
		decisions, status, target, join, leave, placements := DecideLayout(asset, found, linked || seen, objects, current, in.Remotes, albums, events, moved, in.Policies, in.RunID)
		for _, r := range in.Remotes {
			// Where a new file of the user takes its asset into no album, it is
			// only reported (R-MODE-01, R-MODE-04), so the layout must not take
			// it to quarantine.
			reported := func(l domain.Link) bool {
				return r.Mode == domain.ModeUploadOnly || l.Root == domain.LibraryPersonal && !in.Policies.AlbumsForPersonal
			}
			var unmade []domain.Link
			for _, pl := range placements[r.Name] {
				if pl.Action == domain.ActionQuarantineFile && reported(pl.Link) && slices.ContainsFunc(matched[r.Name], func(d domain.LinkDecision) bool { return d.Link.ID == pl.Link.ID }) {
					unmade = append(unmade, pl.Link)
				}
			}
			if len(unmade) == 0 {
				continue
			}
			later[[2]string{"plan.not_imported", r.Name}] += len(unmade)
			gone := func(l domain.Link) bool {
				return slices.ContainsFunc(unmade, func(x domain.Link) bool { return x.ID == l.ID || ComponentBeside(x, l) })
			}
			matched[r.Name] = slices.DeleteFunc(matched[r.Name], func(d domain.LinkDecision) bool { return gone(d.Link) })
			placements[r.Name] = slices.DeleteFunc(placements[r.Name], func(pl domain.Placement) bool { return gone(pl.Link) })
		}
		if IsNew(asset, in.Policies) && !linked && len(objectAlbums(target, albums)) == 0 {
			notices = append(notices, domain.Notice{Code: "plan.skipped", Params: []string{"asset", asset.OriginalName, asset.Key.UUID}})
		}
		fates, components, copies, after := DecideComponents(decisions, status, placements, owned[id], files, in.Remotes, in.Policies.Deletes)
		start := len(out.Ops)
		out.Ops = append(out.Ops, AssetOps(id, decisions, status, join, leave, components, files)...)
		out.Ops = append(out.Ops, ImportOps(decisions, current, files, albums, in.Policies.AlbumsForPersonal)...)
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
			k := domain.AlbumPair{Remote: r.Name, Root: target.TargetLibrary}
			main, mk, more, err := FileOps(names[r.Name], r, target, slices.Concat(decisions[r.Name], matched[r.Name]), placements[r.Name], files, albums, rootPairs[k], in.Template, fields, in.Policies, in.Now)
			if err != nil {
				return domain.Plan{}, domain.SnapshotDelta{}, err
			}
			comp, most := ComponentOps(names[r.Name], target, main, components, copies, after, files, in.Now)
			side, last := SidecarOps(names[r.Name], id, main, fates, sides[r.Name], files, r.Mode, in.Policies.Deletes, in.Now)
			out.Ops, notices = append(out.Ops, slices.Concat(main, comp, side)...), append(notices, slices.Concat(more, most, last)...)
			pairs, rootPairs[k], out.Pairs = append(pairs, mk...), append(rootPairs[k], mk...), append(out.Pairs, mk...)
		}
		replacing, told := ReplaceOps(target, decisions, components, current, out.Ops[start:], files, albums)
		out.Ops, notices = append(out.Ops, replacing...), append(notices, told...)
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
				// A detached link names its file by its path alone: left at the old
				// path, it would let the next run take the moved file for a new one.
				if e, ok := files[l.ID]; ok && end == domain.StatusDetached {
					l.Root, l.Path = e.Root, e.Path
				}
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
				if d.Rule == "R-MODE-04" {
					later[[2]string{"plan.albums_kept", d.Link.Remote}]++
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
	finishing := map[domain.AssetID]bool{}
	for _, o := range out.Ops {
		if o.Kind == domain.OpAssetDelete {
			finishing[o.Asset] = true
		}
	}
	// The user may decline the dialog that takes an old asset to Recently
	// Deleted, which leaves its replacement half done until a run finishes it.
	for _, h := range delta.History {
		if !h.Deleted && !finishing[h.AssetID] {
			finishing[h.AssetID] = true
			out.Ops = append(out.Ops, domain.Op{Kind: domain.OpAssetDelete, Asset: h.AssetID, Rule: "R-STATUS-08"})
		}
	}
	hashes, relinked := map[string]domain.HashKind{}, map[string]bool{}
	for _, r := range remotes {
		hashes[r.Name] = r.Capabilities.ContentHash
	}
	content := func(o domain.Op) string { return moveKey(o.Links[0].MD5, o.Links[0].SHA256, hashes[o.Remote]) }
	for _, o := range out.Ops {
		if o.Kind == domain.OpImport && o.Links[0].ID != 0 {
			relinked[content(o)] = true
		}
	}
	delete(relinked, "")
	// A new file can wait to be matched on the next run with the asset that the
	// import of a link makes; a linked file cannot, as no run takes it for new.
	out.Ops = slices.DeleteFunc(out.Ops, func(o domain.Op) bool { return o.Kind == domain.OpImport && o.Links[0].ID == 0 && relinked[content(o)] })
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
	for _, r := range remotes {
		for _, f := range read[r.Name].NewFiles {
			code := "plan.not_imported"
			if f.Decision.Action == domain.ActionImport {
				code = "plan.waiting_import"
			}
			later[[2]string{code, r.Name}]++
		}
		later[[2]string{"plan.edits_left", r.Name}] += len(read[r.Name].Edited)
	}
	for _, o := range out.Ops {
		switch {
		case o.Kind == domain.OpImport && o.Links[0].ID == 0:
			for _, l := range o.Links {
				if l.Role == domain.RoleOriginal || l.Role == domain.RoleSource {
					later[[2]string{"plan.waiting_import", l.Remote}]--
				}
			}
		case o.Rule == "R-STATUS-10":
			later[[2]string{"plan.edits_left", o.Remote}]--
		}
	}
	for k, n := range later {
		if n > 0 {
			notices = append(notices, domain.Notice{Code: k[0], Params: []string{k[1], strconv.Itoa(n)}})
		}
	}
	for i, x := range out.Pairs {
		if a, _ := albumByID(albums, x.AlbumID); layout.DirFor(a, in.Policies) == x.DirPath {
			out.Pairs[i].NamedAfter = pathKey(a)
		}
	}
	slices.SortFunc(out.Ops, domain.Op.Compare)
	slices.SortFunc(notices, func(x, y domain.Notice) int {
		return cmp.Or(cmp.Compare(x.Code, y.Code), slices.Compare(x.Params, y.Params))
	})
	out.Notices = notices
	return out, delta, nil
}
