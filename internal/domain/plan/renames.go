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
// pair, to the directory that layout.DirFor gives the album with p; one with
// an empty NamedAfter stays, so that a directory that the user named
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
		if a.ID == 0 || a.InTrash || a.Kind == domain.KindFolder || x.NamedAfter == "" || x.NamedAfter == pathKey(a) || taken[strings.ToUpper(to)] {
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
