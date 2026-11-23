package plan

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// MovePairs returns the pairs of remote r in root that a run moves to the
// directory of the new path of their album, in the order of their old
// DirPath, each with the new one and NamedAfter as it was. albums and pairs
// are those of the run, and listing the whole listing of r. A pair moves
// when its album in albums is no folder, is outside Recently Deleted and has
// a path other than NamedAfter, the path that named the directory of the
// pair, to the directory, if any, that layout.DirFor gives the album with p;
// one with an empty NamedAfter stays, so that a directory that the user named
// otherwise keeps its name (R-LAYOUT-11). It stays too when listing holds
// the new directory or a path below it, or when a pair of r in root, its own
// among them, holds it or another takes it first, letter case aside: a
// client of the cloud blind to case takes such names for one. A
// download-only r moves no pair. The inputs are not changed.
func MovePairs(albums []domain.Album, pairs []domain.AlbumPair, listing []domain.RemoteEntry, r domain.RemoteSpec, root domain.LibraryKind, p domain.Policies) []domain.AlbumPair {
	if r.Mode == domain.ModeDownloadOnly {
		return nil
	}
	own := slices.DeleteFunc(slices.Clone(pairs), func(x domain.AlbumPair) bool { return x.Remote != r.Name || x.Root != root })
	slices.SortFunc(own, func(x, y domain.AlbumPair) int { return cmp.Compare(x.DirPath, y.DirPath) })
	taken, now := map[string]bool{}, map[domain.AlbumID]domain.Album{}
	for _, e := range listing {
		for d := strings.ToUpper(e.Path); e.Root == root && !taken[d]; d = path.Dir(d) {
			taken[d] = true
		}
	}
	for _, x := range own {
		taken[strings.ToUpper(x.DirPath)] = true
	}
	for _, a := range albums {
		now[a.ID] = a
	}
	var moved []domain.AlbumPair
	for _, x := range own {
		a := now[x.AlbumID]
		to := layout.DirFor(a, p)
		if a.ID == 0 || a.InTrash || a.Kind == domain.KindFolder || x.NamedAfter == "" || x.NamedAfter == pathKey(a) || to == "" || taken[strings.ToUpper(to)] {
			continue
		}
		taken[strings.ToUpper(to)], x.DirPath = true, to
		moved = append(moved, x)
	}
	return moved
}

// A rename changes the path of an album and keeps its cloud key, so only a
// key without the cloud key tells the old path from the new.
func pathKey(a domain.Album) string {
	key, _ := layout.Key(domain.Album{FolderTitles: a.FolderTitles, Title: a.Title})
	return key
}

// FollowPairs returns the placements that take the files of remote along
// with the pairs that a run moves, given placements, those that the run
// makes there already. moved are those pairs as they were before MovePairs
// moved them. Each active link of the original or the source role of remote
// in links that lies directly in the directory of a pair of moved for its
// album, remote and root, and that no placement but a copy takes from there,
// gets one that moves it into its album (R-LAYOUT-11), in the order of
// links. The inputs are not changed.
func FollowPairs(links []domain.Link, moved []domain.AlbumPair, remote string, placements []domain.Placement) []domain.Placement {
	var out []domain.Placement
	for _, l := range links {
		from := func(x domain.AlbumPair) bool {
			return x.AlbumID == l.AlbumID && x.Remote == remote && x.Root == l.Root && x.DirPath == path.Dir(l.Path)
		}
		placed := func(pl domain.Placement) bool { return pl.Link.ID == l.ID && pl.Action != domain.ActionCopyFile }
		if l.Remote == remote && objectFile(l) && slices.ContainsFunc(moved, from) && !slices.ContainsFunc(placements, placed) {
			out = append(out, domain.Placement{Action: domain.ActionMoveFile, Link: l, AlbumID: l.AlbumID, Rule: "R-LAYOUT-11"})
		}
	}
	return out
}

// RenamedDirs returns moved, the pairs of remote r in root whose directory
// the user renamed or moved in the cloud, each with its new DirPath and an
// empty NamedAfter, in the order of their old DirPath; renamed and refiled, in the
// same order, the albums of those that take the title (R-LAYOUT-12) or the
// folders (R-LAYOUT-13) that the new directory reads as, renamed also the
// folders of albums that take a new title instead (R-LAYOUT-14); and added,
// the folders that refiled albums need and albums lack, in the order of
// AlbumID. albums and pairs are those of the run, last the highest AlbumID of
// the state, links those of the state, files the entries of r where ReadScan
// finds them, and listing what ReadScan reads. A pair of an album that
// albums hold outside Recently Deleted, with neither its directory nor a
// path below it in listing, claims d when the files of the active links of
// the original or the source role in root that lay directly in its directory
// lie now, one at least, directly in d of root, unless a pair of r in root
// holds d or another claimed it first. It moves to d when layout.AlbumFor
// reads d with p as an
// album, not a folder, either in the folders of its album with d at the
// depth of its old directory or with the title that it reads the old
// directory with. When NamedAfter is the path of the album, which iCloud has
// kept, and r adds albums in root, as it does unless it is upload-only or
// root is the personal one while p.AlbumsForPersonal is false, the album
// takes the title read in its folders at that depth, stays in its folders at
// another depth, and else takes the folders read, the kind that
// layout.Classify gives it with p and, as its parent, the folder of albums at that
// path outside Recently Deleted, the lowest AlbumID first, or one added for
// each level missing, with an AlbumID above last and those of albums. A level
// missing where the folders read differ from those of the album at it alone
// is not added when the folder at the old path to it, found the same way, has
// a directory, as layout.DirFor gives it, and neither it nor a path below
// it is in listing, in any root: that folder takes the new title, and so does that
// level of the folders of all inside it, at any depth, for the pairs that
// follow. An album is refiled only when its parent changes, and a pair of a
// folder moves nothing. The inputs are not changed.
func RenamedDirs(albums []domain.Album, pairs []domain.AlbumPair, last domain.AlbumID, links []domain.Link, files map[domain.LinkID]domain.RemoteEntry, listing []domain.RemoteEntry, r domain.RemoteSpec, root domain.LibraryKind, p domain.Policies) (moved []domain.AlbumPair, renamed, refiled, added []domain.Album) {
	own := slices.DeleteFunc(slices.Clone(pairs), func(x domain.AlbumPair) bool { return x.Remote != r.Name || x.Root != root })
	slices.SortFunc(own, func(x, y domain.AlbumPair) int { return cmp.Compare(x.DirPath, y.DirPath) })
	listed, held := map[string]bool{}, map[string]bool{}
	for _, e := range listing {
		for d := e.Path; e.Root == root && !listed[d]; d = path.Dir(d) {
			listed[d] = true
		}
	}
	for _, x := range own {
		held[x.DirPath] = true
	}
	known := slices.Clone(albums)
	slices.SortFunc(known, func(x, y domain.Album) int { return cmp.Compare(x.ID, y.ID) })
	for _, a := range known {
		last = max(last, a.ID)
	}
	folder := func(titles []string, title string) int {
		return slices.IndexFunc(known, func(b domain.Album) bool {
			return !b.InTrash && b.Kind == domain.KindFolder && b.Title == title && slices.Equal(b.FolderTitles, titles)
		})
	}
	gone := func(dir string) bool {
		return dir != "" && !slices.ContainsFunc(listing, func(e domain.RemoteEntry) bool { return strings.HasPrefix(e.Path+"/", dir+"/") })
	}
	adds := r.Mode != domain.ModeUploadOnly && (root != domain.LibraryPersonal || p.AlbumsForPersonal)
	for _, x := range own {
		i := slices.IndexFunc(albums, func(a domain.Album) bool { return a.ID == x.AlbumID && !a.InTrash && a.Kind != domain.KindFolder })
		if i < 0 || listed[x.DirPath] {
			continue
		}
		var at []string
		for _, l := range links {
			if e, ok := files[l.ID]; ok && e.Root == root && l.Root == root && path.Dir(l.Path) == x.DirPath && objectFile(l) {
				at = append(at, path.Dir(e.Path))
			}
		}
		if len(slices.Compact(at)) != 1 || held[at[0]] {
			continue
		}
		held[at[0]] = true
		a := albums[i]
		was, _ := layout.AlbumFor(x.DirPath, p)
		read, ok := layout.AlbumFor(at[0], p)
		within := slices.Equal(read.FolderTitles, a.FolderTitles) && strings.Count(at[0], "/") == strings.Count(x.DirPath, "/")
		if !ok || read.Kind == domain.KindFolder || !within && read.Title != was.Title {
			continue
		}
		switch {
		case x.NamedAfter != pathKey(a) || !adds:
		case within:
			a.Title = read.Title
			renamed = append(renamed, a)
		case slices.Equal(read.FolderTitles, a.FolderTitles):
		default:
			from := a.FolderTitles
			a.FolderTitles, a.ParentID = read.FolderTitles, 0
			for j, title := range read.FolderTitles {
				f := domain.Album{Kind: domain.KindFolder, Title: title, ParentID: a.ParentID, FolderTitles: slices.Clone(read.FolderTitles[:j])}
				k := folder(f.FolderTitles, title)
				if k < 0 && j < len(from) && slices.Equal(slices.Concat(from[:j], []string{title}, from[j+1:]), read.FolderTitles) {
					if old := folder(from[:j], from[j]); old >= 0 && old < len(albums) && gone(layout.DirFor(known[old], p)) {
						known[old].Title, k = title, old
						for _, b := range underFolder(known, known[old], p) {
							known[slices.IndexFunc(known, func(c domain.Album) bool { return c.ID == b.ID })] = b
						}
						renamed = append(renamed, known[old])
					}
				}
				if k >= 0 {
					a.ParentID = known[k].ID
					continue
				}
				last++
				f.ID, a.ParentID = last, last
				known, added = append(known, f), append(added, f)
			}
			a.Kind = layout.Classify(a, p)
			if a.ParentID != albums[i].ParentID {
				refiled = append(refiled, a)
			}
		}
		x.DirPath, x.NamedAfter = at[0], ""
		moved = append(moved, x)
	}
	return moved, renamed, refiled, added
}

// Albums and folders may share a path, so only ParentID tells what lies in
// a folder. f comes first, so that one loop puts any album in its place.
func underFolder(albums []domain.Album, f domain.Album, p domain.Policies) []domain.Album {
	out, in := []domain.Album{f}, map[domain.AlbumID]bool{f.ID: true}
	for grew := true; grew; {
		grew = false
		for _, a := range albums {
			if in[a.ParentID] && !in[a.ID] && len(a.FolderTitles) > len(f.FolderTitles) {
				a.FolderTitles = slices.Clone(a.FolderTitles)
				a.FolderTitles[len(f.FolderTitles)], in[a.ID], grew = f.Title, true, true
				a.Kind = layout.Classify(a, p)
				out = append(out, a)
			}
		}
	}
	return out
}
