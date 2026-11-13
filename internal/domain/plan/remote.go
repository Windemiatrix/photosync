package plan

import (
	"cmp"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DecideRemoteLinks returns the decisions on the links of an asset in remote
// r and, as current, those links where their files lie now. now is the asset
// of the snapshot reconciled with the state, found reports that the snapshot
// holds it, and baseLibrary is its Library in the state. links are the links
// of the asset in the state, in any remote, role and status; only those of r
// count. files gives, by link ID, the entry of the scan of r where the file of
// a link lies now: at the path of the link, or where MovesByContent found it;
// a link that it lacks has no file. pairs are the album pairs of every remote
// that the run keeps or makes, and albums are as ICloudSide takes them.
//
// An asset that does not sync under p, or for which StatusFrozen holds in r,
// is frozen there: it gets no decisions, and current is its links in r as the
// state has them. Otherwise an active link of the original or the source role
// gets the decision of ModeDecide on the sides that ICloudSide and RemoteSide
// give, a file found elsewhere being moved, or of ModeKeepAlbums while
// p.AlbumsForPersonal is false and the file lies under the personal root both
// in the base and now; a quarantined link gets one on the sides of StatusSides
// when it reports ok. Each decision has same false and Link as the state has
// it; other links get none. A moved file lies in the album of the pair of r
// that holds its directory, or in none, except that a move which the base
// takes with ActionUpdateBase keeps the album of the base, so that the layout
// does not undo it. An active link without a file is not in current, and a
// quarantined link with a decision is there, active, only once its file is
// back. Any other link is in current as the state has it. Decisions and
// current are in the order of link ID, and the inputs are not changed.
func DecideRemoteLinks(now domain.Asset, found bool, baseLibrary domain.LibraryKind, links []domain.Link, r domain.RemoteSpec, files map[domain.LinkID]domain.RemoteEntry, pairs []domain.AlbumPair, albums []domain.Album, p domain.Policies) (decisions []domain.LinkDecision, current []domain.Link, frozen bool) {
	links = slices.DeleteFunc(slices.Clone(links), func(l domain.Link) bool { return l.Remote != r.Name })
	slices.SortFunc(links, func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) })
	if found && !Syncs(now, p) || StatusFrozen(links, r.Name) {
		return nil, links, true
	}
	for _, l := range links {
		e, there := files[l.ID]
		at := there && e.Root == l.Root && e.Path == l.Path
		file := RemoteSide(l, e, at, r.Capabilities.ContentHash)
		cur, kept, decide := l, true, objectFile(l)
		var icloud, remote domain.ChangeKind
		switch {
		case l.Status != domain.StatusActive:
			icloud, remote, decide = StatusSides(l.Status, found, now.InTrash, file)
			if decide {
				cur.Status, cur.QuarantinePath, kept = domain.StatusActive, "", remote == domain.ChangeNew
			}
		case there && !at:
			remote, cur.Root, cur.Path, cur.AlbumID = domain.ChangeMoved, e.Root, e.Path, 0
			if i := slices.IndexFunc(pairs, func(a domain.AlbumPair) bool {
				return a.Remote == r.Name && a.Root == e.Root && a.DirPath == path.Dir(e.Path)
			}); i >= 0 {
				cur.AlbumID = pairs[i].AlbumID
			}
		default:
			remote, kept = file, file != domain.ChangeDeleted
		}
		if decide {
			if l.Status == domain.StatusActive {
				icloud = ICloudSide(l, baseLibrary, now, found, albums)
			}
			d, _ := ModeDecide(r.Mode, icloud, remote, false)
			if !p.AlbumsForPersonal && l.Root == domain.LibraryPersonal && cur.Root == domain.LibraryPersonal {
				d, _ = ModeKeepAlbums(r.Mode, icloud, remote, false)
			}
			d.Link = l
			if d.Action == domain.ActionUpdateBase {
				cur.AlbumID = l.AlbumID
			}
			decisions = append(decisions, d)
		}
		if kept {
			current = append(current, cur)
		}
	}
	return decisions, current, false
}

// DecideRemoteObject returns o, what the files of remote r make of an asset
// that DecideRemoteLinks did not freeze there, with each conflict given to its
// winner; conflict, the conflict of the asset in r as the run leaves it; and
// whether o takes part in the plan, o being zero when it does not. now and
// found are as DecideRemoteLinks takes them, decisions are what it gives for r,
// and links are its current for every remote. DecideObject reduces decisions
// unless the asset is gone, in Recently Deleted or r is upload-only, where they
// stand. While p.AlbumsForPersonal is false, the asset joins and leaves no
// album unless links hold an active link of it in r with the original or the
// source role outside the personal root. c is the conflict that the state
// keeps for the asset in r, else one with only AssetID and Remote, and
// DecideConflict and SettleConflict decide it under p.Conflict. The deletes
// policy is not applied yet, and the inputs are not changed.
func DecideRemoteObject(now domain.Asset, found bool, decisions []domain.LinkDecision, links []domain.Link, r domain.RemoteSpec, albums []domain.Album, c domain.Conflict, p domain.Policies) (o domain.ObjectDecision, conflict domain.Conflict, in bool) {
	o = domain.ObjectDecision{Links: slices.Clone(decisions)}
	if found && !now.InTrash && r.Mode != domain.ModeUploadOnly {
		o = DecideObject(now, decisions, links, r.Name, albums)
	}
	if !p.AlbumsForPersonal && !slices.ContainsFunc(links, func(l domain.Link) bool {
		return l.Remote == r.Name && objectFile(l) && l.Root != domain.LibraryPersonal
	}) {
		o.Join, o.Leave = nil, nil
	}
	if conflict, in = DecideConflict(c, o.Links, p.Conflict); !in {
		return domain.ObjectDecision{}, conflict, false
	}
	return SettleConflict(o, conflict.Resolution, p.Conflict), conflict, true
}
