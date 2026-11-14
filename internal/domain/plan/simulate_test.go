package plan_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is a plan with the state, the photo library and the listings that
// it runs on, and the delta that the run writes first. The expected state,
// snapshot and listings follow what the delta, each record and each
// operation do once they succeed, not the code, and must not depend on the
// order of the input, but for that of the operations, which is the order of
// the run.
func TestSimulate(t *testing.T) {
	t.Parallel()
	const (
		personal, shared     = domain.LibraryPersonal, domain.LibraryShared
		original, live       = domain.RoleOriginal, domain.RoleLiveVideo
		holidaysDir, tripDir = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		quarantined          = ".photosync/quarantine/2026-11-13/"

		holidays, trip, party domain.AlbumID = 10, 11, 12
	)
	type (
		scans     = map[string][]domain.RemoteEntry
		links     = []domain.Link
		ops       = []domain.Op
		pairs     = []domain.AlbumPair
		conflicts = []domain.Conflict
	)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	settled := now.Add(-time.Hour)
	file := func(root domain.LibraryKind, at string, size int64, md5 string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: settled}
	}
	to := func(e domain.RemoteEntry, root domain.LibraryKind, at string) domain.RemoteEntry {
		e.Root, e.Path = root, at
		return e
	}
	link := func(id domain.LinkID, of domain.AssetID, remote string, e domain.RemoteEntry, role domain.LinkRole) domain.Link {
		return domain.Link{
			ID: id, AssetID: of, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: holidays,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	moved := func(l domain.Link, e domain.RemoteEntry, album domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID = e.Root, e.Path, album
		return l
	}
	withID := func(l domain.Link, id domain.LinkID) domain.Link {
		l.ID = id
		return l
	}
	op := func(kind domain.OpKind, of domain.AssetID, src, dst domain.RemoteEntry, rule string, ls ...domain.Link) domain.Op {
		return domain.Op{
			Kind: kind, Remote: "alpha", Root: src.Root, Src: src.Path, DstRoot: dst.Root, Dst: dst.Path, Asset: of, Rule: rule,
			Irreversible: kind == domain.OpDelete || kind == domain.OpExpire, Links: ls,
		}
	}
	put := func(l domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpPut, Remote: "alpha", DstRoot: l.Root, Dst: l.Path, Asset: l.AssetID, Rule: "R-LINK-01", Links: links{l}}
	}
	pair := func(album domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: album, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud}
	}
	conflict := func(of domain.AssetID, remote string, status domain.ConflictStatus, choice domain.ConflictPolicy) domain.Conflict {
		return domain.Conflict{AssetID: of, Remote: remote, Status: status, Resolution: choice}
	}

	e1, e2 := file(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1"), file(personal, holidaysDir+"/IMG_0002.HEIC", 200, "b2")
	ev, xmp := file(personal, holidaysDir+"/IMG_0001.MOV", 30, "v1"), file(personal, holidaysDir+"/IMG_0001.xmp", 5, "x1")
	edit := file(personal, holidaysDir+"/IMG_0002_edited.jpg", 10, "g2")
	aged := file(personal, ".photosync/quarantine/2026-08-14/"+holidaysDir+"/IMG_0005.HEIC", 10, "q5")
	folder, userFile := domain.RemoteEntry{Root: personal, Path: holidaysDir, IsDir: true, MTime: settled}, file(personal, tripDir+"/IMG_0001.HEIC", 50, "c1")
	shot := e1
	shot.Meta = domain.MediaMeta{Taken: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC), Width: 4032, Height: 3024}
	sharedFile, tripCopy := to(e1, shared, e1.Path), to(e1, personal, tripDir+"/IMG_0001.HEIC")
	tripPhoto, tripVideo, tripSidecar := to(shot, shared, tripDir+"/IMG_0001.HEIC"), to(ev, shared, tripDir+"/IMG_0001.mov"), to(xmp, shared, tripDir+"/IMG_0001.xmp")
	inQuarantine, editInQuarantine := to(e1, personal, quarantined+e1.Path), to(edit, personal, quarantined+edit.Path)
	photo, video, other := link(1, 7, "alpha", e1, original), link(2, 7, "alpha", ev, live), link(3, 7, "beta", e1, original)
	fresh, copied := link(4, 8, "alpha", e2, original), withID(moved(photo, tripCopy, trip), 0)
	fresh.Level = domain.LevelSameShot
	forgotten, removed, parked := video, photo, photo
	forgotten.Status, removed.Status = "", ""
	parked.Status, parked.QuarantinePath = domain.StatusQuarantinedRemote, inQuarantine.Path
	up := domain.Link{
		AssetID: 9, Remote: "alpha", Root: personal, Path: holidaysDir + "/IMG_0003.HEIC", Role: original, AlbumID: holidays,
		Size: 300, SHA256: "sc3", MD5: "c3", Level: domain.LevelUploaded, Status: domain.StatusActive,
	}
	upLive := up
	upLive.Path, upLive.Role, upLive.Size, upLive.SHA256, upLive.MD5 = holidaysDir+"/IMG_0003.mov", live, 40, "", ""
	upFile := domain.RemoteEntry{Root: personal, Path: up.Path, Size: 300, MD5: "c3", SHA256: "sc3", MTime: now}
	upLiveFile := domain.RemoteEntry{Root: personal, Path: upLive.Path, Size: 40, MTime: now}
	a7 := domain.Asset{
		ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"}, Library: personal, TargetLibrary: personal,
		OriginalName: "IMG_0001.HEIC", Memberships: []domain.Membership{{AlbumID: holidays, FirstSeenRun: "run-1"}},
	}
	a5, a8, toShared := a7, a7, a7
	a5.ID, a5.Key, a5.OriginalName, a5.Memberships = 5, domain.AssetKey{CloudAssetGUID: "guid-5", UUID: "uuid-5"}, "IMG_0005.HEIC", nil
	a8.ID, a8.Key, a8.OriginalName = 8, domain.AssetKey{UUID: "uuid-8"}, "IMG_0002.HEIC"
	toShared.TargetLibrary = shared
	hol := domain.Album{ID: holidays, UUID: "u-hol", Kind: domain.KindEvent, Title: "05/01 Holidays"}
	zoo, tripAlbum, renamed, partyAlbum := hol, hol, hol, hol
	zoo.ID, zoo.UUID, zoo.Title = 9, "u-zoo", "05/03 Zoo"
	tripAlbum.ID, tripAlbum.UUID, tripAlbum.Title = trip, "u-trip", "05/02 Trip"
	renamed.ID, renamed.UUID, renamed.Title = trip, "u-trip", "05/02 Trip by the sea"
	partyAlbum.ID, partyAlbum.UUID, partyAlbum.Title = party, "u-party", "05/04 Party"
	history := domain.AssetHistory{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-6", UUID: "uuid-6"}}
	historyGone := history
	historyGone.Deleted = true
	library := domain.LibrarySnapshot{
		Assets: []domain.Asset{{Key: a8.Key, OriginalName: a8.OriginalName}, {Key: a7.Key, OriginalName: a7.OriginalName}},
		Albums: []domain.Album{{UUID: renamed.UUID, Title: renamed.Title}, {UUID: hol.UUID, Title: hol.Title}},
	}
	ordered := domain.LibrarySnapshot{
		Assets: []domain.Asset{library.Assets[1], library.Assets[0]},
		Albums: []domain.Album{library.Albums[1], library.Albums[0]},
	}
	alphaHolidays, alphaShared := pair(holidays, "alpha", personal, holidaysDir), pair(holidays, "alpha", shared, holidaysDir)
	betaHolidays, alphaTrip := pair(holidays, "beta", personal, holidaysDir), pair(trip, "alpha", personal, tripDir)
	year := domain.Album{ID: 1, UUID: "u-2024", Key: `[] "2024"`, KeySource: domain.KeyPath, Kind: domain.KindFolder, Title: "2024"}
	cats := domain.Album{ID: 5, UUID: "u-cats", Kind: domain.KindThematic, Title: "Cats"}
	creating := func(id domain.AlbumID) domain.Op {
		return domain.Op{Kind: domain.OpAlbumCreate, AlbumID: id, Rule: "R-LAYOUT-06"}
	}
	albumOp := func(kind domain.OpKind, of domain.AssetID, id domain.AlbumID, rule string, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: kind, Asset: of, AlbumID: id, Rule: rule, Links: ls}
	}
	importing := func(of domain.AssetID, id domain.AlbumID, rule string, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: "alpha", Root: ls[0].Root, Src: ls[0].Path, Asset: of, AlbumID: id, Rule: rule, Links: ls}
	}
	imported := func(e domain.RemoteEntry, role domain.LinkRole, id domain.AlbumID) domain.Link {
		return domain.Link{
			Remote: "alpha", Root: e.Root, Path: e.Path, Role: role, AlbumID: id, Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag,
			MTime: e.MTime, Level: domain.LevelImported, Status: domain.StatusActive,
		}
	}
	owned := func(l domain.Link, id domain.LinkID, of domain.AssetID) domain.Link {
		l.ID, l.AssetID = id, of
		return l
	}
	member := func(a domain.Asset, run string, of ...domain.Album) domain.Asset {
		a.AlbumUUIDs, a.Memberships = nil, nil
		for _, x := range of {
			a.AlbumUUIDs, a.Memberships = append(a.AlbumUUIDs, x.UUID), append(a.Memberships, domain.Membership{AlbumID: x.ID, FirstSeenRun: run})
		}
		return a
	}
	seen := func(a domain.Asset) domain.Asset {
		a.ID, a.TargetLibrary, a.Memberships = 0, "", nil
		return a
	}
	binned := func(a domain.Asset) domain.Asset {
		a.InTrash = true
		return a
	}
	identity := func(of domain.AssetID, n int, deleted bool) domain.AssetHistory {
		return domain.AssetHistory{AssetID: of, Key: domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", n), UUID: fmt.Sprintf("uuid-%d", n)}, Deleted: deleted}
	}
	earlier := func(h domain.AssetHistory) domain.Asset { return domain.Asset{Key: h.Key, Library: personal} }
	offset := 7200
	newPhoto, newVideo := file(personal, holidaysDir+"/IMG_0003.HEIC", 300, "c3"), file(personal, holidaysDir+"/IMG_0003.mov", 40, "c4")
	newPhoto.Meta = domain.MediaMeta{Taken: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC), TZOffset: &offset, SubSec: "075", Width: 4032, Height: 3024}
	mpeg := file(shared, holidaysDir+"/IMG_0004.MPG", 500, "d4")
	mpegLink, relinked := link(6, 7, "alpha", mpeg, domain.RoleSource), owned(imported(mpeg, domain.RoleSource, 0), 6, 0)
	made9 := domain.Asset{
		ID: 9, Key: domain.AssetKey{UUID: "sim-run-2-1"}, Library: personal, TargetLibrary: personal, OriginalName: "IMG_0003.HEIC",
		Taken: newPhoto.Meta.Taken, TZOffset: &offset, SubSec: "075", Width: 4032, Height: 3024,
		Resources: []domain.Resource{
			{Role: domain.ResourceOriginal, Filename: "IMG_0003.HEIC", Size: 300, SHA256: "sc3", MD5: "c3"},
			{Role: domain.ResourceLiveVideo, Filename: "IMG_0003.mov", Size: 40, SHA256: "sc4", MD5: "c4"},
		},
		AlbumUUIDs: []string{"u-hol"}, Memberships: []domain.Membership{{AlbumID: holidays, FirstSeenRun: "run-2"}},
	}
	made10 := domain.Asset{
		ID: 10, Key: domain.AssetKey{UUID: "sim-run-2-2"}, Library: personal, TargetLibrary: shared, OriginalName: "IMG_0004.MPG",
		Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.MPG", Size: 500, SHA256: "sd4", MD5: "d4"}},
	}
	shot9, shot10 := seen(made9), seen(made10)
	shot9.Resources = []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0003.HEIC", Size: 300}, {Role: domain.ResourceLiveVideo, Filename: "IMG_0003.mov", Size: 40}}
	shot10.Resources = []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0004.MPG", Size: 500}}
	r5, r7 := member(a5, "run-1", zoo, hol), member(a7, "run-1", hol)
	changed5 := file(personal, holidaysDir+"/IMG_0005.HEIC", 150, "f5")
	changed5.Meta = domain.MediaMeta{Taken: time.Date(2024, 5, 1, 11, 0, 0, 0, time.UTC), Width: 3024, Height: 4032}
	link5 := link(9, 5, "alpha", file(personal, changed5.Path, 100, "e5"), original)
	renewed5 := link5
	renewed5.Size, renewed5.SHA256, renewed5.MD5, renewed5.ETag, renewed5.MTime, renewed5.Level = 150, "sf5", "f5", "ef5", settled, domain.LevelImported
	new5 := domain.Asset{
		ID: 5, Key: domain.AssetKey{UUID: "sim-run-2-0"}, Library: personal, TargetLibrary: personal, OriginalName: "IMG_0005.HEIC",
		Taken: changed5.Meta.Taken, Width: 3024, Height: 4032,
		Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0005.HEIC", Size: 150, SHA256: "sf5", MD5: "f5"}},
		AlbumUUIDs: []string{"u-zoo", "u-hol"}, Memberships: []domain.Membership{{AlbumID: zoo.ID, FirstSeenRun: "run-2"}, {AlbumID: holidays, FirstSeenRun: "run-2"}},
	}
	shot5 := seen(new5)
	shot5.Resources = []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0005.HEIC", Size: 150}}
	q5, q7, q8 := member(a5, "run-1", zoo), member(a7, "run-1", hol), member(a8, "run-1", zoo, hol)
	x5, x7, x8 := q5, q7, binned(member(a8, "run-1", hol))
	x5.AlbumUUIDs, x5.Memberships = []string{}, nil
	x7.AlbumUUIDs, x7.Memberships = []string{"u-hol", "u-zoo"}, []domain.Membership{{AlbumID: zoo.ID, FirstSeenRun: "run-2"}, {AlbumID: holidays, FirstSeenRun: "run-1"}}
	otherZoo := moved(other, to(e1, personal, "2024/05-03 - Zoo/IMG_0001.HEIC"), zoo.ID)
	reversed := func(in plan.Input, p domain.Plan, d domain.SnapshotDelta) (plan.Input, domain.Plan, domain.SnapshotDelta) {
		// The operations keep the order of the run, and the history of the
		// delta the order that Build gives it.
		s, snap, listed := in.State, in.Snapshot, scans{}
		s.Assets, s.Albums, s.AlbumPairs = slices.Clone(s.Assets), slices.Clone(s.Albums), slices.Clone(s.AlbumPairs)
		s.Links, s.History, s.Conflicts = slices.Clone(s.Links), slices.Clone(s.History), slices.Clone(s.Conflicts)
		snap.Assets, snap.Albums = slices.Clone(snap.Assets), slices.Clone(snap.Albums)
		p.Links, p.Pairs, p.Conflicts, p.Albums = slices.Clone(p.Links), slices.Clone(p.Pairs), slices.Clone(p.Conflicts), slices.Clone(p.Albums)
		d.Assets, d.Albums = slices.Clone(d.Assets), slices.Clone(d.Albums)
		slices.Reverse(s.Assets)
		slices.Reverse(s.Albums)
		slices.Reverse(s.AlbumPairs)
		slices.Reverse(s.Links)
		slices.Reverse(s.History)
		slices.Reverse(s.Conflicts)
		slices.Reverse(snap.Assets)
		slices.Reverse(snap.Albums)
		slices.Reverse(p.Links)
		slices.Reverse(p.Pairs)
		slices.Reverse(p.Conflicts)
		slices.Reverse(p.Albums)
		slices.Reverse(d.Assets)
		slices.Reverse(d.Albums)
		for name, es := range in.Scans {
			listed[name] = slices.Clone(es)
			slices.Reverse(listed[name])
		}
		in.State, in.Snapshot, in.Scans = s, snap, listed
		return in, p, d
	}

	for _, c := range []struct {
		name          string
		state         domain.State
		snapshot      domain.LibrarySnapshot
		scans         scans
		p             domain.Plan
		delta         domain.SnapshotDelta
		want          domain.State
		snapshotAfter domain.LibrarySnapshot
		scansAfter    scans
	}{
		{"nothing at all", domain.State{}, domain.LibrarySnapshot{}, nil, domain.Plan{}, domain.SnapshotDelta{}, domain.State{}, domain.LibrarySnapshot{}, scans{}},
		{"the state takes the delta, the records and the pairs: a row of the delta replaces that of the state with its ID, one that it lacks stays, and a record with no status takes its link out",
			domain.State{
				Assets: []domain.Asset{a7, a5}, Albums: []domain.Album{hol, tripAlbum, zoo}, AlbumPairs: pairs{betaHolidays, alphaShared},
				Links: links{photo, video, other}, History: []domain.AssetHistory{history},
			},
			library, scans{"alpha": {sharedFile, e2}, "beta": {e1}},
			domain.Plan{Links: links{moved(photo, sharedFile, holidays), forgotten, fresh}, Pairs: pairs{alphaHolidays, alphaTrip}},
			domain.SnapshotDelta{Assets: []domain.Asset{toShared, a8}, Albums: []domain.Album{hol, renamed, partyAlbum}, History: []domain.AssetHistory{historyGone}},
			domain.State{
				Assets: []domain.Asset{a5, toShared, a8}, Albums: []domain.Album{zoo, hol, renamed, partyAlbum},
				AlbumPairs: pairs{alphaHolidays, alphaShared, betaHolidays, alphaTrip},
				Links:      links{moved(photo, sharedFile, holidays), other, fresh}, History: []domain.AssetHistory{historyGone},
			},
			ordered, scans{"alpha": {e2, sharedFile}, "beta": {e1}}},
		{"a file moves with its link to another root and folder, its Live Photo video and its sidecar go along, and its folder, a file of the user and a file of another remote stay",
			domain.State{Links: links{photo, video, other}}, domain.LibrarySnapshot{}, scans{"alpha": {xmp, userFile, shot, folder, ev}, "beta": {e1}},
			domain.Plan{Links: links{photo}, Ops: ops{
				op(domain.OpMove, 7, shot, tripPhoto, "R-OBJECT-07", moved(photo, tripPhoto, trip)),
				op(domain.OpMove, 7, ev, tripVideo, "R-COMPONENT-04", moved(video, tripVideo, trip)),
				op(domain.OpMove, 7, xmp, tripSidecar, "R-COMPONENT-04"),
			}},
			domain.SnapshotDelta{},
			domain.State{Links: links{moved(photo, tripPhoto, trip), moved(video, tripVideo, trip), other}},
			domain.LibrarySnapshot{}, scans{"alpha": {folder, userFile, tripPhoto, tripVideo, tripSidecar}, "beta": {e1}}},
		{"a copy and two puts make new links with the IDs after the highest of the state and the plan, in the order of the operations, and a put writes its file with the size and hashes of its link at the time of the run",
			domain.State{Links: links{photo}}, domain.LibrarySnapshot{}, scans{"alpha": {e2, e1}},
			domain.Plan{Ops: ops{op(domain.OpCopy, 7, e1, tripCopy, "R-OBJECT-03", copied), put(up), put(upLive)}, Links: links{fresh}},
			domain.SnapshotDelta{},
			domain.State{Links: links{photo, fresh, withID(copied, 5), withID(up, 6), withID(upLive, 7)}},
			domain.LibrarySnapshot{}, scans{"alpha": {e1, e2, upFile, upLiveFile, tripCopy}}},
		{"a file goes to quarantine with its link, which keeps its path, and an edit goes there without one",
			domain.State{Links: links{photo}}, domain.LibrarySnapshot{}, scans{"alpha": {edit, e1}},
			domain.Plan{Ops: ops{
				op(domain.OpQuarantine, 7, e1, inQuarantine, "R-STATUS-01", parked),
				op(domain.OpQuarantine, 0, edit, editInQuarantine, "R-STATUS-10"),
			}},
			domain.SnapshotDelta{},
			domain.State{Links: links{parked}},
			domain.LibrarySnapshot{}, scans{"alpha": {inQuarantine, editInQuarantine}}},
		{"a delete takes a file out of the cloud with its link and a sidecar without one, an expire a file of the quarantine, and the other files stay",
			domain.State{Links: links{photo, video}}, domain.LibrarySnapshot{}, scans{"alpha": {aged, xmp, ev, e1}},
			domain.Plan{Ops: ops{
				op(domain.OpDelete, 7, e1, domain.RemoteEntry{}, "R-STATUS-06", removed),
				op(domain.OpDelete, 7, xmp, domain.RemoteEntry{}, "R-STATUS-06"),
				op(domain.OpExpire, 0, aged, domain.RemoteEntry{}, "R-STATUS-09"),
			}},
			domain.SnapshotDelta{},
			domain.State{Links: links{video}},
			domain.LibrarySnapshot{}, scans{"alpha": {ev}}},
		{"the conflicts of the plan take the place of those of the state in their remote, a resolved one leaves, and the others of the state stay",
			domain.State{Conflicts: conflicts{
				conflict(7, "beta", domain.ConflictStatusSkipped, ""), conflict(5, "alpha", domain.ConflictStatusOpen, ""),
				conflict(7, "alpha", domain.ConflictStatusOpen, ""), conflict(3, "alpha", domain.ConflictStatusOpen, domain.ConflictICloudWins),
			}},
			domain.LibrarySnapshot{}, nil,
			domain.Plan{Conflicts: conflicts{
				conflict(3, "alpha", domain.ConflictStatusResolved, domain.ConflictICloudWins), conflict(5, "alpha", domain.ConflictStatusSkipped, ""),
				conflict(9, "alpha", domain.ConflictStatusOpen, ""),
			}},
			domain.SnapshotDelta{},
			domain.State{Conflicts: conflicts{
				conflict(5, "alpha", domain.ConflictStatusSkipped, ""), conflict(7, "alpha", domain.ConflictStatusOpen, ""),
				conflict(7, "beta", domain.ConflictStatusSkipped, ""), conflict(9, "alpha", domain.ConflictStatusOpen, ""),
			}},
			domain.LibrarySnapshot{}, scans{}},
		{"an album and a folder that the run adds come to the state with the UUID of their operation, their key and the UUID of their folder, new or not, and to the snapshot as Photos gives them",
			domain.State{}, domain.LibrarySnapshot{Albums: []domain.Album{{UUID: "u-2024", Kind: domain.KindFolder, Title: "2024"}}}, nil,
			domain.Plan{
				Ops: ops{creating(13), creating(14), creating(15)},
				Albums: []domain.Album{
					{ID: 15, Kind: domain.KindEvent, Title: "05/03 Zoo", ParentID: year.ID, FolderTitles: []string{"2024"}},
					{ID: 13, Kind: domain.KindFolder, Title: "2025"},
					{ID: 14, Kind: domain.KindEvent, Title: "06/01 Fair", ParentID: 13, FolderTitles: []string{"2025"}},
				},
				Pairs: pairs{pair(14, "alpha", personal, "2025/06-01 - Fair")},
			},
			domain.SnapshotDelta{Albums: []domain.Album{year}},
			domain.State{
				Albums: []domain.Album{
					year,
					{ID: 13, UUID: "sim-run-2-0", Key: `[] "2025"`, KeySource: domain.KeyPath, Kind: domain.KindFolder, Title: "2025"},
					{
						ID: 14, UUID: "sim-run-2-1", Key: `["2025"] "06/01 Fair"`, KeySource: domain.KeyPath, Kind: domain.KindEvent, Title: "06/01 Fair",
						ParentID: 13, ParentUUID: "sim-run-2-0", FolderTitles: []string{"2025"},
					},
					{
						ID: 15, UUID: "sim-run-2-2", Key: `["2024"] "05/03 Zoo"`, KeySource: domain.KeyPath, Kind: domain.KindEvent, Title: "05/03 Zoo",
						ParentID: year.ID, ParentUUID: "u-2024", FolderTitles: []string{"2024"},
					},
				},
				AlbumPairs: pairs{pair(14, "alpha", personal, "2025/06-01 - Fair")},
			},
			domain.LibrarySnapshot{Albums: []domain.Album{
				{UUID: "sim-run-2-0", Kind: domain.KindFolder, Title: "2025"},
				{UUID: "sim-run-2-1", Title: "06/01 Fair", ParentUUID: "sim-run-2-0", FolderTitles: []string{"2025"}},
				{UUID: "sim-run-2-2", Title: "05/03 Zoo", ParentUUID: "u-2024", FolderTitles: []string{"2024"}},
				{UUID: "u-2024", Kind: domain.KindFolder, Title: "2024"},
			}},
			scans{}},
		{"an import makes a new asset in the snapshot from its file and in the state under the next asset ID, which its links take, in the order of the operations, and the import by a link of a file of the shared root in the source role makes one that waits to move there",
			domain.State{Assets: []domain.Asset{a7}, Links: links{mpegLink, photo}},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{{Key: a8.Key, OriginalName: a8.OriginalName}, {Key: a7.Key, OriginalName: a7.OriginalName}},
				Albums: []domain.Album{{UUID: hol.UUID, Title: hol.Title}},
			},
			scans{"alpha": {mpeg, newVideo, edit, e1, newPhoto}},
			domain.Plan{Ops: ops{
				op(domain.OpQuarantine, 0, edit, editInQuarantine, "R-STATUS-10"),
				importing(0, holidays, "R-LINK-02", imported(newPhoto, original, holidays), imported(newVideo, live, holidays)),
				importing(0, 0, "R-LINK-11", relinked),
			}},
			domain.SnapshotDelta{Assets: []domain.Asset{a8, a7}, Albums: []domain.Album{hol}},
			domain.State{
				Assets: []domain.Asset{a7, a8, made9, made10}, Albums: []domain.Album{hol},
				Links: links{photo, owned(relinked, 6, 10), owned(imported(newPhoto, original, holidays), 7, 9), owned(imported(newVideo, live, holidays), 8, 9)},
			},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{{Key: a7.Key, OriginalName: a7.OriginalName}, shot9, shot10, {Key: a8.Key, OriginalName: a8.OriginalName}},
				Albums: []domain.Album{{UUID: hol.UUID, Title: hol.Title}},
			},
			scans{"alpha": {editInQuarantine, e1, newPhoto, newVideo, mpeg}}},
		{"an import with an asset replaces it: the asset keeps its ID under the new key, whose albums and delete come after, and its old key goes to History; the delete takes the old asset and an earlier identity to Recently Deleted and leaves the new asset",
			domain.State{Assets: []domain.Asset{a5, a7}, Links: links{link5}},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{seen(r7), earlier(identity(5, 6, false)), seen(r5), earlier(identity(7, 1, false))},
				Albums: []domain.Album{{UUID: zoo.UUID, Title: zoo.Title}, {UUID: hol.UUID, Title: hol.Title}},
			},
			scans{"alpha": {changed5}},
			domain.Plan{Ops: ops{
				importing(5, 0, "R-LINK-06", renewed5),
				albumOp(domain.OpAlbumAdd, 5, zoo.ID, "R-LINK-06"), albumOp(domain.OpAlbumAdd, 5, holidays, "R-LINK-06"),
				albumOp(domain.OpAssetDelete, 5, 0, "R-STATUS-08"),
			}},
			domain.SnapshotDelta{
				Assets: []domain.Asset{r7, r5}, Albums: []domain.Album{hol, zoo},
				History: []domain.AssetHistory{identity(5, 2, true), identity(5, 6, false), identity(7, 1, false)},
			},
			domain.State{
				Assets: []domain.Asset{new5, r7}, Albums: []domain.Album{zoo, hol}, Links: links{renewed5},
				History: []domain.AssetHistory{identity(5, 2, true), identity(5, 5, true), identity(5, 6, true), identity(7, 1, false)},
			},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{earlier(identity(7, 1, false)), binned(seen(r5)), binned(earlier(identity(5, 6, false))), seen(r7), shot5},
				Albums: []domain.Album{{UUID: hol.UUID, Title: hol.Title}, {UUID: zoo.UUID, Title: zoo.Title}},
			},
			scans{"alpha": {changed5}}},
		{"an album operation puts its asset into its album or takes it out in the state and the snapshot, and does nothing where the asset is in already or not in it, and a delete takes its asset and the earlier identities to Recently Deleted, the identities alone under R-STATUS-08",
			domain.State{Links: links{other}},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{seen(q8), earlier(identity(5, 4, false)), seen(q5), earlier(identity(8, 3, false)), seen(q7)},
				Albums: []domain.Album{{UUID: zoo.UUID, Title: zoo.Title}, {UUID: cats.UUID, Title: cats.Title}, {UUID: hol.UUID, Title: hol.Title}},
			},
			nil,
			domain.Plan{Ops: ops{
				albumOp(domain.OpAlbumAdd, 7, zoo.ID, "R-LINK-05", otherZoo), albumOp(domain.OpAlbumAdd, 7, holidays, "R-LINK-05"),
				albumOp(domain.OpAlbumRemove, 5, zoo.ID, "R-LINK-05"), albumOp(domain.OpAlbumRemove, 7, cats.ID, "R-LINK-05"),
				albumOp(domain.OpAlbumRemove, 8, zoo.ID, "R-LINK-05"),
				albumOp(domain.OpAssetDelete, 5, 0, "R-STATUS-08"), albumOp(domain.OpAssetDelete, 8, 0, "R-STATUS-02"),
			}},
			domain.SnapshotDelta{
				Assets: []domain.Asset{q8, q5, q7}, Albums: []domain.Album{hol, cats, zoo},
				History: []domain.AssetHistory{identity(5, 4, false), identity(8, 3, false)},
			},
			domain.State{
				Assets: []domain.Asset{x5, x7, x8}, Albums: []domain.Album{cats, zoo, hol}, Links: links{otherZoo},
				History: []domain.AssetHistory{identity(5, 4, true), identity(8, 3, true)},
			},
			domain.LibrarySnapshot{
				Assets: []domain.Asset{binned(earlier(identity(8, 3, false))), binned(earlier(identity(5, 4, false))), seen(x5), seen(x7), seen(x8)},
				Albums: []domain.Album{{UUID: cats.UUID, Title: cats.Title}, {UUID: hol.UUID, Title: hol.Title}, {UUID: zoo.UUID, Title: zoo.Title}},
			},
			scans{}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			in := plan.Input{State: c.state, Snapshot: c.snapshot, Scans: c.scans, Now: now, RunID: "run-2"}
			show := func() string { return fmt.Sprintf("%+v\n%+v\n%+v", in, c.p, c.delta) }
			input := show()
			s, snap, listed := plan.Simulate(in, c.p, c.delta)
			if !reflect.DeepEqual(s, c.want) || !reflect.DeepEqual(snap, c.snapshotAfter) || !reflect.DeepEqual(listed, c.scansAfter) {
				t.Errorf("Simulate =\n%+v\n%+v\n%#v\nwant\n%+v\n%+v\n%#v", s, snap, listed, c.want, c.snapshotAfter, c.scansAfter)
			}
			again, snapAgain, listedAgain := plan.Simulate(reversed(in, c.p, c.delta))
			if !reflect.DeepEqual(again, s) || !reflect.DeepEqual(snapAgain, snap) || !reflect.DeepEqual(listedAgain, listed) {
				t.Errorf("with the input reversed:\n%+v\n%+v\n%#v\nwant\n%+v\n%+v\n%#v", again, snapAgain, listedAgain, s, snap, listed)
			}
			for i := range s.Assets {
				s.Assets[i].OriginalName = "changed"
			}
			for i := range s.Albums {
				s.Albums[i].Title = "changed"
			}
			for i := range s.AlbumPairs {
				s.AlbumPairs[i].DirPath = "changed"
			}
			for i := range s.Links {
				s.Links[i].Path = "changed"
			}
			for i := range s.History {
				s.History[i].Deleted = !s.History[i].Deleted
			}
			for i := range s.Conflicts {
				s.Conflicts[i].Remote = "changed"
			}
			for i := range snap.Assets {
				snap.Assets[i].OriginalName = "changed"
			}
			for i := range snap.Albums {
				snap.Albums[i].Title = "changed"
			}
			for _, es := range listed {
				for i := range es {
					es[i].Path = "changed"
				}
			}
			if after := show(); after != input {
				t.Errorf("Simulate changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}

// Each row is the world of a run, as in TestBuild. Once every operation of
// its plan succeeds, a run an hour later, when what the first wrote has
// settled, finds nothing to do: no operation, record, album or pair, only
// the conflicts that the first left open, and its plan, applied, changes
// nothing in the state, the snapshot or the listings. Each row names the
// operations of the first plan, so that no row passes by planning nothing.
func TestSimulateConverges(t *testing.T) {
	t.Parallel()
	const (
		personal, shared     = domain.LibraryPersonal, domain.LibraryShared
		original, live       = domain.RoleOriginal, domain.RoleLiveVideo
		holidaysDir, tripDir = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		dayDir, catsDir      = "2024/05-01", "Albums/Cats"
		aged                 = ".photosync/quarantine/2026-08-14/"

		year, cats, holidays, trip domain.AlbumID = 1, 5, 10, 11
	)
	type (
		assets = []domain.Asset
		pairs  = []domain.AlbumPair
		scans  = map[string][]domain.RemoteEntry
		specs  = []domain.RemoteSpec
		kinds  = []domain.OpKind
	)
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	settled, young := now.Add(-time.Hour), now.Add(-5*time.Minute)
	may1, may2 := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC), time.Date(2024, 5, 2, 12, 0, 0, 0, time.UTC)
	inYear := func(id domain.AlbumID, uuid, title string, start time.Time) domain.Album {
		return domain.Album{
			ID: id, UUID: uuid, Key: `["2024"] "` + title + `"`, KeySource: domain.KeyPath, Kind: domain.KindEvent, Title: title,
			Start: start, ParentID: year, ParentUUID: "u-2024", FolderTitles: []string{"2024"},
		}
	}
	albums := []domain.Album{
		{ID: year, UUID: "u-2024", Key: `[] "2024"`, KeySource: domain.KeyPath, Kind: domain.KindFolder, Title: "2024"},
		{ID: cats, UUID: "u-cats", Key: `[] "Cats"`, KeySource: domain.KeyPath, Kind: domain.KindThematic, Title: "Cats"},
		inYear(holidays, "u-hol", "05/01 Holidays", may1.Add(-2*time.Hour)),
		inYear(trip, "u-trip", "05/02 Trip", may2.Add(-2*time.Hour)),
	}
	var library []domain.Album
	for _, a := range albums {
		a.ID, a.Key, a.KeySource, a.ParentID = 0, "", "", 0
		if a.Kind != domain.KindFolder {
			a.Kind = ""
		}
		library = append(library, a)
	}
	uuidOf := map[domain.AlbumID]string{cats: "u-cats", holidays: "u-hol", trip: "u-trip"}
	asset := func(id domain.AssetID, name string, size int64, taken time.Time, run string, in ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: id, Key: domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", id), UUID: fmt.Sprintf("uuid-%d", id)},
			Library: personal, TargetLibrary: personal, OriginalName: name, Taken: taken, Width: 4032, Height: 3024,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		}
		for _, album := range in {
			a.AlbumUUIDs = append(a.AlbumUUIDs, uuidOf[album])
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: album, FirstSeenRun: run})
		}
		return a
	}
	withVideo := func(a domain.Asset) domain.Asset {
		a.Resources = append(slices.Clone(a.Resources), domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 30})
		return a
	}
	snapshot := func(as ...domain.Asset) domain.LibrarySnapshot {
		s := domain.LibrarySnapshot{Albums: library}
		for _, a := range as {
			a.ID, a.TargetLibrary, a.Memberships = 0, "", nil
			s.Assets = append(s.Assets, a)
		}
		return s
	}
	file := func(root domain.LibraryKind, at string, size int64, md5 string, mtime time.Time) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: mtime}
	}
	link := func(id domain.LinkID, remote string, e domain.RemoteEntry, role domain.LinkRole) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: holidays,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	pair := func(album domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: album, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud}
	}
	state := func(as assets, ps pairs, ls ...domain.Link) domain.State {
		return domain.State{Assets: as, Albums: albums, AlbumPairs: ps, Links: ls}
	}
	spec := func(name string, mode domain.Mode) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}
	}
	alpha, beta, alphaUp := spec("alpha", domain.ModeTwoWay), spec("beta", domain.ModeTwoWay), spec("alpha", domain.ModeUploadOnly)
	p := domain.DefaultPolicies()
	p.Deletes, p.UntitledEvent, p.AlbumsRoot = domain.DeletesQuarantine, "Untitled", "Albums"
	propagate, ask, edits := p, p, p
	propagate.Deletes, ask.Conflict, edits.EditedFiles = domain.DeletesPropagate, domain.ConflictAsk, domain.EditedQuarantine
	e1, ev := file(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, holidaysDir+"/IMG_0001.MOV", 30, "v1", settled)
	xmp, replaced := file(personal, holidaysDir+"/IMG_0001.xmp", 5, "x1", settled), file(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1", settled)
	dayFile, catsFile := file(personal, dayDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, catsDir+"/IMG_0001.HEIC", 100, "a1", settled)
	sharedFile, youngBeta := file(shared, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, tripDir+"/IMG_0002.HEIC", 50, "y2", young)
	aged5 := file(personal, aged+holidaysDir+"/IMG_0005.HEIC", 10, "q5", settled)
	kept6 := file(personal, ".photosync/quarantine/2026-08-15/"+holidaysDir+"/IMG_0006.HEIC", 10, "q6", settled)
	edit3 := file(personal, holidaysDir+"/IMG_0003_edited.jpg", 10, "g4", settled)
	photo, video, other := link(1, "alpha", e1, original), link(2, "alpha", ev, live), link(2, "beta", e1, original)
	inCats := photo
	inCats.Path, inCats.AlbumID = catsFile.Path, cats
	a7, in7Trip := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", holidays), asset(7, "IMG_0001.HEIC", 100, may1, "run-2", trip)
	n8 := withVideo(asset(8, "IMG_0001.HEIC", 200, may1.Add(time.Hour), "run-2", holidays))
	catsHolidays := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", cats, holidays)
	alphaHolidays, alphaTrip := pair(holidays, "alpha", personal, holidaysDir), pair(trip, "alpha", personal, tripDir)
	alphaCats, alphaShared := pair(cats, "alpha", personal, catsDir), pair(holidays, "alpha", shared, holidaysDir)
	betaHolidays := pair(holidays, "beta", personal, holidaysDir)
	conflict := func(remote string, status domain.ConflictStatus) domain.Conflict {
		return domain.Conflict{AssetID: 7, Remote: remote, Status: status}
	}
	recorded := func(s domain.State, cs ...domain.Conflict) domain.State {
		s.Conflicts = cs
		return s
	}
	remembered := func(s domain.State, hs ...domain.AssetHistory) domain.State {
		s.History = hs
		return s
	}
	partyDir, quarantined := "2024/05-04 - Party", ".photosync/quarantine/2026-11-13/"
	e2, e4 := file(personal, holidaysDir+"/IMG_0002.HEIC", 200, "b2", settled), file(personal, partyDir+"/IMG_0004.HEIC", 400, "d4", settled)
	e2.Meta = domain.MediaMeta{Taken: may1, Width: 4032, Height: 3024}
	tripFile, partyFile := file(personal, tripDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, partyDir+"/IMG_0001.HEIC", 100, "a1", settled)
	changedMov := file(personal, holidaysDir+"/IMG_0001.MOV", 35, "v2", settled)
	back, awayVideo := photo, video
	back.Status, back.QuarantinePath = domain.StatusQuarantinedRemote, quarantined+e1.Path
	awayVideo.Status, awayVideo.QuarantinePath = domain.StatusQuarantinedRemote, quarantined+ev.Path
	trashed, m8 := asset(8, "IMG_0002.HEIC", 200, may1, "run-2"), asset(8, "IMG_0002.HEIC", 200, may1, "run-2")
	trashed.InTrash = true
	n1, n2 := asset(1, "IMG_0001.HEIC", 100, may2, "run-2"), asset(2, "IMG_0001.HEIC", 200, may2, "run-2", trip)
	restored, restored5 := asset(6, "IMG_0006.HEIC", 100, may1, "run-2"), asset(5, "IMG_0005.HEIC", 100, may1, "run-2")
	history, history5 := domain.AssetHistory{AssetID: 7, Key: restored.Key}, domain.AssetHistory{AssetID: 7, Key: restored5.Key}
	fieldsOf := func(s domain.LibrarySnapshot) func(domain.AssetKey) template.Photo {
		return func(k domain.AssetKey) template.Photo {
			for _, a := range s.Assets {
				if a.Key == k {
					return templateFields{"original_name": {Strings: []string{a.OriginalName}}}
				}
			}
			return templateFields{}
		}
	}

	for _, c := range []struct {
		name      string
		state     domain.State
		snapshot  domain.LibrarySnapshot
		scans     scans
		remotes   specs
		only      string
		p         domain.Policies
		kinds     kinds
		conflicts []domain.Conflict
	}{
		{"a new asset goes up into its event with its Live Photo video",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, n8), scans{"alpha": {e1}}, specs{alpha}, "", p,
			kinds{domain.OpPut, domain.OpPut}, nil},
		{"an asset that moves to a new event in two remotes takes its files there, makes a pair in each and closes its recorded conflicts",
			recorded(state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), conflict("beta", domain.ConflictStatusSkipped), conflict("alpha", domain.ConflictStatusSkipped)),
			snapshot(in7Trip), scans{"alpha": {e1}, "beta": {e1}}, specs{beta, alpha}, "", p,
			kinds{domain.OpMove, domain.OpMove}, nil},
		{"under propagate an asset gone from the library takes its file, its Live Photo video and its sidecar out of the cloud",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", propagate,
			kinds{domain.OpDelete, domain.OpDelete, domain.OpDelete}, nil},
		{"under quarantine an asset gone from the library takes its file, its Live Photo video and its sidecar to quarantine, where they wait",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", p,
			kinds{domain.OpQuarantine, domain.OpQuarantine, domain.OpQuarantine}, nil},
		{"a file kept in quarantine longer than the policy says leaves the cloud, and one kept as long stays",
			state(nil, nil), snapshot(), scans{"alpha": {kept6, aged5}}, specs{alpha}, "", p, kinds{domain.OpExpire}, nil},
		{"upload-only: a file that the user replaced in place gives its link the new content",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {replaced}}, specs{alphaUp}, "", p, nil, nil},
		{"a photo that the plan moves to its new event takes its Live Photo video and its sidecar along",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(in7Trip)),
			scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", p, kinds{domain.OpMove, domain.OpMove, domain.OpMove}, nil},
		{"a conflict under ask stays open, and the asset stays as the state has it",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip), scans{"alpha": {dayFile}}, specs{alpha}, "", ask,
			nil, []domain.Conflict{conflict("alpha", domain.ConflictStatusOpen)}},
		{"a file that the user moved to the shared root takes the asset to the shared library in the state, and the file of another remote follows",
			state(assets{a7}, pairs{alphaHolidays, alphaShared, betaHolidays}, photo, other), snapshot(a7),
			scans{"alpha": {sharedFile}, "beta": {e1}}, specs{alpha, beta}, "", p, kinds{domain.OpMove}, nil},
		{"an edit goes to quarantine",
			state(nil, nil), snapshot(), scans{"alpha": {edit3}}, specs{alpha}, "", edits, kinds{domain.OpQuarantine}, nil},
		{"one remote alone syncs: a copy goes into the folder of an event of the asset, and the listing of the other remote stays as it is",
			state(assets{catsHolidays}, pairs{alphaHolidays, alphaCats, betaHolidays}, inCats, other), snapshot(catsHolidays),
			scans{"alpha": {catsFile}, "beta": {youngBeta, e1}}, specs{alpha, beta}, "alpha", p, kinds{domain.OpCopy}, nil},
		{"a file that the user moved takes the asset into the album of its new folder, and a new asset in Recently Deleted goes nowhere",
			remembered(state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), history), snapshot(a7, trashed), scans{"alpha": {tripFile}}, specs{alpha}, "", p,
			kinds{domain.OpAlbumAdd, domain.OpAlbumRemove}, nil},
		{"a file that the user moved out of its event into the day folder takes the asset out of the album",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {dayFile}}, specs{alpha}, "", p, kinds{domain.OpAlbumRemove}, nil},
		{"matching links a new asset with a new file of a paired folder, and the asset joins its album",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, m8), scans{"alpha": {e2, e1}}, specs{alpha}, "", p, kinds{domain.OpAlbumAdd}, nil},
		{"a file that the user moved into a new folder beside a new file: the album comes to iCloud, then the import and the asset join it",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(a7), scans{"alpha": {e4, partyFile}}, specs{alpha}, "", p,
			kinds{domain.OpAlbumCreate, domain.OpImport, domain.OpAlbumAdd, domain.OpAlbumRemove}, nil},
		{"the first placement takes a new asset into the only event of its day, where two new assets go up",
			state(nil, pairs{alphaHolidays}), snapshot(n2, n1), nil, specs{alpha}, "", p, kinds{domain.OpPut, domain.OpPut, domain.OpAlbumAdd}, nil},
		{"a file that the user brings back from quarantine for an asset gone from the library comes back to iCloud as a new asset under its link",
			state(assets{a7}, pairs{alphaHolidays}, back), snapshot(), scans{"alpha": {e1}}, specs{alpha}, "", p, kinds{domain.OpImport}, nil},
		{"a photo and its Live Photo video that the user changed while their asset is gone from the library come back to iCloud as one new asset",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {changedMov, replaced}}, specs{alpha}, "", p,
			kinds{domain.OpImport}, nil},
		{"a photo that the user changed in place replaces its asset, and the old asset and that of an earlier replacement go to Recently Deleted",
			remembered(state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, awayVideo), history), snapshot(withVideo(a7), restored),
			scans{"alpha": {ev, replaced}}, specs{alpha}, "", p, kinds{domain.OpImport, domain.OpAlbumAdd, domain.OpAssetDelete}, nil},
		{"a photo that the user changed while its asset moved to another event goes there with its Live Photo video, and the asset is replaced from their new place",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(in7Trip)), scans{"alpha": {ev, replaced}}, specs{alpha}, "", p,
			kinds{domain.OpMove, domain.OpMove, domain.OpImport, domain.OpAlbumAdd, domain.OpAssetDelete}, nil},
		{"two earlier replacements left half done take their old assets to Recently Deleted, beside the other changes of the asset",
			remembered(state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), history, history5), snapshot(a7, restored, restored5),
			scans{"alpha": {tripFile}}, specs{alpha}, "", p, kinds{domain.OpAlbumAdd, domain.OpAlbumRemove, domain.OpAssetDelete}, nil},
		{"an earlier replacement left half done takes its old asset to Recently Deleted while the asset that replaced it is gone from the library",
			remembered(domain.State{Assets: assets{a7}, Albums: albums}, history), snapshot(restored), nil, specs{alpha}, "", p, kinds{domain.OpAssetDelete}, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.Parse("{event_path}/{original_name}")
			if err != nil {
				t.Fatalf("template.Parse: %v", err)
			}
			in := plan.Input{
				State: c.state, Snapshot: c.snapshot, Scans: c.scans, Remotes: c.remotes, OnlyRemote: c.only, Policies: c.p,
				Template: tmpl, Fields: fieldsOf(c.snapshot), Now: now, RunID: "run-2",
			}
			first, d, err := plan.Build(in)
			var did kinds
			for _, o := range first.Ops {
				did = append(did, o.Kind)
			}
			if err != nil || !reflect.DeepEqual(did, c.kinds) || len(first.Ops)+len(first.Links)+len(first.Conflicts) == 0 {
				t.Fatalf("Build = %+v, %v; want a plan with the operations %v", first, err, c.kinds)
			}
			s, snap, listed := plan.Simulate(in, first, d)
			next := in
			next.State, next.Snapshot, next.Scans, next.Fields, next.Now, next.RunID = s, snap, listed, fieldsOf(snap), now.Add(time.Hour), "run-3"
			second, again, err := plan.Build(next)
			s2, snap2, listed2 := plan.Simulate(next, second, again)
			if err != nil || len(second.Ops)+len(second.Links)+len(second.Albums)+len(second.Pairs) > 0 || !reflect.DeepEqual(second.Conflicts, c.conflicts) ||
				!reflect.DeepEqual(s2, s) || !reflect.DeepEqual(snap2, snap) || !reflect.DeepEqual(listed2, listed) {
				t.Errorf("after Simulate =\n%+v\n%+v\n%+v\nBuild =\n%+v\n%+v\n%v\nSimulate of it =\n%+v\n%+v\n%+v\nwant no operation, record, album or pair, the conflicts %+v, and nothing that the second Simulate changes",
					s, snap, listed, second, again, err, s2, snap2, listed2, c.conflicts)
			}
		})
	}
}
