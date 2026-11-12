package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// DecidePlacement returns the changes that make the files of asset in remote
// lie under the root of asset.TargetLibrary in the directories of its
// albums: one file in each event album, else one in its primary placement as
// layout.Primary gives it outside a first placement, and, while one of those
// directories holds a file of asset in any remote, one in each thematic
// album that holds a file of asset in any remote. asset is as the sync
// leaves it: Memberships are the albums it is in, its first placement
// applied, and TargetLibrary is the library whose root holds its files; it
// syncs and is not in Recently Deleted. albums are as layout.Primary takes
// them. links are the links of asset in every remote, each where its file
// lies now, with none for a file gone from the cloud; only the active links
// with the original or the source role count. A file of remote elsewhere
// moves to a directory without a file, else goes to quarantine; a directory
// still without one gets a copy of a file of remote, or an upload when remote
// has none. The changes are in the order of AlbumID, quarantines last.
func DecidePlacement(asset domain.Asset, links []domain.Link, remote string, albums []domain.Album) []domain.Placement {
	want, noEvent := objectAlbums(asset, links, albums)
	root := asset.TargetLibrary
	var files []domain.Link
	for _, l := range links {
		if l.Remote == remote && objectFile(l) {
			files = append(files, l)
		}
	}
	slices.SortFunc(files, func(x, y domain.Link) int {
		return cmp.Or(cmp.Compare(x.Root, y.Root), cmp.Compare(x.Path, y.Path))
	})
	placed := func(l domain.Link) bool { return l.Root == root && slices.Contains(want, l.AlbumID) }
	orphans := slices.DeleteFunc(slices.Clone(files), placed)
	var out []domain.Placement
	for _, id := range want {
		switch {
		case slices.ContainsFunc(files, func(l domain.Link) bool { return placed(l) && l.AlbumID == id }):
		case len(orphans) > 0:
			rule := objectMoveRule(orphans[0], root, noEvent)
			out = append(out, domain.Placement{Action: domain.ActionMoveFile, Link: orphans[0], AlbumID: id, Rule: rule})
			orphans = orphans[1:]
		case len(files) > 0:
			out = append(out, domain.Placement{Action: domain.ActionCopyFile, Link: files[0], AlbumID: id, Rule: "R-OBJECT-03"})
		default:
			out = append(out, domain.Placement{Action: domain.ActionUpload, AlbumID: id, Rule: "R-LINK-01"})
		}
	}
	for _, l := range orphans {
		out = append(out, domain.Placement{Action: domain.ActionQuarantineFile, Link: l, Rule: "R-OBJECT-04"})
	}
	return out
}

// objectAlbums returns, sorted, the albums whose directories hold a file of
// asset once the sync is done, zero standing for the day folder, and whether
// asset is in no event album.
func objectAlbums(asset domain.Asset, links []domain.Link, albums []domain.Album) (want []domain.AlbumID, noEvent bool) {
	var thematic []domain.AlbumID
	for _, a := range albums {
		member := !a.InTrash && slices.ContainsFunc(asset.Memberships, func(m domain.Membership) bool { return m.AlbumID == a.ID })
		switch kind := layout.Classify(a); {
		case member && kind == domain.KindEvent:
			want = append(want, a.ID)
		case member && kind == domain.KindThematic && objectLinked(links, a.ID):
			thematic = append(thematic, a.ID)
		}
	}
	if noEvent = len(want) == 0; noEvent {
		primary, _ := layout.Primary(asset, albums, false)
		want = []domain.AlbumID{primary}
	}
	// A file in a thematic album is a copy beside the file of an event or of
	// the primary placement; alone, it stood for the primary placement and
	// moves when that changes.
	if slices.ContainsFunc(want, func(id domain.AlbumID) bool { return objectLinked(links, id) }) {
		want = append(want, thematic...)
	}
	slices.Sort(want)
	return slices.Compact(want), noEvent
}

func objectLinked(links []domain.Link, album domain.AlbumID) bool {
	return slices.ContainsFunc(links, func(l domain.Link) bool { return objectFile(l) && l.AlbumID == album })
}

// The source that a converted asset came from stands for it in its remote;
// files of the other roles follow the file that stands for the asset, and a
// link out of sync waits for its status to change.
func objectFile(l domain.Link) bool {
	return (l.Role == domain.RoleOriginal || l.Role == domain.RoleSource) && l.Status == domain.StatusActive
}

func objectMoveRule(l domain.Link, root domain.LibraryKind, noEvent bool) string {
	switch {
	case l.Root != root:
		return "R-OBJECT-07"
	case noEvent:
		return "R-OBJECT-06"
	}
	return "R-OBJECT-05"
}

// DecideObject returns what the files of remote make of asset in iCloud: the
// decisions on its links there and the albums that it joins and leaves.
// asset is the asset of the snapshot reconciled with the state, outside
// Recently Deleted, and it syncs. decisions are those that DecideLink gave,
// with same false, for the links of asset in remote, each with Link as the
// last applied run left it; links and albums are as DecidePlacement takes
// them. A file of the original or the source role gone from remote while
// another stays there is forgotten, and asset leaves the album of its
// directory (R-OBJECT-02); with none left, the decisions stand (R-OBJECT-01).
// A moved file makes asset leave the album of its old directory and join
// that of its new one, under the rule of its decision; a conflict whose file
// lies where DecidePlacement keeps a file of asset is the same move on both
// sides, the file having moved to the place the asset moved to. asset leaves
// no album that still holds a file of it in remote, and the day folder is no
// album. Join and Leave may name albums that asset is already in or out of;
// they are in the order of album ID and then rule, without repeats, and
// Links in the order of link ID. Other decisions are as given.
func DecideObject(asset domain.Asset, decisions []domain.LinkDecision, links []domain.Link, remote string, albums []domain.Album) domain.ObjectDecision {
	want, _ := objectAlbums(asset, links, albums)
	files := slices.DeleteFunc(slices.Clone(links), func(l domain.Link) bool { return l.Remote != remote || !objectFile(l) })
	var out domain.ObjectDecision
	for _, d := range decisions {
		i := slices.IndexFunc(files, func(l domain.Link) bool { return l.ID == d.Link.ID })
		from, to := d.Link.AlbumID, d.Link.AlbumID
		switch {
		case !objectFile(d.Link), i < 0 && len(files) == 0:
		case i < 0:
			// A file that stays carries a change in iCloud to the new place
			// of the asset, so a gone copy is not uploaded again.
			d.Action, d.Rule, to = domain.ActionForget, "R-OBJECT-02", 0
		case d.Action == domain.ActionConflict && files[i].Root == asset.TargetLibrary && slices.Contains(want, files[i].AlbumID):
			same, _ := DecideLink(d.ICloud, d.Remote, true)
			d.Action, d.Rule = same.Action, same.Rule
		case d.Action == domain.ActionMoveAsset || d.Action == domain.ActionConflict:
			to = files[i].AlbumID
		}
		if from != to && from != 0 && !slices.ContainsFunc(files, func(l domain.Link) bool { return l.AlbumID == from }) {
			out.Leave = append(out.Leave, domain.AlbumChange{AlbumID: from, Rule: d.Rule})
		}
		if from != to && to != 0 {
			out.Join = append(out.Join, domain.AlbumChange{AlbumID: to, Rule: d.Rule})
		}
		out.Links = append(out.Links, d)
	}
	slices.SortFunc(out.Links, func(x, y domain.LinkDecision) int { return cmp.Compare(x.Link.ID, y.Link.ID) })
	out.Join, out.Leave = objectChanges(out.Join), objectChanges(out.Leave)
	return out
}

func objectChanges(changes []domain.AlbumChange) []domain.AlbumChange {
	slices.SortFunc(changes, func(x, y domain.AlbumChange) int {
		return cmp.Or(cmp.Compare(x.AlbumID, y.AlbumID), cmp.Compare(x.Rule, y.Rule))
	})
	return slices.Compact(changes)
}

// DecideFirstPlacement returns the albums that asset joins and leaves in
// iCloud at its first placement. asset is the asset of the snapshot
// reconciled with the state; it is new to every remote, with no link in any
// status in any of them, and IsNew holds for it. An asset in no album joins
// the only event of its day (R-LAYOUT-05); under closest_date_remove_others
// an asset leaves every event but the one closest to its date, closest first
// (R-LAYOUT-04). albums are as layout.Primary takes them.
func DecideFirstPlacement(asset domain.Asset, albums []domain.Album, multi domain.MultiAlbumPolicy) (join, leave []domain.AlbumChange) {
	primary, others := layout.Primary(asset, albums, true)
	if primary != 0 && !slices.ContainsFunc(asset.Memberships, func(m domain.Membership) bool { return m.AlbumID == primary }) {
		join = []domain.AlbumChange{{AlbumID: primary, Rule: "R-LAYOUT-05"}}
	}
	if multi == domain.MultiAlbumRemoveOthers {
		for _, id := range others {
			leave = append(leave, domain.AlbumChange{AlbumID: id, Rule: "R-LAYOUT-04"})
		}
	}
	return join, leave
}
