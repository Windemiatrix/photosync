package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Move is a file that the user moved in the cloud: Link is an active link as
// the state has it, whose file is gone from its path, and To is the entry of
// the scan where that file is now.
type Move struct {
	Link domain.Link
	To   domain.RemoteEntry
}

// MovesByContent returns the files of one remote that the user moved. gone
// are the active links of the remote whose file RemoteSide finds deleted, and
// fresh are the entries of its scan that no link names; hash is the
// ContentHash of the remote. A link and an entry have one content when both
// have the same hash of kind hash; an entry without it, such as a directory,
// matches nothing. With HashNone or an unknown kind nothing moves: such a
// remote is not trusted with a hash, and a caller that hashed the files
// itself passes HashSHA256. The links and entries of one content pair one to
// one in the order of Root, then Path, whatever their roots: a link left over
// stays deleted, an entry left over stays new. When a content has links of
// several assets and entries, but fewer entries than links, no order can tell
// which asset lost its file: none of those links moves, and each has a notice
// "plan.ambiguous_move" with its root and path. Moves and notices are in the
// order of the root and path of the link; gone and fresh are not changed.
func MovesByContent(gone []domain.Link, fresh []domain.RemoteEntry, hash domain.HashKind) ([]Move, []domain.Notice) {
	gone, fresh = slices.Clone(gone), slices.Clone(fresh)
	slices.SortFunc(gone, func(x, y domain.Link) int {
		return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
	})
	slices.SortFunc(fresh, func(x, y domain.RemoteEntry) int {
		return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
	})
	links, files := map[string][]domain.Link{}, map[string][]domain.RemoteEntry{}
	for _, l := range gone {
		k := moveKey(l.MD5, l.SHA256, hash)
		links[k] = append(links[k], l)
	}
	for _, e := range fresh {
		if k := moveKey(e.MD5, e.SHA256, hash); k != "" {
			files[k] = append(files[k], e)
		}
	}
	var moves []Move
	var notices []domain.Notice
	for _, l := range gone {
		k := moveKey(l.MD5, l.SHA256, hash)
		ls, es := links[k], files[k]
		if len(es) == 0 {
			continue
		}
		switch i := slices.Index(ls, l); {
		case len(es) < len(ls) && slices.ContainsFunc(ls, func(o domain.Link) bool { return o.AssetID != l.AssetID }):
			notices = append(notices, domain.Notice{Code: "plan.ambiguous_move", Params: []string{string(l.Root), l.Path}})
		case i < len(es):
			moves = append(moves, Move{Link: l, To: es[i]})
		}
	}
	return moves, notices
}

func moveKey(md5, sha256 string, hash domain.HashKind) string {
	switch hash {
	case domain.HashMD5:
		return md5
	case domain.HashSHA256:
		return sha256
	}
	return ""
}
