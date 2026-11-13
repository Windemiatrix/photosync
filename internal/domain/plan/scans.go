package plan

import (
	"cmp"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// RemoteScan is what a run reads from the scan of one remote. Linked are
// the decisions on its new links as JoinFresh gives them with DecideDerived
// applied, Join and NewFiles are as JoinFresh gives them, Edited is as
// ReadScan gives it, and Settling holds the assets that the run must leave
// as they are in the remote, a file of theirs there not having settled.
type RemoteScan struct {
	Linked   []domain.LinkDecision
	Join     map[domain.AssetID][]domain.AlbumChange
	NewFiles []NewFile
	Edited   []domain.RemoteEntry
	Settling map[domain.AssetID]bool
}

// ReadScans reads the scans of remotes, in the order of their names, before
// a run decides on its assets. state is what the last applied run left,
// delta the snapshot reconciled with it, and scans give by the name of a
// remote its whole listing. An entry that is no directory and whose MTime is
// after now less p.SettleMinutes has not settled: the scan leaves it out,
// and notices hold "plan.settling" with the remote, the root and the path of
// each. A link of the remote at the root and path of such an entry is not
// given to ReadScan, and Settling holds its asset, as it holds the asset of
// a link of the remote whose content such an entry has, by the hash of kind
// Capabilities.ContentHash of the remote. ReadScan reads the rest with
// p.EditedDirs, and MatchFresh matches its fresh with delta.Assets, new link
// IDs running on from the highest link ID of state. PairDirs runs for the
// personal root, then the shared one, with the albums of delta and those
// added before, the pairs of state and those made before and the highest
// AlbumID of state; its dirs and files hold the files of the original or the
// source role of the new links and of the active links of the remote, where
// ReadScan finds them, whose asset is in delta outside Recently Deleted,
// Syncs under p, is not in Settling and is not one that StatusFrozen holds
// for, and in dirs alone the main files of the new files; a file goes in
// files under each album of the Memberships of its asset. JoinFresh runs
// with the pairs of state and those made, and DecideDerived with fresh and
// p.DerivedCopy. files is the union of what ReadScan gives, added and made
// are what PairDirs gives in the order of the calls, and notices hold, for
// each remote in turn, those of settling, ReadScan, MatchFresh and
// DecideDerived. The inputs are not changed.
func ReadScans(state domain.State, delta domain.SnapshotDelta, scans map[string][]domain.RemoteEntry, remotes []domain.RemoteSpec, p domain.Policies, now time.Time) (files map[domain.LinkID]domain.RemoteEntry, read map[string]RemoteScan, added []domain.Album, made []domain.AlbumPair, notices []domain.Notice) {
	var lastLink domain.LinkID
	byAsset := map[domain.AssetID][]domain.Link{}
	for _, l := range state.Links {
		lastLink, byAsset[l.AssetID] = max(lastLink, l.ID), append(byAsset[l.AssetID], l)
	}
	var lastAlbum domain.AlbumID
	for _, a := range state.Albums {
		lastAlbum = max(lastAlbum, a.ID)
	}
	byID := map[domain.AssetID]domain.Asset{}
	for _, a := range delta.Assets {
		byID[a.ID] = a
	}
	albums, pairs := slices.Clone(delta.Albums), slices.Clone(state.AlbumPairs)
	files, read = map[domain.LinkID]domain.RemoteEntry{}, map[string]RemoteScan{}
	since := now.Add(-time.Duration(p.SettleMinutes) * time.Minute)
	for _, r := range slices.SortedFunc(slices.Values(remotes), func(x, y domain.RemoteSpec) int { return cmp.Compare(x.Name, y.Name) }) {
		hash := r.Capabilities.ContentHash
		young, held := map[[2]string]bool{}, map[string]bool{}
		var scan []domain.RemoteEntry
		for _, e := range slices.SortedFunc(slices.Values(scans[r.Name]), func(x, y domain.RemoteEntry) int {
			return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
		}) {
			if e.IsDir || !e.MTime.After(since) {
				scan = append(scan, e)
				continue
			}
			young[[2]string{string(e.Root), e.Path}] = true
			if k := moveKey(e.MD5, e.SHA256, hash); k != "" {
				held[k] = true
			}
			notices = append(notices, domain.Notice{Code: "plan.settling", Params: []string{r.Name, string(e.Root), e.Path}})
		}
		var links []domain.Link
		settling := map[domain.AssetID]bool{}
		for _, l := range state.Links {
			at := l.Remote == r.Name && young[[2]string{string(l.Root), l.Path}]
			if at || l.Remote == r.Name && held[moveKey(l.MD5, l.SHA256, hash)] {
				settling[l.AssetID] = true
			}
			if !at {
				links = append(links, l)
			}
		}
		fs, fresh, edited, ns := ReadScan(links, r, scan, p.EditedDirs)
		maps.Copy(files, fs)
		matched, left, ms := MatchFresh(delta.Assets, state.Links, fs, r, fresh, lastLink, p)
		dirs, paths := map[domain.LibraryKind]map[string]bool{}, map[domain.LibraryKind]map[domain.AlbumID][]string{}
		count := func(l domain.Link) {
			if dirs[l.Root] == nil {
				dirs[l.Root], paths[l.Root] = map[string]bool{}, map[domain.AlbumID][]string{}
			}
			dirs[l.Root][path.Dir(l.Path)] = true
			for _, m := range byID[l.AssetID].Memberships {
				paths[l.Root][m.AlbumID] = append(paths[l.Root][m.AlbumID], l.Path)
			}
		}
		for _, l := range state.Links {
			a, found := byID[l.AssetID]
			if e, ok := fs[l.ID]; ok && objectFile(l) && found && !a.InTrash && Syncs(a, p) && !settling[l.AssetID] &&
				!StatusFrozen(byAsset[l.AssetID], r.Name) {
				l.Root, l.Path = e.Root, e.Path
				count(l)
			}
		}
		for _, d := range matched {
			lastLink = max(lastLink, d.Link.ID)
			if objectFile(d.Link) {
				count(d.Link)
			}
		}
		for _, c := range left {
			count(domain.Link{Root: c.Main.Root, Path: c.Main.Path})
		}
		for _, root := range []domain.LibraryKind{domain.LibraryPersonal, domain.LibraryShared} {
			newPairs, newAlbums := PairDirs(albums, pairs, lastAlbum, r, root, dirs[root], paths[root], p)
			made, added = append(made, newPairs...), append(added, newAlbums...)
			pairs, albums = append(pairs, newPairs...), append(albums, newAlbums...)
		}
		linked, join, newFiles := JoinFresh(matched, left, pairs, r, p)
		decided, ds := DecideDerived(linked, delta.Assets, fresh, r.Mode, p.DerivedCopy)
		read[r.Name] = RemoteScan{Linked: decided, Join: join, NewFiles: newFiles, Edited: edited, Settling: settling}
		notices = slices.Concat(notices, ns, ms, ds)
	}
	return files, read, added, made, notices
}
