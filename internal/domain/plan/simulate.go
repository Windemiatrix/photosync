package plan

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// Simulate returns the state, the snapshot and the scans that a run leaves
// once every operation of p succeeds, where p and delta are what Build gives
// for in and p may be cut by Limit. The state takes delta as the run writes
// it before the operations: its assets and albums over those of in.State
// with the same ID, the others of in.State staying, and its History. A pair
// of p takes the place of the pair of in.State for its album, remote and
// root, if any. The links of p, then those of each operation in the order
// of p.Ops, go over the links of in.State with the same ID: a link with an
// empty Status leaves the state, and one with a zero ID takes the next ID
// above every link ID of in.State and p. The conflicts
// of p take the place of those of in.State with the same AssetID and Remote,
// and a resolved one leaves the state. In the scans, an operation on the
// cloud deletes, expires, moves, quarantines or copies the entry at its Root
// and Src in its remote, a moved or copied entry keeping all but its root
// and path, which become DstRoot and Dst, and a put writes at DstRoot and Dst
// an entry with the size and hashes of its link and the MTime in.Now; no
// directory comes or goes.
//
// In iCloud, the album or the asset that an operation makes takes the UUID
// "sim-", in.RunID, "-" and i, the index of the operation in p.Ops, and no
// cloud GUID. An OpAlbumCreate adds the album of p.Albums with its AlbumID to
// the state with that UUID, the UUID of the album of its ParentID as
// ParentUUID and the key that layout.Key gives it, and to the snapshot
// without ID, ParentID and key, and without Kind unless it is a folder. An
// OpAlbumRename gives its album the title Dst in the state and the snapshot
// alike and, unless the key of the album is a cloud one, the path key of its
// new title in the state. An OpImport adds to the snapshot an asset of the
// personal library named by the base of Src, with the capture time, offset,
// fraction of a second, width and height of the Meta of the entry at Src as
// the operations before it leave it, in the album of AlbumID unless it is
// zero, and with a resource for each of its links in their order, the first
// the original, each named by the base of its path, with its size. The state
// takes that asset with the hashes of the links in its resources, Root as its
// TargetLibrary and in.RunID as the FirstSeenRun of the album: under the ID
// of Asset, whose key before goes to History with Deleted false, or, for a
// zero Asset, under the next ID above every asset ID of in.State, delta and
// the imports before it, which its links with a zero AssetID take. An
// OpAlbumAdd or an OpAlbumRemove puts its asset into its album, with in.RunID
// as FirstSeenRun, or takes it out, in the state and the snapshot alike,
// unless the asset is in the album already or not in it. An OpAssetDelete
// moves to Recently Deleted, in the snapshot, each identity of its asset that
// History holds with Deleted false, which turns true, and, unless under
// R-STATUS-08, the asset itself, in the state too.
//
// The assets, albums and links of the state are in the order of ID, its
// pairs of AlbumID, Remote and Root, its History of AssetID, then
// AssetKey.Compare, and its conflicts of AssetID and Remote; the entries of
// each scan are in the order of Root and Path, a remote left with no entry
// has no key in the scans, which are never nil, and the assets and albums
// of the snapshot of AssetKey.Compare and UUID. The inputs are not changed,
// and the result may share with them, and its state with its snapshot, the
// slices and pointers that Simulate leaves as they are.
func Simulate(in Input, p domain.Plan, delta domain.SnapshotDelta) (domain.State, domain.LibrarySnapshot, map[string][]domain.RemoteEntry) {
	assets, albums, born := map[domain.AssetID]domain.Asset{}, map[domain.AlbumID]domain.Album{}, domain.AssetID(0)
	for _, a := range slices.Concat(in.State.Assets, delta.Assets) {
		assets[a.ID], born = a, max(born, a.ID)
	}
	for _, a := range slices.Concat(in.State.Albums, delta.Albums) {
		albums[a.ID] = a
	}
	ops, written := slices.Clone(p.Ops), slices.Concat(in.State.Links, p.Links)
	for i := range ops {
		if ops[i].Kind == domain.OpImport && ops[i].Asset == 0 {
			born++
			ops[i].Asset = born
		}
		for _, l := range ops[i].Links {
			l.AssetID = cmp.Or(l.AssetID, ops[i].Asset)
			written = append(written, l)
		}
	}
	links, last := map[domain.LinkID]domain.Link{}, domain.LinkID(0)
	for _, l := range written {
		last = max(last, l.ID)
	}
	for _, l := range written {
		if l.ID == 0 {
			last++
			l.ID = last
		}
		links[l.ID] = l
		if l.Status == "" {
			delete(links, l.ID)
		}
	}
	entries := map[domain.Link]domain.RemoteEntry{}
	for name, scan := range in.Scans {
		for _, e := range scan {
			entries[domain.Link{Remote: name, Root: e.Root, Path: e.Path}] = e
		}
	}
	library, made, shown := map[domain.AssetKey]domain.Asset{}, map[domain.AlbumID]domain.Album{}, slices.Clone(in.Snapshot.Albums)
	for _, a := range in.Snapshot.Assets {
		library[a.Key] = a
	}
	for _, a := range p.Albums {
		made[a.ID] = a
	}
	history, gone := slices.Clone(delta.History), map[domain.AssetID]bool{}
	trash := func(k domain.AssetKey) {
		a := library[k]
		a.InTrash = true
		library[k] = a
	}
	for i, o := range ops {
		from, to := domain.Link{Remote: o.Remote, Root: o.Root, Path: o.Src}, domain.Link{Remote: o.Remote, Root: o.DstRoot, Path: o.Dst}
		e, uuid := entries[from], fmt.Sprintf("sim-%s-%d", in.RunID, i)
		e.Root, e.Path = o.DstRoot, o.Dst
		switch o.Kind {
		case domain.OpDelete, domain.OpExpire:
			delete(entries, from)
		case domain.OpMove, domain.OpQuarantine:
			delete(entries, from)
			entries[to] = e
		case domain.OpCopy:
			entries[to] = e
		case domain.OpPut:
			for _, l := range o.Links {
				entries[to] = domain.RemoteEntry{Root: o.DstRoot, Path: o.Dst, Size: l.Size, MD5: l.MD5, SHA256: l.SHA256, MTime: in.Now}
			}
		case domain.OpAlbumCreate:
			a := made[o.AlbumID]
			a.UUID, a.ParentUUID = uuid, albums[a.ParentID].UUID
			a.Key, a.KeySource = layout.Key(a)
			albums[a.ID] = a
			kind := a.Kind
			if kind != domain.KindFolder {
				kind = ""
			}
			shown = append(shown, domain.Album{UUID: a.UUID, Kind: kind, Title: a.Title, ParentUUID: a.ParentUUID, FolderTitles: a.FolderTitles})
		case domain.OpAlbumRename:
			a := albums[o.AlbumID]
			a.Title = o.Dst
			if a.KeySource == domain.KeyPath {
				a.Key = pathKey(a)
			}
			albums[a.ID] = a
			for j := range shown {
				if shown[j].UUID == a.UUID {
					shown[j].Title = o.Dst
				}
			}
		case domain.OpImport:
			if before, ok := assets[o.Asset]; ok {
				history = append(history, domain.AssetHistory{AssetID: o.Asset, Key: before.Key})
			}
			a := domain.Asset{
				Key: domain.AssetKey{UUID: uuid}, Library: domain.LibraryPersonal, OriginalName: path.Base(o.Src),
				Taken: e.Meta.Taken, TZOffset: e.Meta.TZOffset, SubSec: e.Meta.SubSec, Width: e.Meta.Width, Height: e.Meta.Height,
			}
			var hashed []domain.Resource
			var members []domain.Membership
			if o.AlbumID != 0 {
				a.AlbumUUIDs, members = []string{albums[o.AlbumID].UUID}, []domain.Membership{{AlbumID: o.AlbumID, FirstSeenRun: in.RunID}}
			}
			for j, l := range o.Links {
				r := domain.Resource{Role: domain.ResourceRole(l.Role), Filename: path.Base(l.Path), Size: l.Size}
				if j == 0 {
					r.Role = domain.ResourceOriginal
				}
				a.Resources = append(a.Resources, r)
				r.SHA256, r.MD5 = l.SHA256, l.MD5
				hashed = append(hashed, r)
			}
			library[a.Key] = a
			a.ID, a.TargetLibrary, a.Resources, a.Memberships = o.Asset, o.Root, hashed, members
			assets[o.Asset] = a
		case domain.OpAlbumAdd, domain.OpAlbumRemove:
			a := assets[o.Asset]
			at, member := slices.BinarySearchFunc(a.Memberships, o.AlbumID, func(m domain.Membership, id domain.AlbumID) int {
				return cmp.Compare(m.AlbumID, id)
			})
			switch {
			case o.Kind == domain.OpAlbumAdd && !member:
				a.Memberships = slices.Concat(a.Memberships[:at], []domain.Membership{{AlbumID: o.AlbumID, FirstSeenRun: in.RunID}}, a.Memberships[at:])
				a.AlbumUUIDs = slices.Concat(a.AlbumUUIDs, []string{albums[o.AlbumID].UUID})
			case o.Kind == domain.OpAlbumRemove && member:
				a.Memberships = slices.Concat(a.Memberships[:at], a.Memberships[at+1:])
				a.AlbumUUIDs = slices.DeleteFunc(slices.Clone(a.AlbumUUIDs), func(u string) bool { return u == albums[o.AlbumID].UUID })
			}
			shot := library[a.Key]
			shot.AlbumUUIDs = a.AlbumUUIDs
			assets[o.Asset], library[a.Key] = a, shot
		case domain.OpAssetDelete:
			gone[o.Asset] = true
			if o.Rule != "R-STATUS-08" {
				a := assets[o.Asset]
				a.InTrash = true
				assets[o.Asset] = a
				trash(a.Key)
			}
		}
	}
	for i, h := range history {
		if gone[h.AssetID] && !h.Deleted {
			trash(h.Key)
			history[i].Deleted = true
		}
	}
	slices.SortFunc(history, func(x, y domain.AssetHistory) int {
		return cmp.Or(cmp.Compare(x.AssetID, y.AssetID), x.Key.Compare(y.Key))
	})
	decided := map[domain.Conflict]bool{}
	var conflicts []domain.Conflict
	for _, c := range p.Conflicts {
		decided[domain.Conflict{AssetID: c.AssetID, Remote: c.Remote}] = true
		if c.Status != domain.ConflictStatusResolved {
			conflicts = append(conflicts, c)
		}
	}
	for _, c := range in.State.Conflicts {
		if !decided[domain.Conflict{AssetID: c.AssetID, Remote: c.Remote}] {
			conflicts = append(conflicts, c)
		}
	}
	slices.SortFunc(conflicts, func(x, y domain.Conflict) int {
		return cmp.Or(cmp.Compare(x.AssetID, y.AssetID), cmp.Compare(x.Remote, y.Remote))
	})
	paired := map[domain.AlbumPair]domain.AlbumPair{}
	for _, x := range slices.Concat(in.State.AlbumPairs, p.Pairs) {
		paired[domain.AlbumPair{AlbumID: x.AlbumID, Remote: x.Remote, Root: x.Root}] = x
	}
	pairs := slices.SortedFunc(maps.Values(paired), func(x, y domain.AlbumPair) int {
		return cmp.Or(cmp.Compare(x.AlbumID, y.AlbumID), cmp.Compare(x.Remote, y.Remote), cmp.Compare(x.Root, y.Root))
	})
	scans := map[string][]domain.RemoteEntry{}
	for at, e := range entries {
		scans[at.Remote] = append(scans[at.Remote], e)
	}
	for _, scan := range scans {
		slices.SortFunc(scan, func(x, y domain.RemoteEntry) int {
			return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
		})
	}
	state := domain.State{
		Assets:     slices.SortedFunc(maps.Values(assets), func(x, y domain.Asset) int { return cmp.Compare(x.ID, y.ID) }),
		Albums:     slices.SortedFunc(maps.Values(albums), func(x, y domain.Album) int { return cmp.Compare(x.ID, y.ID) }),
		AlbumPairs: pairs,
		Links:      slices.SortedFunc(maps.Values(links), func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) }),
		History:    history,
		Conflicts:  conflicts,
	}
	snap := domain.LibrarySnapshot{
		Assets: slices.SortedFunc(maps.Values(library), func(x, y domain.Asset) int { return x.Key.Compare(y.Key) }),
		Albums: slices.SortedFunc(slices.Values(shown), func(x, y domain.Album) int { return cmp.Compare(x.UUID, y.UUID) }),
	}
	return state, snap, scans
}
