package plan

import (
	"cmp"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// RemoteScan is what a run reads from the scan of one remote. Linked are
// the decisions on its new links as JoinFresh gives them with DecideDerived
// applied, Join and NewFiles are as JoinFresh gives them, Edited is as
// ReadScan gives it, Settling holds the assets that the run must leave as
// they are in the remote, a file of theirs there not having settled or lying
// outside the templates, as ReadScans tells, Renamed holds the albums and
// the folders of albums that take the titles of their directories there,
// Refiled the albums that take their folders, and Moved the pairs that
// MovePairs moves there, as they were before.
type RemoteScan struct {
	Linked   []domain.LinkDecision
	Join     map[domain.AssetID][]domain.AlbumChange
	NewFiles []NewFile
	Edited   []domain.RemoteEntry
	Settling map[domain.AssetID]bool
	Renamed  []domain.Album
	Refiled  []domain.Album
	Moved    []domain.AlbumPair
}

// ReadScans reads the scans of remotes, in the order of their names, before a
// run decides on its assets. state is what the last applied run left, delta
// the snapshot reconciled with it, and scans give by the name of a remote its
// whole listing. An entry that is no directory and whose MTime is after now
// less p.SettleMinutes has not settled: the scan leaves it out, and notices
// hold "plan.settling" with the remote, the root and the path of each. A link
// of the remote at the root and path of such an entry is not given to
// ReadScan, and Settling holds its asset, as it holds the asset of a link of
// the remote whose content such an entry has, by the hash of kind
// Capabilities.ContentHash of the remote. An entry that has settled, is no
// directory and is not under .photosync at the top of a root lies outside
// the templates when layout.AlbumFor with p reads its directory, as path.Dir
// gives it, as no album or as a folder. A directory that a pair of the remote
// in state holds at its root decides first: it lies inside the templates
// while delta holds the album of the pair as an event, in Recently Deleted or
// not, and outside them while delta holds that album as thematic outside
// Recently Deleted. Unless match.Edited with p.EditedDirs takes it for an
// edit and no link of the remote lies at its root and path, the scan leaves
// it out too, and notices hold "plan.skipped" with "dir", the remote, the root
// and that directory, once for each remote, root and directory. A link of the remote at the root and path of an entry
// left out is not given to ReadScan, and Settling holds its asset, as it
// holds the asset of a link of the remote at whose root and path the listing
// has no entry and whose content an entry outside the templates has.
// ReadScan reads the rest with p.EditedDirs. Its fresh lose those that share
// the root, the directory and the export.StemKey of the name with an entry
// that has not settled, which wait with it. MatchFresh matches the fresh left
// with delta.Assets, new link IDs running on from the highest link ID of
// state.
//
// For the personal root, then the shared one, RenamedDirs runs with the
// albums of delta, each as the last RenamedDirs gave it or, inside a folder
// that it renamed, under the new title of that folder, and those added
// before, the pairs of state and those made before and the highest AlbumID of
// state; each pair it moves takes the place of its old one. PairDirs then
// pairs the directories with the albums and pairs as they are by then. Its
// dirs and files hold the files of the original or the source role of the new
// links and of the active links of the remote, where ReadScan finds them,
// whose asset is in delta outside Recently Deleted, Syncs under p, is not in
// Settling and is not one that StatusFrozen holds for, but for a file still at
// the path of its link outside the directory that the pair of the remote in
// state holds for its album and root, which a moved pair left behind; dirs
// alone hold the main files of the new files too, and a file goes in files
// under each album of the Memberships of its asset. MovePairs then moves the
// pairs with the whole scan of the remote. layout.FirstPairs gives the
// albums, not folders, the directories that those moved pairs leave, by the
// files of PairDirs; the pairs it makes there are made with the remote and
// the root. JoinFresh runs with the pairs of state, those that RenamedDirs
// moves in place of their old ones and those that PairDirs makes, and
// DecideDerived with the fresh left and p.DerivedCopy.
//
// files is the union of what ReadScan gives. added and made are what
// RenamedDirs, PairDirs, MovePairs and layout.FirstPairs add and make, in the
// order of the calls. The Renamed and the Refiled of a remote hold what
// RenamedDirs renames and refiles there, and its Moved the pairs that
// MovePairs moves there as they were before. notices hold, for each remote
// in turn, those of settling and skipping, in the order of the root and the
// path of their entries, then those of ReadScan, MatchFresh and
// DecideDerived. An entry under the directory
// .photosync at the top of a root, which ReadScan leaves out, has settled
// whatever its MTime. The inputs are not changed.
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
	home := map[domain.AlbumPair]string{}
	for _, x := range state.AlbumPairs {
		home[domain.AlbumPair{AlbumID: x.AlbumID, Remote: x.Remote, Root: x.Root}] = x.DirPath
	}
	since := now.Add(-time.Duration(p.SettleMinutes) * time.Minute)
	// Asking AlbumFor once for each directory keeps a scan of tens of
	// thousands of files from reading the templates for each.
	events := map[string]bool{}
	inEvent := func(dir string) bool {
		in, known := events[dir]
		if !known {
			a, ok := layout.AlbumFor(dir, p)
			in = ok && a.Kind != domain.KindFolder
			events[dir] = in
		}
		return in
	}
	isEdit := func(e domain.RemoteEntry) bool {
		_, edits := match.Edited([]domain.RemoteEntry{e}, p.EditedDirs)
		return edits != nil
	}
	// The pair of an album, not the cloud template, says what its directory
	// holds: a new cloud template must not freeze the events, and an album
	// that left the templates of Photos keeps its files until it comes back.
	albumOf := map[domain.AlbumID]domain.Album{}
	for _, a := range delta.Albums {
		albumOf[a.ID] = a
	}
	paired := map[[3]string]bool{}
	for _, x := range state.AlbumPairs {
		a, ok := albumOf[x.AlbumID]
		if ok && (a.Kind == domain.KindEvent || a.Kind == domain.KindThematic && !a.InTrash) {
			k := [3]string{x.Remote, string(x.Root), x.DirPath}
			paired[k] = paired[k] || a.Kind == domain.KindEvent
		}
	}
	for _, r := range slices.SortedFunc(slices.Values(remotes), func(x, y domain.RemoteSpec) int { return cmp.Compare(x.Name, y.Name) }) {
		hash := r.Capabilities.ContentHash
		young, held, shots := map[[2]string]bool{}, map[string]bool{}, map[[3]string]bool{}
		present, outside, carried, told := map[[2]string]bool{}, map[[2]string]bool{}, map[string]bool{}, map[[2]string]bool{}
		linkedAt := map[[2]string]bool{}
		for _, l := range state.Links {
			if l.Remote == r.Name {
				linkedAt[[2]string{string(l.Root), l.Path}] = true
			}
		}
		var scan []domain.RemoteEntry
		for _, e := range slices.SortedFunc(slices.Values(scans[r.Name]), func(x, y domain.RemoteEntry) int {
			return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
		}) {
			k, dir := [2]string{string(e.Root), e.Path}, path.Dir(e.Path)
			present[k] = true
			event, isPair := paired[[3]string{r.Name, string(e.Root), dir}]
			top, _, _ := strings.Cut(e.Path, "/")
			switch {
			// What photosync wrote itself, such as a file it has just quarantined,
			// is no write of the user that may still be going on.
			case e.IsDir || top == ".photosync":
				scan = append(scan, e)
			case e.MTime.After(since):
				young[k], shots[sidecarKey(e.Root, e.Path)] = true, true
				if h := moveKey(e.MD5, e.SHA256, hash); h != "" {
					held[h] = true
				}
				notices = append(notices, domain.Notice{Code: "plan.settling", Params: []string{r.Name, string(e.Root), e.Path}})
			case event || !isPair && inEvent(dir):
				scan = append(scan, e)
			default:
				if h := moveKey(e.MD5, e.SHA256, hash); h != "" {
					carried[h] = true
				}
				// An edit is no file of an album: the edited directories, not the
				// templates, say where it lies. A file that a link names is a file of
				// its asset, whatever its name.
				if isEdit(e) && !linkedAt[k] {
					scan = append(scan, e)
					continue
				}
				outside[k] = true
				if d := [2]string{string(e.Root), dir}; !told[d] {
					told[d] = true
					notices = append(notices, domain.Notice{Code: "plan.skipped", Params: []string{"dir", r.Name, string(e.Root), dir}})
				}
			}
		}
		var links []domain.Link
		settling := map[domain.AssetID]bool{}
		for _, l := range state.Links {
			k := [2]string{string(l.Root), l.Path}
			at := l.Remote == r.Name && (young[k] || outside[k])
			// A file that the user moves out of the templates is no deletion.
			lost := !present[k] && carried[moveKey(l.MD5, l.SHA256, hash)]
			if at || l.Remote == r.Name && (held[moveKey(l.MD5, l.SHA256, hash)] || lost) {
				settling[l.AssetID] = true
			}
			if !at {
				links = append(links, l)
			}
		}
		fs, fresh, edited, ns := ReadScan(links, r, scan, p.EditedDirs)
		// A photo matched or imported without the Live Photo video or RAW file
		// that is still being written beside it would never take that file later.
		fresh = slices.DeleteFunc(fresh, func(e domain.RemoteEntry) bool { return shots[sidecarKey(e.Root, e.Path)] })
		maps.Copy(files, fs)
		matched, left, ms := MatchFresh(delta.Assets, state.Links, fs, r, fresh, lastLink, p, state.Assets)
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
			e, ok := fs[l.ID]
			dir, paired := home[domain.AlbumPair{AlbumID: l.AlbumID, Remote: r.Name, Root: l.Root}]
			behind := paired && dir != path.Dir(e.Path) && e.Root == l.Root && e.Path == l.Path
			if ok && objectFile(l) && found && !a.InTrash && Syncs(a, p) && !settling[l.AssetID] &&
				!StatusFrozen(byAsset[l.AssetID], r.Name) && !behind {
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
		same := func(x domain.AlbumPair) func(domain.AlbumPair) bool {
			return func(y domain.AlbumPair) bool {
				return y.AlbumID == x.AlbumID && y.Remote == x.Remote && y.Root == x.Root
			}
		}
		var renamed, refiled []domain.Album
		var before []domain.AlbumPair
		for _, root := range []domain.LibraryKind{domain.LibraryPersonal, domain.LibraryShared} {
			moved, titled, filed, folders := RenamedDirs(albums, pairs, lastAlbum, state.Links, fs, scan, r, root, p)
			for _, a := range slices.Concat(titled, filed) {
				for _, b := range underFolder(albums, a, p) {
					albums[slices.IndexFunc(albums, func(c domain.Album) bool { return c.ID == b.ID })] = b
				}
			}
			// MovePairs must not take the old place of a pair that the user moved
			// for one more pair of its album.
			for _, x := range moved {
				pairs[slices.IndexFunc(pairs, same(x))] = x
			}
			made, renamed, refiled = append(made, moved...), append(renamed, titled...), append(refiled, filed...)
			added, albums = append(added, folders...), append(albums, folders...)
			newPairs, newAlbums := PairDirs(albums, pairs, lastAlbum, r, root, dirs[root], paths[root], p)
			made, added = append(made, newPairs...), append(added, newAlbums...)
			pairs, albums = append(pairs, newPairs...), append(albums, newAlbums...)
			// The pairs that follow their albums keep their old places for
			// JoinFresh, so that a new file there goes into the album that it was
			// put in. Those places go only to albums that the run has: an album
			// made for one would take that file instead. Pairing them now keeps an
			// album that lies there from waiting for the next run.
			after := slices.DeleteFunc(slices.Clone(pairs), func(x domain.AlbumPair) bool { return x.Remote != r.Name || x.Root != root })
			freed := map[string]bool{}
			for _, x := range MovePairs(albums, pairs, scans[r.Name], r, root, p) {
				i := slices.IndexFunc(after, same(x))
				before, freed[after[i].DirPath] = append(before, after[i]), true
				after[i], made = x, append(made, x)
			}
			owners := slices.DeleteFunc(slices.Clone(albums), func(a domain.Album) bool { return a.Kind == domain.KindFolder })
			for _, x := range layout.FirstPairs(owners, after, freed, paths[root], p) {
				if freed[x.DirPath] {
					x.Remote, x.Root = r.Name, root
					made = append(made, x)
				}
			}
		}
		linked, join, newFiles := JoinFresh(matched, left, pairs, r, p)
		decided, ds := DecideDerived(linked, delta.Assets, fresh, r.Mode, p.DerivedCopy)
		read[r.Name] = RemoteScan{
			Linked: decided, Join: join, NewFiles: newFiles, Edited: edited, Settling: settling, Renamed: renamed, Refiled: refiled, Moved: before,
		}
		notices = slices.Concat(notices, ns, ms, ds)
	}
	return files, read, added, made, notices
}
