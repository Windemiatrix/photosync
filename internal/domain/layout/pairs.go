package layout

import (
	"cmp"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// FirstPairs returns the pairs that the albums and folders without one in
// pairs make with the directories of dirs without one, in the same remote
// root; albums in the trash get none. An album pairs by name with the
// directory that DirFor gives it, and of several such albums the lowest
// AlbumID wins. Otherwise it pairs with the directory that holds more than
// half of its files, unless the files there join no album, as in a day
// folder; of several such albums the one with the most files there wins,
// then the lowest AlbumID. files maps an album to the paths of the cloud
// files linked to its assets, one per file, as the current scan lists them,
// so the directory of each path is in dirs. AlbumIDs are unique. The pairs
// are in the order of AlbumID, nil when there is none, with CreatedBy
// domain.OriginRemote; Remote and Root are left to the caller.
func FirstPairs(albums []domain.Album, pairs []domain.AlbumPair, dirs map[string]bool, files map[domain.AlbumID][]string, p domain.Policies) []domain.AlbumPair {
	paired := map[domain.AlbumID]bool{}
	taken := map[string]bool{}
	for _, old := range pairs {
		paired[old.AlbumID], taken[old.DirPath] = true, true
	}
	open := slices.DeleteFunc(slices.Clone(albums), func(a domain.Album) bool { return paired[a.ID] || a.InTrash })
	slices.SortFunc(open, func(x, y domain.Album) int { return cmp.Compare(x.ID, y.ID) })
	var made []domain.AlbumPair
	add := func(id domain.AlbumID, dir string) {
		paired[id], taken[dir] = true, true
		made = append(made, domain.AlbumPair{AlbumID: id, DirPath: dir, CreatedBy: domain.OriginRemote})
	}
	for _, a := range open {
		if dir := DirFor(a, p); dirs[dir] && !taken[dir] {
			add(a.ID, dir)
		}
	}
	var claims []claim
	for _, a := range open {
		if paired[a.ID] {
			continue
		}
		if dir, n, ok := majority(files[a.ID], p); ok {
			claims = append(claims, claim{a.ID, dir, n})
		}
	}
	slices.SortFunc(claims, func(x, y claim) int { return cmp.Or(cmp.Compare(y.files, x.files), cmp.Compare(x.album, y.album)) })
	for _, c := range claims {
		if !taken[c.dir] {
			add(c.album, c.dir)
		}
	}
	slices.SortFunc(made, func(x, y domain.AlbumPair) int { return cmp.Compare(x.AlbumID, y.AlbumID) })
	return made
}

type claim struct {
	album domain.AlbumID
	dir   string
	files int
}

// At most one directory holds more than half of the files, so the order of
// the map does not matter.
func majority(paths []string, p domain.Policies) (dir string, files int, ok bool) {
	count := map[string]int{}
	for _, f := range paths {
		count[path.Dir(f)]++
	}
	for d, n := range count {
		if n*2 > len(paths) && holdsAlbum(d, p) {
			return d, n, true
		}
	}
	return "", 0, false
}

// The files of the root, the albums root, a year folder and a day folder
// join no album; pairing one of them would pull those files into an album.
func holdsAlbum(dir string, p domain.Policies) bool {
	a, ok := AlbumFor(dir, map[string]bool{dir: true}, p)
	return ok && a.Kind != domain.KindFolder
}
