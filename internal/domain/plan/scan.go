package plan

import (
	"slices"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// ReadScan reads scan, the listing of remote r, against links, the links of
// the state in every remote, role and status; only those of r count. The
// directory .photosync at the top of a root, where photosync keeps its own
// files, and every entry under it count for nothing. files gives, by link ID,
// the entry where the file of a link lies now, as DecideRemoteLinks takes it:
// for an active link whose file RemoteSide finds deleted at its path, the
// entry where MovesByContent finds it under r.Capabilities.ContentHash, if
// any; otherwise the entry at the root and path of the link, a directory too,
// if any. An active or detached link names the entry at its path, and a
// quarantined link only when RemoteSide finds there the content of its base.
// fresh are the entries, directories too, that no link names and no move
// takes, without the edits that match.Edited tells by editedDirs, and edited
// are those edits: an entry that a link names or a move takes is no edit.
// notices are those of MovesByContent. fresh and edited are in the order of
// Root, then Path, each nil when empty, and the inputs are not changed.
func ReadScan(links []domain.Link, r domain.RemoteSpec, scan []domain.RemoteEntry, editedDirs []string) (files map[domain.LinkID]domain.RemoteEntry, fresh, edited []domain.RemoteEntry, notices []domain.Notice) {
	key := func(root domain.LibraryKind, p string) [2]string { return [2]string{string(root), p} }
	scan = slices.DeleteFunc(slices.Clone(scan), func(e domain.RemoteEntry) bool {
		top, _, _ := strings.Cut(e.Path, "/")
		return top == ".photosync"
	})
	at := map[[2]string]domain.RemoteEntry{}
	for _, e := range scan {
		at[key(e.Root, e.Path)] = e
	}
	files, named := map[domain.LinkID]domain.RemoteEntry{}, map[[2]string]bool{}
	var gone []domain.Link
	for _, l := range links {
		if l.Remote != r.Name {
			continue
		}
		k := key(l.Root, l.Path)
		e, found := at[k]
		side := RemoteSide(l, e, found, r.Capabilities.ContentHash)
		if l.Status == domain.StatusActive && side == domain.ChangeDeleted {
			gone = append(gone, l)
		}
		if l.Status == domain.StatusActive || l.Status == domain.StatusDetached || side == domain.ChangeUnchanged {
			named[k] = true
		}
		if found {
			files[l.ID] = e
		}
	}
	isNamed := func(e domain.RemoteEntry) bool { return named[key(e.Root, e.Path)] }
	scan = slices.DeleteFunc(scan, isNamed)
	moves, notices := MovesByContent(gone, scan, r.Capabilities.ContentHash)
	for _, m := range moves {
		files[m.Link.ID] = m.To
		named[key(m.To.Root, m.To.Path)] = true
	}
	fresh, edited = match.Edited(slices.DeleteFunc(scan, isNamed), editedDirs)
	return files, fresh, edited, notices
}
