package plan

import (
	"cmp"
	"maps"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Simulate returns the state, the snapshot and the scans that a run leaves
// once every operation of p succeeds, where p and delta are what Build gives
// for in and p may be cut by Limit. The state takes delta as the run writes
// it before the operations: its assets and albums over those of in.State
// with the same ID, the others of in.State staying, and its History. The
// pairs of p join those of in.State. The links of p, then those of each
// operation in the order of p.Ops, go over the links of in.State with the
// same ID: a link with an empty Status leaves the state, and one with a zero
// ID takes the next ID above every link ID of in.State and p. The conflicts
// of p take the place of those of in.State with the same AssetID and Remote,
// and a resolved one leaves the state. In the scans, an operation on the
// cloud deletes, expires, moves, quarantines or copies the entry at its Root
// and Src in its remote, a moved or copied entry keeping all but its root
// and path, which become DstRoot and Dst, and a put writes at DstRoot and Dst
// an entry with the size and hashes of its link and the MTime in.Now; no
// directory comes or goes. Operations on iCloud change nothing but the links
// of the state, and the snapshot holds what in.Snapshot holds. The assets,
// albums and links of the state are in the order of ID, its pairs of
// AlbumID, Remote and Root, its History as in delta and its conflicts of
// AssetID and Remote; the entries of each scan are in the order of Root and
// Path, a remote left with no entry has no key in the scans, which are never
// nil, and the assets and albums of the snapshot of AssetKey.Compare and
// UUID. The inputs are not changed.
func Simulate(in Input, p domain.Plan, delta domain.SnapshotDelta) (domain.State, domain.LibrarySnapshot, map[string][]domain.RemoteEntry) {
	assets, albums := map[domain.AssetID]domain.Asset{}, map[domain.AlbumID]domain.Album{}
	for _, a := range slices.Concat(in.State.Assets, delta.Assets) {
		assets[a.ID] = a
	}
	for _, a := range slices.Concat(in.State.Albums, delta.Albums) {
		albums[a.ID] = a
	}
	written := slices.Concat(in.State.Links, p.Links)
	for _, o := range p.Ops {
		written = append(written, o.Links...)
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
	for _, o := range p.Ops {
		from, to := domain.Link{Remote: o.Remote, Root: o.Root, Path: o.Src}, domain.Link{Remote: o.Remote, Root: o.DstRoot, Path: o.Dst}
		e := entries[from]
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
		}
	}
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
	pairs := slices.Concat(in.State.AlbumPairs, p.Pairs)
	slices.SortFunc(pairs, func(x, y domain.AlbumPair) int {
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
		History:    slices.Clone(delta.History),
		Conflicts:  conflicts,
	}
	snap := domain.LibrarySnapshot{
		Assets: slices.SortedFunc(slices.Values(in.Snapshot.Assets), func(x, y domain.Asset) int { return x.Key.Compare(y.Key) }),
		Albums: slices.SortedFunc(slices.Values(in.Snapshot.Albums), func(x, y domain.Album) int { return cmp.Compare(x.UUID, y.UUID) }),
	}
	return state, snap, scans
}
