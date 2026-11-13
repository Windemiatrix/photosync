package plan

import (
	"cmp"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// NewFile is a new file of a remote that no asset holds: Decision is what a
// run does with it, with a zero Link, and AlbumID is the album that the asset
// imported from it joins, zero for none.
type NewFile struct {
	File     match.Candidate
	Decision domain.LinkDecision
	AlbumID  domain.AlbumID
}

// JoinFresh returns what the new files of remote r make of iCloud once the
// album pairs of the run are known. matched and left are the decisions and
// the new files that MatchFresh gives for r, and pairs are the album pairs of
// the run in every remote and root. linked are matched in the order of link
// ID, the Link of each with the AlbumID of the pair of r for its root and the
// directory of its path, zero with none. join gives, by AssetID, the albums
// of those pairs with the rule of the decision, in the order of album ID and
// then rule, without repeats: an asset joins the album of each directory
// where matching found its file and keeps the albums it is in. A link under
// the personal root while p.AlbumsForPersonal is false, and any link of an
// upload-only r, joins no album. newFiles holds a NewFile for each of left,
// in the order of Root and Path, but for one whose Main has the hash of kind
// r.Capabilities.ContentHash of the Main of one before it, an empty hash
// matching none, so that a content is imported once. Its Decision is that of
// ModeDecide on the sides none and new, and its AlbumID is that of the pair
// of r for its Main, zero where a link would join no album. The inputs are
// not changed.
func JoinFresh(matched []domain.LinkDecision, left []match.Candidate, pairs []domain.AlbumPair, r domain.RemoteSpec, p domain.Policies) (linked []domain.LinkDecision, join map[domain.AssetID][]domain.AlbumChange, newFiles []NewFile) {
	joins := func(root domain.LibraryKind) bool {
		return r.Mode != domain.ModeUploadOnly && (root != domain.LibraryPersonal || p.AlbumsForPersonal)
	}
	byDir := map[domain.AlbumPair]domain.AlbumID{}
	for _, x := range pairs {
		byDir[domain.AlbumPair{Remote: x.Remote, Root: x.Root, DirPath: x.DirPath}] = x.AlbumID
	}
	album := func(root domain.LibraryKind, at string) domain.AlbumID {
		return byDir[domain.AlbumPair{Remote: r.Name, Root: root, DirPath: path.Dir(at)}]
	}
	join = map[domain.AssetID][]domain.AlbumChange{}
	for _, d := range matched {
		d.Link.AlbumID = album(d.Link.Root, d.Link.Path)
		linked = append(linked, d)
		if id := d.Link.AssetID; d.Link.AlbumID != 0 && joins(d.Link.Root) {
			join[id] = objectChanges(append(join[id], domain.AlbumChange{AlbumID: d.Link.AlbumID, Rule: d.Rule}))
		}
	}
	slices.SortFunc(linked, func(x, y domain.LinkDecision) int { return cmp.Compare(x.Link.ID, y.Link.ID) })
	decision, _ := ModeDecide(r.Mode, domain.ChangeNone, domain.ChangeNew, false)
	left = slices.Clone(left)
	slices.SortFunc(left, func(x, y match.Candidate) int {
		return cmp.Or(cmp.Compare(x.Main.Root, y.Main.Root), cmp.Compare(x.Main.Path, y.Main.Path))
	})
	seen := map[string]bool{}
	for _, c := range left {
		k := moveKey(c.Main.MD5, c.Main.SHA256, r.Capabilities.ContentHash)
		if k != "" && seen[k] {
			continue
		}
		seen[k] = true
		f := NewFile{File: c, Decision: decision}
		if joins(c.Main.Root) {
			f.AlbumID = album(c.Main.Root, c.Main.Path)
		}
		newFiles = append(newFiles, f)
	}
	return linked, join, newFiles
}
