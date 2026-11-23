package plan

import (
	"cmp"
	"maps"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// PairDirs returns made, the album pairs that a run makes in root of remote
// r, and added, the albums and folders that it adds to iCloud for them.
// albums are the albums and folders of the run: those of the snapshot
// reconciled with the state and those that the calls for other roots and
// remotes added; last is the highest AlbumID of the state. pairs are the
// pairs of the run in every remote and root. dirs holds each directory of
// root, as path.Dir gives it, with a main file of an asset that is linked
// there now or is new, and files maps an album to the paths of the linked
// ones of its assets, as layout.FirstPairs takes it.
//
// The albums without a pair in root pair as layout.FirstPairs pairs them with
// the directories of dirs and those above them; folders get no pair. Then
// each directory of dirs without a pair, in the order of their paths, stands for the album,
// not a folder, that layout.AlbumFor gives it with p, if any. The album, not a
// folder, of albums or added with that title and those FolderTitles, outside
// the trash and without a pair in root, the lowest AlbumID first, takes the
// directory. Else the run adds the album and each folder above it that it
// does not find by title and FolderTitles outside the trash, paired or not,
// unless r is upload-only or root is the personal one while
// p.AlbumsForPersonal is false. Added albums and folders get AlbumIDs above
// last and those of albums, Kind from layout.Classify with p, and ParentID. Pairs
// have CreatedBy domain.OriginRemote. Both results are in the order of
// AlbumID, and the inputs are not changed.
func PairDirs(albums []domain.Album, pairs []domain.AlbumPair, last domain.AlbumID, r domain.RemoteSpec, root domain.LibraryKind, dirs map[string]bool, files map[domain.AlbumID][]string, p domain.Policies) (made []domain.AlbumPair, added []domain.Album) {
	pairs = slices.DeleteFunc(slices.Clone(pairs), func(x domain.AlbumPair) bool { return x.Remote != r.Name || x.Root != root })
	above := map[string]bool{}
	for d := range dirs {
		for ; !above[d]; d = path.Dir(d) {
			above[d] = true
		}
	}
	// A folder holds no assets: a pair would give a file moved into its
	// directory an album that cannot take it.
	first := layout.FirstPairs(slices.DeleteFunc(slices.Clone(albums), func(a domain.Album) bool { return a.Kind == domain.KindFolder }), pairs, above, files, p)
	known := slices.Clone(albums)
	slices.SortFunc(known, func(x, y domain.Album) int { return cmp.Compare(x.ID, y.ID) })
	at, paired := map[string]domain.AlbumID{}, map[domain.AlbumID]bool{}
	for _, a := range known {
		last = max(last, a.ID)
	}
	for _, x := range pairs {
		at[x.DirPath], paired[x.AlbumID] = x.AlbumID, true
	}
	add := func(id domain.AlbumID, dir string) {
		at[dir], paired[id] = id, true
		made = append(made, domain.AlbumPair{AlbumID: id, Remote: r.Name, Root: root, DirPath: dir, CreatedBy: domain.OriginRemote})
	}
	for _, x := range first {
		add(x.AlbumID, x.DirPath)
	}
	find := func(a domain.Album) (domain.AlbumID, bool) {
		i := slices.IndexFunc(known, func(b domain.Album) bool {
			return !b.InTrash && (a.Kind == domain.KindFolder) == (b.Kind == domain.KindFolder) &&
				(b.Kind == domain.KindFolder || !paired[b.ID]) && a.Title == b.Title && slices.Equal(a.FolderTitles, b.FolderTitles)
		})
		if i < 0 {
			return 0, false
		}
		return known[i].ID, true
	}
	create := func(a domain.Album) domain.AlbumID {
		last++
		a.ID, a.Kind = last, layout.Classify(a, p)
		added, known = append(added, a), append(known, a)
		return last
	}
	adds := r.Mode != domain.ModeUploadOnly && (root != domain.LibraryPersonal || p.AlbumsForPersonal)
	for _, d := range slices.Sorted(maps.Keys(dirs)) {
		if _, ok := at[d]; ok {
			continue
		}
		a, ok := layout.AlbumFor(d, p)
		if !ok || a.Kind == domain.KindFolder {
			continue
		}
		id, found := find(a)
		if !found && !adds {
			continue
		}
		if !found {
			var folder domain.AlbumID
			for i, title := range a.FolderTitles {
				f := domain.Album{Kind: domain.KindFolder, FolderTitles: slices.Clone(a.FolderTitles[:i]), Title: title, ParentID: folder}
				if folder, found = find(f); !found {
					folder = create(f)
				}
			}
			a.ParentID = folder
			id = create(a)
		}
		add(id, d)
	}
	slices.SortFunc(made, func(x, y domain.AlbumPair) int { return cmp.Compare(x.AlbumID, y.AlbumID) })
	return made, added
}
