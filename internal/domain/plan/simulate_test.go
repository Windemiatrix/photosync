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
// operation on the cloud do once they succeed, not the code, and must not
// depend on the order of the input, but for that of the operations, which is
// the order of the run.
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
	reversed := func(in plan.Input, p domain.Plan, d domain.SnapshotDelta) (plan.Input, domain.Plan, domain.SnapshotDelta) {
		// The operations keep the order of the run, and the history of the
		// delta the order that Build gives it.
		s, snap, listed := in.State, in.Snapshot, scans{}
		s.Assets, s.Albums, s.AlbumPairs = slices.Clone(s.Assets), slices.Clone(s.Albums), slices.Clone(s.AlbumPairs)
		s.Links, s.History, s.Conflicts = slices.Clone(s.Links), slices.Clone(s.History), slices.Clone(s.Conflicts)
		snap.Assets, snap.Albums = slices.Clone(snap.Assets), slices.Clone(snap.Albums)
		p.Links, p.Pairs, p.Conflicts = slices.Clone(p.Links), slices.Clone(p.Pairs), slices.Clone(p.Conflicts)
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

// Each row is the world of a run whose plan changes the cloud and the state
// but not iCloud. Once every operation of the plan succeeds, a run an hour
// later, when what the first wrote has settled, finds nothing to do: no
// operation, record, album or pair, only the conflicts that the first left
// open, and the same delta. Each row names the operations of the first plan,
// so that no row passes by planning nothing.
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
			if err != nil || len(second.Ops)+len(second.Links)+len(second.Albums)+len(second.Pairs) > 0 ||
				!reflect.DeepEqual(second.Conflicts, c.conflicts) || !reflect.DeepEqual(again, d) {
				t.Errorf("after Simulate =\n%+v\n%+v\n%+v\nBuild =\n%+v\n%+v\n%v\nwant no operation, record, album or pair, the conflicts %+v and the delta\n%+v",
					s, snap, listed, second, again, err, c.conflicts, d)
			}
		})
	}
}
