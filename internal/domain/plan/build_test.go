package plan_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is the world of a run: the state that the last applied run left,
// the photo library and the listings of the remotes. The expected plan and
// delta follow the rules of the steps of a run, not the code, and must not
// depend on the order of the input, nor on the order of the remotes while
// the policies name their priority.
func TestBuild(t *testing.T) {
	t.Parallel()
	const (
		personal, shared               = domain.LibraryPersonal, domain.LibraryShared
		original, live                 = domain.RoleOriginal, domain.RoleLiveVideo
		holidaysDir, tripDir, partyDir = "2024/05-01 - Holidays", "2024/05-02 - Trip", "2024/05-04 - Party"
		looseDir, catsDir              = "2024", "Albums/Cats"
		vacationDir, laterDir          = "2024/05-01 - Vacation", "2023/05-01 - Holidays"
		bareDir, beachDir              = "2024/05-01", "2024/05-05 - Beach"
		slashDir                       = "2024/05-01 - A∕B"
		event                          = "{event_path}/{original_name}"
		quarantined                    = ".photosync/quarantine/2026-11-13/"

		year, cats, holidays, trip, party domain.AlbumID = 1, 5, 10, 11, 12
		folder2023, bareDay, beach, slash domain.AlbumID = 12, 13, 13, 14
	)
	type (
		assets = []domain.Asset
		pairs  = []domain.AlbumPair
		scans  = map[string][]domain.RemoteEntry
		specs  = []domain.RemoteSpec
		ops    = []domain.Op
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
	uuidOf := map[domain.AlbumID]string{cats: "u-cats", holidays: "u-hol", trip: "u-trip", bareDay: "u-bare", slash: "u-slash"}
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
	delta := func(as ...domain.Asset) domain.SnapshotDelta { return domain.SnapshotDelta{Assets: as, Albums: albums} }
	file := func(root domain.LibraryKind, at string, size int64, md5 string, mtime time.Time) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: mtime}
	}
	link := func(id domain.LinkID, remote string, e domain.RemoteEntry, role domain.LinkRole) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: holidays,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	named := map[string]string{
		holidaysDir: `["2024"] "05/01 Holidays"`, tripDir: `["2024"] "05/02 Trip"`, partyDir: `["2024"] "05/04 Party"`,
		catsDir: `[] "Cats"`, vacationDir: `["2024"] "05/01 Vacation"`, laterDir: `["2023"] "05/01 Holidays"`, bareDir: `["2024"] "05/01"`,
		beachDir: `["2024"] "05/05 Beach"`, slashDir: `["2024"] "05/01 A/B"`,
	}
	pair := func(album domain.AlbumID, remote string, root domain.LibraryKind, dir string, by domain.PairOrigin) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: album, Remote: remote, Root: root, DirPath: dir, CreatedBy: by, NamedAfter: named[dir]}
	}
	state := func(as assets, ps pairs, ls ...domain.Link) domain.State {
		return domain.State{Assets: as, Albums: albums, AlbumPairs: ps, Links: ls}
	}
	spec := func(name string, mode domain.Mode) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}
	}
	alpha, beta, down := spec("alpha", domain.ModeTwoWay), spec("beta", domain.ModeTwoWay), spec("alpha", domain.ModeDownloadOnly)
	p := domain.DefaultPolicies()
	p.Deletes = domain.DeletesQuarantine
	propagate, ask := p, p
	propagate.Deletes, ask.Conflict = domain.DeletesPropagate, domain.ConflictAsk

	e1 := file(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled)
	e2 := file(personal, holidaysDir+"/IMG_0002.HEIC", 200, "b2", settled)
	e2.Meta = domain.MediaMeta{Taken: may1, Width: 4032, Height: 3024}
	e4 := file(personal, partyDir+"/IMG_0004.HEIC", 400, "d4", settled)
	ev := file(personal, holidaysDir+"/IMG_0001.MOV", 30, "v1", settled)
	xmp := file(personal, holidaysDir+"/IMG_0001.xmp", 5, "x1", settled)
	rewritten := file(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1", young)
	photo, video, other := link(1, "alpha", e1, original), link(2, "alpha", ev, live), link(2, "beta", e1, original)
	removed, removedVideo, detached := photo, video, photo
	removed.Status, removedVideo.Status, detached.Status = "", "", domain.StatusDetached
	a7, in7Trip := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", holidays), asset(7, "IMG_0001.HEIC", 100, may1, "run-2", trip)
	left7 := asset(7, "IMG_0001.HEIC", 100, may1, "run-1")
	trashed := asset(8, "IMG_0002.HEIC", 200, may1, "run-2")
	trashed.InTrash = true
	n8 := withVideo(asset(8, "IMG_0001.HEIC", 200, may1.Add(time.Hour), "run-2", holidays))
	m8 := asset(8, "IMG_0002.HEIC", 200, may1, "run-2")
	n1, n2 := asset(1, "IMG_0001.HEIC", 100, may2, "run-2"), asset(2, "IMG_0001.HEIC", 200, may2, "run-2", trip)
	h1 := asset(1, "IMG_0001.HEIC", 100, may1, "run-2", holidays)
	twin := h1
	twin.Key.UUID = "uuid-x"
	bare8, bare9 := asset(8, "IMG_0008.HEIC", 0, may1, "run-2", holidays), asset(9, "IMG_0009.HEIC", 0, may1, "run-2", holidays)
	bare8.Key, bare8.Resources = domain.AssetKey{CloudAssetGUID: "guid-8", UUID: "uuid-9"}, nil
	bare9.Key, bare9.Resources = domain.AssetKey{UUID: "uuid-8"}, nil
	alphaHolidays, alphaTrip := pair(holidays, "alpha", personal, holidaysDir, domain.OriginICloud), pair(trip, "alpha", personal, tripDir, domain.OriginICloud)
	betaHolidays := pair(holidays, "beta", personal, holidaysDir, domain.OriginICloud)
	history := domain.AssetHistory{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-6", UUID: "uuid-6"}}
	historyGone := history
	historyGone.Deleted = true
	restored := asset(6, "IMG_0006.HEIC", 100, may1, "run-2")
	looseFile, catsFile := file(personal, looseDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, catsDir+"/IMG_0001.HEIC", 100, "a1", settled)
	lesser, lesser7 := file(personal, holidaysDir+"/IMG_0002.jpg", 150, "c2", settled), file(personal, holidaysDir+"/IMG_0001.jpg", 150, "c1", settled)
	lesser.Meta = domain.MediaMeta{Taken: may1, Width: 1600, Height: 1200}
	lesser7.Meta = lesser.Meta
	youngBeta := file(personal, tripDir+"/IMG_0002.HEIC", 50, "y2", young)
	partyPhoto, inTrip, inCats, binned := photo, photo, photo, other
	partyPhoto.Path, partyPhoto.AlbumID = partyDir+"/IMG_0001.HEIC", party
	inTrip.Path, inTrip.AlbumID = tripDir+"/IMG_0001.HEIC", trip
	inCats.Path, inCats.AlbumID = catsFile.Path, cats
	binned.Status = domain.StatusQuarantinedICloud
	catsHolidays := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", cats, holidays)
	catsAll := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", cats, holidays, trip)
	alphaCats := pair(cats, "alpha", personal, catsDir, domain.OriginICloud)
	conflict := func(remote string, status domain.ConflictStatus, choice domain.ConflictPolicy) domain.Conflict {
		return domain.Conflict{AssetID: 7, Remote: remote, Status: status, Resolution: choice}
	}
	recorded := func(s domain.State, cs ...domain.Conflict) domain.State {
		s.Conflicts = cs
		return s
	}
	loose8 := withVideo(asset(8, "IMG_0001.HEIC", 200, may1.Add(time.Hour), "run-2"))
	youngTwin := file(personal, "2024/05-01 - holidays/IMG_0001 (1).HEIC", 50, "y1", young)
	stray := file(personal, holidaysDir+"/img_0001.heic", 100, "n1", settled)
	edit2 := file(personal, holidaysDir+"/IMG_0002_edited.jpg", 10, "g2", settled)
	edit2x := file(personal, holidaysDir+"/img_0002_edited.jpg", 10, "g3", settled)
	edit3 := file(personal, holidaysDir+"/IMG_0003_edited.jpg", 10, "g4", settled)
	edits, slashed := p, p
	edits.EditedFiles = domain.EditedQuarantine
	slashed.Templates.Cloud = []string{"{YYYY}", "{MM}/{DD} - {Description}"}
	doubles := func(paths ...string) domain.Notice {
		return domain.Notice{Code: "plan.doubles", Params: append([]string{"alpha", string(personal)}, paths...)}
	}

	album := func(kind domain.OpKind, of domain.AssetID, id domain.AlbumID, rule string) domain.Op {
		return domain.Op{Kind: kind, Asset: of, AlbumID: id, Rule: rule}
	}
	put := func(of domain.AssetID, dst string, role domain.LinkRole, id domain.AlbumID, size int64) domain.Op {
		return domain.Op{Kind: domain.OpPut, Remote: "alpha", DstRoot: personal, Dst: dst, Asset: of, Rule: "R-LINK-01", Links: []domain.Link{{
			AssetID: of, Remote: "alpha", Root: personal, Path: dst, Role: role, AlbumID: id, Size: size,
			Level: domain.LevelUploaded, Status: domain.StatusActive,
		}}}
	}
	moved := func(l domain.Link, root domain.LibraryKind, dst string, id domain.AlbumID, rule string) domain.Op {
		to := l
		to.Root, to.Path, to.AlbumID = root, dst, id
		return domain.Op{Kind: domain.OpMove, Remote: l.Remote, Root: l.Root, Src: l.Path, DstRoot: root, Dst: dst, Asset: 7, Rule: rule, Links: []domain.Link{to}}
	}
	settling := func(at string) domain.Notice {
		return domain.Notice{Code: "plan.settling", Params: []string{"alpha", string(personal), at}}
	}
	noPath := func(uuid string) domain.Notice {
		return domain.Notice{Code: "plan.no_path", Params: []string{"alpha", uuid, "pathtemplate: invalid cloud path: asset " + uuid + " has no original file"}}
	}
	swapped := put(8, holidaysDir+"/IMG_0002 (1).HEIC", original, holidays, 200)
	swapped.Rule, swapped.Links[0].ID = "R-OBJECT-10", 2
	parked := domain.Link{
		ID: 2, AssetID: 8, Remote: "alpha", Root: personal, Path: lesser.Path, Role: original, AlbumID: holidays, Size: 150,
		SHA256: "sc2", MD5: "c2", ETag: "ec2", MTime: settled, Level: domain.LevelSameShot,
		Status: domain.StatusQuarantinedRemote, QuarantinePath: quarantined + lesser.Path,
	}
	betaPut := func(dst string, role domain.LinkRole, size int64) domain.Op {
		o := put(8, dst, role, holidays, size)
		o.Remote, o.Links[0].Remote = "beta", "beta"
		return o
	}
	holding := func(o domain.Op, ls ...domain.Link) domain.Op {
		o.Links = ls
		return o
	}
	took := func(l domain.Link, e domain.RemoteEntry, id domain.AlbumID) domain.Link {
		l.Root, l.Path, l.AlbumID, l.Size, l.SHA256, l.MD5, l.ETag, l.MTime = e.Root, e.Path, id, e.Size, e.SHA256, e.MD5, e.ETag, e.MTime
		return l
	}
	tripFile, tripVideo := file(personal, tripDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, tripDir+"/IMG_0001.MOV", 30, "v1", settled)
	partyFile, replaced := file(personal, partyDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1", settled)
	twinMov, twinHeic := file(personal, "2024/05-02 - trip/IMG_0001.mov", 30, "y3", young), file(personal, "2024/05-02 - trip/IMG_0001.HEIC", 50, "y4", young)
	photo3, trip3, back := photo, inTrip, photo
	photo3.ID, trip3.ID = 3, 3
	back.Status, back.QuarantinePath = domain.StatusQuarantinedRemote, quarantined+e1.Path
	paired, matched3 := took(link(2, "alpha", e2, original), e2, holidays), took(link(3, "alpha", lesser7, original), lesser7, holidays)
	paired.AssetID, matched3.Level = 8, domain.LevelSameShot
	a7both, m8trip, moving := asset(7, "IMG_0001.HEIC", 100, may1, "run-1", holidays, trip), asset(8, "IMG_0002.HEIC", 200, may1, "run-2", trip), a7
	moving.TargetLibrary = shared
	live8trip, e2v := withVideo(m8trip), file(personal, holidaysDir+"/IMG_0002.MOV", 30, "v8", settled)
	pairedVideo := took(link(3, "alpha", e2v, live), file(personal, tripDir+"/IMG_0002.mov", 30, "v8", settled), trip)
	pairedVideo.AssetID, pairedVideo.Level = 8, domain.LevelPairComponent
	reup := put(7, tripDir+"/IMG_0001.HEIC", original, trip, 100)
	reup.Rule = "R-LINK-10"
	alphaUp := spec("alpha", domain.ModeUploadOnly)
	lowHolidays, lowTrip := "2024/05-01 - holidays", "2024/05-02 - trip"
	youngH, youngT := file(personal, lowHolidays+"/IMG_0009.HEIC", 50, "y5", young), file(personal, lowTrip+"/IMG_0009.HEIC", 50, "y6", young)
	oldQ := file(personal, quarantined+holidaysDir+"/IMG_0003_EDITED.jpg", 10, "g5", settled)
	cat1, betaTrip := asset(1, "IMG_0001.HEIC", 100, may1, "run-2", cats), pair(trip, "beta", personal, tripDir, domain.OriginICloud)
	inBeta := func(n domain.Notice) domain.Notice {
		n.Params[0] = "beta"
		return n
	}
	aged := ".photosync/quarantine/2026-08-14/" + holidaysDir
	aged5, kept6 := file(personal, aged+"/IMG_0005.HEIC", 10, "q5", settled), file(personal, ".photosync/quarantine/2026-08-15/"+holidaysDir+"/IMG_0006.HEIC", 10, "q6", settled)
	aged7, aged7x := file(personal, aged+"/IMG_0007.HEIC", 10, "q7", settled), file(personal, aged+"/img_0007.heic", 10, "q8", settled)
	expiring := func(remote string, e domain.RemoteEntry) domain.Op {
		return domain.Op{Kind: domain.OpExpire, Remote: remote, Root: e.Root, Src: e.Path, Rule: "R-STATUS-09", Irreversible: true}
	}
	imported := func(l domain.Link, e domain.RemoteEntry, id domain.AlbumID) domain.Link {
		l = took(l, e, id)
		l.AssetID, l.Level = 0, domain.LevelImported
		return l
	}
	relink := func(rule string, id domain.AlbumID, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: ls[0].Remote, Root: ls[0].Root, Src: ls[0].Path, AlbumID: id, Rule: rule, Links: ls}
	}
	withoutPersonal, blind := p, alpha
	withoutPersonal.AlbumsForPersonal, blind.Capabilities = false, domain.Capabilities{}
	unhashed := file(personal, holidaysDir+"/IMG_0004.HEIC", 50, "b1", settled)
	renewed := func(l domain.Link, e domain.RemoteEntry, id domain.AlbumID) domain.Link {
		l = took(l, e, id)
		l.Level = domain.LevelImported
		return l
	}
	replacing := func(rule string, ls ...domain.Link) domain.Op {
		return domain.Op{Kind: domain.OpImport, Remote: ls[0].Remote, Root: ls[0].Root, Src: ls[0].Path, Asset: 7, Rule: rule, Links: ls}
	}
	remembered := func(s domain.State, hs ...domain.AssetHistory) domain.State {
		s.History = hs
		return s
	}
	awayVideo, hidden7, restored5 := video, a7, asset(5, "IMG_0005.HEIC", 100, may1, "run-2")
	awayVideo.Status, awayVideo.QuarantinePath, hidden7.Hidden = domain.StatusQuarantinedRemote, quarantined+ev.Path, true
	history5 := domain.AssetHistory{AssetID: 7, Key: restored5.Key}
	replacedTrip, movTrip := file(personal, tripDir+"/IMG_0001.HEIC", 120, "b1", settled), file(personal, tripDir+"/IMG_0001.mov", 30, "v1", settled)
	changedMov := file(personal, holidaysDir+"/IMG_0001.MOV", 35, "v2", settled)
	iCloudWins, partyOther := p, other
	iCloudWins.Conflict, partyOther.Path, partyOther.AlbumID = domain.ConflictICloudWins, partyDir+"/IMG_0001.HEIC", party
	new3, new4 := file(personal, holidaysDir+"/IMG_0003.HEIC", 300, "c3", settled), file(personal, holidaysDir+"/IMG_0004.HEIC", 400, "c4", settled)
	new5, trip2 := file(personal, holidaysDir+"/IMG_0005.HEIC", 500, "c5", settled), file(personal, tripDir+"/IMG_0002.HEIC", 200, "b2", settled)
	h8, kept8 := asset(8, "IMG_0002.HEIC", 200, may1, "run-2", holidays), paired
	kept8.ID, kept8.Remote = 3, "beta"
	importNew := func(remote string, e domain.RemoteEntry) domain.Op {
		return relink("R-LINK-02", holidays, imported(domain.Link{Remote: remote, Role: original, Status: domain.StatusActive}, e, holidays))
	}
	counted := func(code, remote, count string) domain.Notice {
		return domain.Notice{Code: code, Params: []string{remote, count}}
	}
	vacationAlbums, vacationLibrary := slices.Clone(albums), slices.Clone(library)
	vacationAlbums[2], vacationLibrary[2].Title = inYear(holidays, "u-hol", "05/01 Vacation", may1.Add(-2*time.Hour)), "05/01 Vacation"
	inVacation := func(s domain.LibrarySnapshot) domain.LibrarySnapshot {
		s.Albums = vacationLibrary
		return s
	}
	vacationPair := pair(holidays, "alpha", personal, vacationDir, domain.OriginICloud)
	atVacation, s9, file9 := photo, asset(9, "IMG_0009.HEIC", 900, may1, "run-1", holidays), file(shared, holidaysDir+"/IMG_0009.HEIC", 900, "c9", settled)
	atVacation.Path, s9.Library, s9.TargetLibrary = vacationDir+"/IMG_0001.HEIC", shared, shared
	link9 := link(3, "alpha", file9, original)
	link9.AssetID = 9
	moved9 := link9
	moved9.Path = vacationDir + "/IMG_0009.HEIC"
	vacationFile := file(personal, vacationDir+"/IMG_0001.HEIC", 100, "a1", settled)
	holidaysFolder := domain.RemoteEntry{Root: personal, Path: holidaysDir, IsDir: true, MTime: settled}
	yearFolder := domain.RemoteEntry{Root: personal, Path: "2024", IsDir: true, MTime: settled}
	renaming := domain.Op{Kind: domain.OpAlbumRename, AlbumID: holidays, Dst: "05/01 Vacation", Rule: "R-LAYOUT-12"}
	laterFile := file(personal, laterDir+"/IMG_0001.HEIC", 100, "a1", settled)
	refiling := domain.Op{Kind: domain.OpAlbumMove, AlbumID: holidays, ParentID: folder2023, Rule: "R-LAYOUT-13"}
	new9, new10 := asset(9, "IMG_0009.HEIC", 900, may1, "run-2", holidays), asset(10, "IMG_0010.HEIC", 1000, may2, "run-2", trip)
	new9.Library, new9.TargetLibrary, new10.Key.CloudAssetGUID = shared, shared, ""
	sharedHolidays := domain.AlbumPair{AlbumID: holidays, Remote: "alpha", Root: shared, DirPath: holidaysDir, CreatedBy: domain.OriginRemote}
	sharedPut := put(9, holidaysDir+"/IMG_0009.HEIC", original, holidays, 900)
	sharedPut.DstRoot, sharedPut.Links[0].Root = shared, shared
	bareAlbums := append(slices.Clone(albums), inYear(bareDay, "u-bare", "05/01", may1.Add(-2*time.Hour)))
	bareLibrary := append(slices.Clone(library), domain.Album{UUID: "u-bare", Title: "05/01", Start: may1.Add(-2 * time.Hour), ParentUUID: "u-2024", FolderTitles: []string{"2024"}})
	onBareDay := func(s domain.LibrarySnapshot) domain.LibrarySnapshot {
		s.Albums = bareLibrary
		return s
	}
	bare1, dayNew := asset(1, "IMG_0001.HEIC", 100, may1, "run-2", bareDay), file(personal, bareDir+"/IMG_0004.HEIC", 400, "d4", settled)
	skippedDir := func(dir string) domain.Notice {
		return domain.Notice{Code: "plan.skipped", Params: []string{"dir", "alpha", string(personal), dir}}
	}
	skippedAsset := func(name, uuid string) domain.Notice {
		return domain.Notice{Code: "plan.skipped", Params: []string{"asset", name, uuid}}
	}
	rawDir := holidaysDir + "/raw"
	rawFile, editedFile := file(personal, rawDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, holidaysDir+"/edited/IMG_0001.HEIC", 100, "a1", settled)
	newAtRoot, newInYear := file(personal, "IMG_0009.HEIC", 900, "t1", settled), file(personal, looseDir+"/IMG_0009.HEIC", 900, "t2", settled)
	newInCats, newInRaw := file(personal, catsDir+"/IMG_0009.HEIC", 900, "t3", settled), file(personal, rawDir+"/IMG_0009.HEIC", 900, "t4", settled)
	catsEdit, inCatsEdit := file(personal, catsDir+"/IMG_0001_edited.HEIC", 100, "a1", settled), inCats
	inCatsEdit.Path = catsEdit.Path
	partyAlbum := domain.Album{ID: party, Kind: domain.KindEvent, Title: "05/04 Party", ParentID: year, FolderTitles: []string{"2024"}}
	beachFile := file(personal, beachDir+"/IMG_0001.HEIC", 100, "a1", settled)
	beachAlbum := domain.Album{ID: beach, Kind: domain.KindEvent, Title: "05/05 Beach", ParentID: year, FolderTitles: []string{"2024"}}
	creating := func(id domain.AlbumID) domain.Op {
		return domain.Op{Kind: domain.OpAlbumCreate, AlbumID: id, Rule: "R-LAYOUT-06"}
	}
	tripFolder := domain.RemoteEntry{Root: personal, Path: tripDir, IsDir: true, MTime: settled}
	dropped := removed
	dropped.QuarantinePath = quarantined + e1.Path
	lost := photo
	lost.Status = domain.StatusQuarantinedICloud
	slashAlbums := append(slices.Clone(albums), inYear(slash, "u-slash", "05/01 A/B", may1.Add(-2*time.Hour)))
	slashLibrary := append(slices.Clone(library), domain.Album{UUID: "u-slash", Title: "05/01 A/B", Start: may1.Add(-2 * time.Hour), ParentUUID: "u-2024", FolderTitles: []string{"2024"}})
	slash1 := asset(1, "IMG_0001.HEIC", 100, may1, "run-2", slash)
	slashShot := snapshot(slash1)
	slashShot.Albums = slashLibrary
	may11 := time.Date(2024, 5, 11, 12, 0, 0, 0, time.UTC)
	alone1, alone2 := asset(1, "IMG_0001.HEIC", 100, may11, "run-2"), asset(1, "IMG_0002.HEIC", 200, may11, "run-2")
	lone2 := file(personal, holidaysDir+"/IMG_0002.HEIC", 200, "b2", settled)
	lone2.Meta = domain.MediaMeta{Taken: may11, Width: 4032, Height: 3024}
	themed := inYear(holidays, "u-hol", "Holidays", may1.Add(-2*time.Hour))
	themed.Kind = domain.KindThematic
	themedAlbums, themedLibrary := slices.Clone(albums), slices.Clone(library)
	themedAlbums[2], themedLibrary[2].Title = themed, "Holidays"
	themedSnapshot := snapshot()
	themedSnapshot.Albums = themedLibrary
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
	reversed := func(in plan.Input) plan.Input {
		s, snap, scan := in.State, in.Snapshot, scans{}
		s.Assets, s.Albums, s.AlbumPairs = slices.Clone(s.Assets), slices.Clone(s.Albums), slices.Clone(s.AlbumPairs)
		s.Links, s.History, s.Conflicts = slices.Clone(s.Links), slices.Clone(s.History), slices.Clone(s.Conflicts)
		snap.Assets, snap.Albums = slices.Clone(snap.Assets), slices.Clone(snap.Albums)
		slices.Reverse(s.Assets)
		slices.Reverse(s.Albums)
		slices.Reverse(s.AlbumPairs)
		slices.Reverse(s.Links)
		slices.Reverse(s.History)
		slices.Reverse(s.Conflicts)
		slices.Reverse(snap.Assets)
		slices.Reverse(snap.Albums)
		for name, es := range in.Scans {
			scan[name] = slices.Clone(es)
			slices.Reverse(scan[name])
		}
		remotes, policies := slices.Clone(in.Remotes), in.Policies
		slices.Reverse(remotes)
		policies.RemotePriority = nil
		for _, r := range in.Remotes {
			policies.RemotePriority = append(policies.RemotePriority, r.Name)
		}
		in.State, in.Snapshot, in.Scans, in.Remotes, in.Policies = s, snap, scan, remotes, policies
		return in
	}
	show := func(pl domain.Plan, d domain.SnapshotDelta, err error) string {
		var out []string
		for _, o := range pl.Ops {
			line := fmt.Sprintf("%s %s %s:%s -> %s:%s asset %d album %d parent %d %s irreversible=%t",
				o.Kind, o.Remote, o.Root, o.Src, o.DstRoot, o.Dst, o.Asset, o.AlbumID, o.ParentID, o.Rule, o.Irreversible)
			for _, l := range o.Links {
				line += fmt.Sprintf(" %+v", l)
			}
			out = append(out, line)
		}
		for _, a := range pl.Albums {
			out = append(out, fmt.Sprintf("album %+v", a))
		}
		for _, x := range pl.Pairs {
			out = append(out, fmt.Sprintf("pair %d %s %s:%s %s named %s", x.AlbumID, x.Remote, x.Root, x.DirPath, x.CreatedBy, x.NamedAfter))
		}
		for _, l := range pl.Links {
			out = append(out, fmt.Sprintf("link %+v", l))
		}
		for _, c := range pl.Conflicts {
			out = append(out, fmt.Sprintf("conflict %+v", c))
		}
		for _, n := range pl.Notices {
			out = append(out, fmt.Sprintf("notice %s %q", n.Code, n.Params))
		}
		return strings.Join(append(out, fmt.Sprintf("delta %+v", d), fmt.Sprintf("error %v", err)), "\n")
	}

	for _, c := range []struct {
		name     string
		state    domain.State
		snapshot domain.LibrarySnapshot
		scans    scans
		remotes  specs
		only     string
		p        domain.Policies
		tmpl     string
		plan     domain.Plan
		delta    domain.SnapshotDelta
		err      error
	}{
		{"nothing at all", domain.State{}, domain.LibrarySnapshot{}, nil, nil, "", p, event, domain.Plan{}, domain.SnapshotDelta{}, nil},
		{"a file that the user moved takes the asset into the album of its new folder, and a new asset in Recently Deleted goes nowhere",
			domain.State{Assets: assets{a7}, Albums: albums, AlbumPairs: pairs{alphaHolidays, alphaTrip}, Links: []domain.Link{photo},
				History: []domain.AssetHistory{history}},
			snapshot(a7, trashed), scans{"alpha": {file(personal, tripDir+"/IMG_0001.HEIC", 100, "a1", settled)}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{holding(album(domain.OpAlbumAdd, 7, trip, "R-LINK-05"), took(photo, tripFile, trip)), album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05")}},
			domain.SnapshotDelta{Assets: assets{a7, trashed}, Albums: albums, History: []domain.AssetHistory{historyGone}}, nil},
		{"a new asset goes up into its event under a name that no file of the folder takes, its Live Photo video beside it",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, n8), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{put(8, holidaysDir+"/IMG_0001 (1).HEIC", original, holidays, 200), put(8, holidaysDir+"/IMG_0001 (1).mov", live, holidays, 30)}},
			delta(a7, n8), nil},
		{"an asset that left its only event takes its file to quarantine, while a new asset goes up into that event", // covers: R-OBJECT-06
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(left7, n8), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				{Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: e1.Path, DstRoot: personal, Dst: dropped.QuarantinePath,
					Asset: 7, Rule: "R-OBJECT-04", Links: []domain.Link{dropped}},
				put(8, holidaysDir+"/IMG_0001 (1).HEIC", original, holidays, 200), put(8, holidaysDir+"/IMG_0001 (1).mov", live, holidays, 30),
			}},
			delta(left7, n8), nil},
		{"the first placement takes a new asset into the only event of its day, and two new assets make one pair for an album without a folder",
			state(nil, pairs{alphaHolidays}), snapshot(n2, n1), nil, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					put(2, tripDir+"/IMG_0001 (1).HEIC", original, trip, 200), put(1, tripDir+"/IMG_0001.HEIC", original, trip, 100),
					album(domain.OpAlbumAdd, 1, trip, "R-LAYOUT-05"),
				},
				Pairs: pairs{pair(trip, "alpha", personal, tripDir, domain.OriginICloud)},
			},
			delta(n1, n2), nil},
		{"a file that is still being written keeps its asset as the state has it in its remote, while another remote follows the asset to its new event",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip, betaHolidays}, photo, other), snapshot(in7Trip),
			scans{"alpha": {rewritten}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops:     ops{moved(other, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")},
				Pairs:   pairs{pair(trip, "beta", personal, tripDir, domain.OriginICloud)},
				Notices: []domain.Notice{settling(rewritten.Path)},
			},
			delta(in7Trip), nil},
		{"matching links a new asset with a new file of a paired folder: the asset joins its album, and nothing goes up",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, m8), scans{"alpha": {e2, e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{holding(album(domain.OpAlbumAdd, 8, holidays, "R-LINK-03"), paired)}},
			delta(a7, m8), nil},
		{"under propagate an asset gone from the library takes its file, its Live Photo video and its sidecar out of the cloud for good, under its ID in the state",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", propagate, event,
			domain.Plan{Ops: ops{
				{Kind: domain.OpDelete, Remote: "alpha", Root: personal, Src: e1.Path, Asset: 7, Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{removed}},
				{Kind: domain.OpDelete, Remote: "alpha", Root: personal, Src: ev.Path, Asset: 7, Rule: "R-STATUS-06", Irreversible: true, Links: []domain.Link{removedVideo}},
				{Kind: domain.OpDelete, Remote: "alpha", Root: personal, Src: xmp.Path, Asset: 7, Rule: "R-STATUS-06", Irreversible: true},
			}},
			delta(), nil},
		{"a file that the user moved into a new folder beside a new file: the album comes to iCloud, then the import and the asset join it",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(a7),
			scans{"alpha": {e4, file(personal, partyDir+"/IMG_0001.HEIC", 100, "a1", settled), holidaysFolder}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpAlbumCreate, AlbumID: party, Rule: "R-LAYOUT-06"},
					{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: e4.Path, AlbumID: party, Rule: "R-LINK-02", Links: []domain.Link{{
						Remote: "alpha", Root: personal, Path: e4.Path, Role: original, AlbumID: party, Size: 400,
						SHA256: "sd4", MD5: "d4", ETag: "ed4", MTime: settled, Level: domain.LevelImported, Status: domain.StatusActive,
					}}},
					holding(album(domain.OpAlbumAdd, 7, party, "R-LINK-05"), took(photo, partyFile, party)),
					album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05"),
				},
				Albums: []domain.Album{{ID: party, Kind: domain.KindEvent, Title: "05/04 Party", ParentID: year, FolderTitles: []string{"2024"}}},
				Pairs:  pairs{pair(party, "alpha", personal, partyDir, domain.OriginRemote)},
			},
			delta(a7), nil},
		{"an unknown field of the template stops the run", state(nil, pairs{alphaHolidays}), snapshot(h1), nil, specs{alpha}, "", p, "{event_path}/{foo}",
			domain.Plan{}, domain.SnapshotDelta{}, pathtemplate.ErrUnknownField},
		{"two assets with one cloud GUID stop the run", state(nil, nil), snapshot(h1, twin), nil, specs{alpha}, "", p, event,
			domain.Plan{}, domain.SnapshotDelta{}, plan.ErrBadSnapshot},
		{"templates that Validate refuses stop the run", state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {e1}}, specs{alpha}, "", slashed, event,
			domain.Plan{}, domain.SnapshotDelta{}, eventtpl.ErrSlash},
		{"templates that Validate refuses stop the run before the snapshot is read", state(nil, nil), snapshot(h1, twin), nil, specs{alpha}, "", slashed, event,
			domain.Plan{}, domain.SnapshotDelta{}, eventtpl.ErrSlash},
		{"a photo that the plan moves to its new event takes its Live Photo video and its sidecar along",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(in7Trip)),
			scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05"),
				moved(video, personal, tripDir+"/IMG_0001.mov", trip, "R-COMPONENT-04"),
				{Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: xmp.Path, DstRoot: personal, Dst: tripDir + "/IMG_0001.xmp", Asset: 7, Rule: "R-COMPONENT-04"},
			}},
			delta(withVideo(in7Trip)), nil},
		{"the notices of the scans and of the operations come in the order of their code, then of their parameters",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, bare8, bare9), scans{"alpha": {rewritten}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{noPath("uuid-8"), noPath("uuid-9"), settling(rewritten.Path)}},
			delta(a7, bare8, bare9), nil},
		{"a file that the user moved to the shared root takes the asset to the shared library, and the file of another remote follows",
			state(assets{a7}, pairs{alphaHolidays, pair(holidays, "alpha", shared, holidaysDir, domain.OriginICloud), betaHolidays}, photo, other),
			snapshot(a7), scans{"alpha": {file(shared, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled)}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops:   ops{moved(other, shared, holidaysDir+"/IMG_0001.HEIC", holidays, "R-OBJECT-07")},
				Pairs: pairs{pair(holidays, "beta", shared, holidaysDir, domain.OriginICloud)},
				Links: []domain.Link{took(photo, file(shared, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled), holidays)},
			},
			delta(moving), nil},
		{"a conflict under ask leaves the asset as the state has it in its remote",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip),
			scans{"alpha": {partyFile, holidaysFolder}}, specs{alpha}, "", ask, event,
			domain.Plan{
				Ops: ops{creating(party)}, Albums: []domain.Album{partyAlbum}, Pairs: pairs{pair(party, "alpha", personal, partyDir, domain.OriginRemote)},
				Conflicts: []domain.Conflict{conflict("alpha", domain.ConflictStatusOpen, "")},
			},
			delta(in7Trip), nil},
		{"download-only: under propagate an asset gone from the library leaves its file and its sidecar in the cloud",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(), scans{"alpha": {xmp, e1}}, specs{down}, "", propagate, event,
			domain.Plan{Links: []domain.Link{detached}}, delta(), nil},
		{"an earlier identity that the user brings back from Recently Deleted comes with a notice and takes no ID of its own",
			domain.State{Albums: albums, History: []domain.AssetHistory{historyGone}}, snapshot(restored), nil, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{{Code: "plan.replaced_restored", Params: []string{"IMG_0006.HEIC", "uuid-6"}}}},
			domain.SnapshotDelta{Albums: albums, History: []domain.AssetHistory{historyGone}}, nil},
		{"an asset that moves to a new event in two remotes makes a pair in each and closes its recorded conflicts there, in the order of their names",
			recorded(state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other),
				conflict("beta", domain.ConflictStatusSkipped, ""), conflict("alpha", domain.ConflictStatusSkipped, "")),
			snapshot(in7Trip), scans{"alpha": {e1}, "beta": {e1}}, specs{beta, alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05"),
					moved(other, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05"),
				},
				Pairs:     pairs{pair(trip, "alpha", personal, tripDir, domain.OriginICloud), pair(trip, "beta", personal, tripDir, domain.OriginICloud)},
				Conflicts: []domain.Conflict{conflict("alpha", domain.ConflictStatusResolved, ""), conflict("beta", domain.ConflictStatusResolved, "")},
			},
			delta(in7Trip), nil},
		{"a detached link keeps its asset out of sync in its remote: the asset moves to a new event, and its file neither follows nor goes up again",
			state(assets{a7}, pairs{alphaHolidays}, detached), snapshot(in7Trip), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{}, delta(in7Trip), nil},
		{"the choice of the user for a recorded conflict wins over the conflict policy",
			recorded(state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), conflict("alpha", domain.ConflictStatusOpen, domain.ConflictICloudWins)),
			snapshot(in7Trip), scans{"alpha": {partyFile, holidaysFolder}}, specs{alpha}, "", ask, event,
			domain.Plan{
				Ops:       ops{creating(party), moved(partyPhoto, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")},
				Albums:    []domain.Album{partyAlbum},
				Pairs:     pairs{pair(party, "alpha", personal, partyDir, domain.OriginRemote)},
				Conflicts: []domain.Conflict{conflict("alpha", domain.ConflictStatusResolved, domain.ConflictICloudWins)},
			},
			delta(in7Trip), nil},
		{"matching finds a lesser copy of a new asset in a paired folder: the original goes up beside it, and the copy goes to quarantine",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, m8), scans{"alpha": {lesser, e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				{Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: lesser.Path, DstRoot: personal, Dst: parked.QuarantinePath,
					Asset: 8, Rule: "R-OBJECT-10", Links: []domain.Link{parked}},
				swapped,
				album(domain.OpAlbumAdd, 8, holidays, "R-LINK-03"),
			}},
			delta(a7, m8), nil},
		{"a conflict under ask keeps the copy that matching finds in its remote out of the layout of the other remotes",
			state(assets{catsAll}, pairs{alphaHolidays, alphaTrip, alphaCats, betaHolidays}, inTrip, other), snapshot(catsHolidays),
			scans{"alpha": {beachFile, partyFile, tripFolder}, "beta": {e1}}, specs{alpha, beta}, "", ask, event,
			domain.Plan{
				Ops:       ops{creating(party), creating(beach)},
				Albums:    []domain.Album{partyAlbum, beachAlbum},
				Pairs:     pairs{pair(party, "alpha", personal, partyDir, domain.OriginRemote), pair(beach, "alpha", personal, beachDir, domain.OriginRemote)},
				Conflicts: []domain.Conflict{conflict("alpha", domain.ConflictStatusOpen, "")},
			},
			delta(catsHolidays), nil},
		{"one remote alone syncs: the other is neither read nor changed, and a link in the directory of a thematic album keeps its asset as the state has it there",
			state(assets{catsHolidays}, pairs{alphaHolidays, alphaCats, betaHolidays}, inCats, other), snapshot(catsHolidays),
			scans{"alpha": {catsFile}, "beta": {youngBeta, e1}}, specs{alpha, beta}, "alpha", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir(catsDir)}},
			delta(catsHolidays), nil},
		{"a remote that the configuration lacks leaves nothing to sync",
			state(assets{catsHolidays}, pairs{alphaHolidays, alphaCats, betaHolidays}, inCats, other), snapshot(catsHolidays),
			scans{"alpha": {catsFile}, "beta": {youngBeta, e1}}, specs{alpha, beta}, "gamma", p, event,
			domain.Plan{}, delta(catsHolidays), nil},
		{"an asset that another remote deletes gets no original in place of the lesser copy that matching finds",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, other), snapshot(a7), scans{"alpha": {lesser7}}, specs{alpha, beta}, "", p, event,
			domain.Plan{Ops: ops{{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-02", Links: []domain.Link{binned}}}, Links: []domain.Link{matched3}},
			delta(a7), nil},
		{"a new asset whose path in a remote differs from a file there in case alone goes up only to the other remote, and still joins its album in iCloud",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), snapshot(a7, loose8),
			scans{"alpha": {youngTwin, e1}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops: ops{
					betaPut(holidaysDir+"/IMG_0001 (1).HEIC", original, 200), betaPut(holidaysDir+"/IMG_0001 (1).mov", live, 30),
					album(domain.OpAlbumAdd, 8, holidays, "R-LAYOUT-05"),
				},
				Notices: []domain.Notice{doubles(holidaysDir, lowHolidays), doubles(holidaysDir+"/IMG_0001 (1).HEIC", youngTwin.Path), settling(youngTwin.Path)},
			},
			delta(a7, loose8), nil},
		{"under propagate a file whose path differs from another in case alone keeps its asset and its Live Photo video in the cloud, and the other file is still imported",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {stray, ev, e1}}, specs{alpha}, "", propagate, event,
			domain.Plan{
				Ops: ops{{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: stray.Path, AlbumID: holidays, Rule: "R-LINK-02", Links: []domain.Link{{
					Remote: "alpha", Root: personal, Path: stray.Path, Role: original, AlbumID: holidays, Size: 100,
					SHA256: "sn1", MD5: "n1", ETag: "en1", MTime: settled, Level: domain.LevelImported, Status: domain.StatusActive,
				}}}},
				Notices: []domain.Notice{doubles(e1.Path, stray.Path)},
			},
			delta(), nil},
		{"an edit whose path differs from another in case alone stays where it is, while a third edit goes to quarantine",
			state(nil, nil), snapshot(), scans{"alpha": {edit3, edit2x, edit2}}, specs{alpha}, "", edits, event,
			domain.Plan{
				Ops: ops{{Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: edit3.Path, DstRoot: personal, Dst: quarantined + edit3.Path,
					Rule: "R-STATUS-10"}},
				Notices: []domain.Notice{doubles(quarantined+edit2.Path, quarantined+edit2x.Path), doubles(edit2.Path, edit2x.Path), counted("plan.edits_left", "alpha", "2")},
			},
			delta(), nil},
		{"a photo that the user moved with its Live Photo video into the folder of another event takes the asset there, and both links take their new place with the album that it joins",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo3, video), snapshot(withVideo(a7)),
			scans{"alpha": {tripVideo, tripFile}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				holding(album(domain.OpAlbumAdd, 7, trip, "R-LINK-05"), took(video, tripVideo, trip), took(photo3, tripFile, trip)),
				album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05"),
			}},
			delta(withVideo(a7)), nil},
		{"a file of an asset in two events that the user deleted takes the asset out of the album of its folder, and its link leaves with it",
			state(assets{a7both}, pairs{alphaHolidays, alphaTrip}, photo, trip3), snapshot(a7both), scans{"alpha": {tripFile}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{holding(album(domain.OpAlbumRemove, 7, holidays, "R-OBJECT-02"), removed)}},
			delta(a7both), nil},
		{"upload-only: a file that the user replaced in place keeps its asset, and its link takes the new content",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {replaced}}, specs{alphaUp}, "", p, event,
			domain.Plan{Links: []domain.Link{took(photo, replaced, holidays)}}, delta(a7), nil},
		{"an asset that moves to another event while the user deletes its file goes up there again under a new link, and the old link leaves",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip), nil, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{reup}, Links: []domain.Link{removed}}, delta(in7Trip), nil},
		{"a file that the user brings back from quarantine takes its link back out of quarantine",
			state(assets{a7}, pairs{alphaHolidays}, back), snapshot(a7), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{Links: []domain.Link{took(photo, e1, holidays)}}, delta(a7), nil},
		{"upload-only: a new file that matching links in the folder of another album moves to the album of its asset, and its link goes with the move",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(a7, m8trip), scans{"alpha": {e2, e1}}, specs{alphaUp}, "", p, event,
			domain.Plan{Ops: ops{{Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: e2.Path, DstRoot: personal, Dst: tripDir + "/IMG_0002.HEIC",
				Asset: 8, Rule: "R-OBJECT-05", Links: []domain.Link{took(paired, file(personal, tripDir+"/IMG_0002.HEIC", 200, "b2", settled), trip)}}}},
			delta(a7, m8trip), nil},
		{"a photo whose Live Photo video would move to a path that differs from another file in case alone leaves both links as the state has them in the remote, while the asset still changes album",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(a7)),
			scans{"alpha": {twinMov, ev, tripFile}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:     ops{album(domain.OpAlbumAdd, 7, trip, "R-LINK-05"), album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05")},
				Notices: []domain.Notice{doubles(tripDir, lowTrip), doubles(tripDir+"/IMG_0001.mov", twinMov.Path), settling(twinMov.Path)},
			},
			delta(withVideo(a7)), nil},
		{"the choice of the user for a recorded conflict stays recorded while the move that it asks for would land on a path that differs from another file in case alone",
			recorded(state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), conflict("alpha", domain.ConflictStatusOpen, domain.ConflictICloudWins)),
			snapshot(in7Trip), scans{"alpha": {twinHeic, partyFile, holidaysFolder}}, specs{alpha}, "", ask, event,
			domain.Plan{
				Ops: ops{creating(party)}, Albums: []domain.Album{partyAlbum}, Pairs: pairs{pair(party, "alpha", personal, partyDir, domain.OriginRemote)},
				Notices: []domain.Notice{doubles(tripDir, lowTrip), doubles(tripDir+"/IMG_0001.HEIC", twinHeic.Path), settling(twinHeic.Path)},
			},
			delta(in7Trip), nil},
		{"a file that the user brings back from quarantine for an asset gone from the library comes back to iCloud as a new asset under its link",
			state(assets{a7}, pairs{alphaHolidays}, back), snapshot(), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{relink("R-LINK-02", holidays, imported(photo, e1, holidays))}}, delta(), nil},
		{"a file that the user moved out of its event into the year folder keeps its asset as the state has it there",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {looseFile}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir(looseDir)}}, delta(a7), nil},
		{"a new asset does not go up into a folder that differs from another folder in case alone, while a file still moves out of that folder to its new event",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip, n8), scans{"alpha": {youngH, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:     ops{moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")},
				Notices: []domain.Notice{doubles(holidaysDir, lowHolidays), settling(youngH.Path)},
			},
			delta(in7Trip, n8), nil},
		{"an asset only in a thematic album is not uploaded, with a notice",
			state(nil, pairs{alphaCats}), snapshot(cat1), nil, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedAsset("IMG_0001.HEIC", "uuid-1")}}, delta(cat1), nil},
		{"an edit does not go to a quarantine path that differs from a file in quarantine in case alone",
			state(nil, nil), snapshot(), scans{"alpha": {edit3, oldQ}}, specs{alpha}, "", edits, event,
			domain.Plan{Ops: ops{}, Notices: []domain.Notice{doubles(oldQ.Path, quarantined+edit3.Path), counted("plan.edits_left", "alpha", "1")}},
			delta(), nil},
		{"a file that would move into a folder that differs from another in case alone keeps its asset as the state has it in its remote, while the other remote takes the move of the user and closes its conflict",
			recorded(state(assets{a7}, pairs{alphaHolidays, alphaTrip, betaHolidays, betaTrip}, photo, other),
				conflict("beta", domain.ConflictStatusSkipped, ""), conflict("alpha", domain.ConflictStatusSkipped, "")),
			snapshot(a7), scans{"alpha": {youngT, e1}, "beta": {youngT, tripFile}}, specs{beta, alpha}, "", p, event,
			domain.Plan{
				Ops:       ops{holding(album(domain.OpAlbumAdd, 7, trip, "R-LINK-05"), took(other, tripFile, trip)), album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05")},
				Conflicts: []domain.Conflict{conflict("beta", domain.ConflictStatusResolved, "")},
				Notices:   []domain.Notice{doubles(tripDir, lowTrip), inBeta(doubles(tripDir, lowTrip)), settling(youngT.Path), inBeta(settling(youngT.Path))},
			},
			delta(a7), nil},
		{"under ask a photo that the user moved into the event that its asset is in now plans nothing while its Live Photo video still cannot follow it, and both links wait as the state has them",
			state(assets{withVideo(in7Trip)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(in7Trip)),
			scans{"alpha": {twinMov, ev, tripFile}}, specs{alpha}, "", ask, event,
			domain.Plan{Ops: ops{}, Notices: []domain.Notice{doubles(tripDir, lowTrip), doubles(tripDir+"/IMG_0001.mov", twinMov.Path), settling(twinMov.Path)}},
			delta(withVideo(in7Trip)), nil},
		{"a file kept in quarantine longer than the policy says leaves the cloud for good, while one kept as long and two that differ in case alone stay",
			state(nil, nil), snapshot(), scans{"alpha": {aged7x, kept6, aged7, aged5}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{expiring("alpha", aged5)}, Notices: []domain.Notice{doubles(aged7.Path, aged7x.Path)}},
			delta(), nil},
		{"one remote alone syncs: the quarantine of the other stays as it is",
			state(nil, nil), snapshot(), scans{"alpha": {aged5}, "beta": {aged5}}, specs{alpha, beta}, "alpha", p, event,
			domain.Plan{Ops: ops{expiring("alpha", aged5)}}, delta(), nil},
		{"a photo that the user changed while its asset is gone from the library comes back to iCloud as a new asset with its Live Photo video, in no album while the personal root sets no albums",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {ev, replaced}}, specs{alpha}, "", withoutPersonal, event,
			domain.Plan{Ops: ops{relink("R-LINK-11", 0, imported(photo, replaced, 0), imported(video, ev, 0))}}, delta(), nil},
		{"a file that the user changed while its asset is gone from the library comes back to iCloud by its link, while the same new file in another remote waits to be matched with the new asset",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo), snapshot(), scans{"alpha": {replaced}, "beta": {replaced}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops:     ops{relink("R-LINK-11", holidays, imported(photo, replaced, holidays))},
				Notices: []domain.Notice{counted("plan.waiting_import", "beta", "1")},
			},
			delta(), nil},
		{"a remote trusted with no hash imports a file that the user changed while its asset is gone from the library and a new file, though their listing gives one MD5",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(), scans{"alpha": {unhashed, replaced}}, specs{blind}, "", p, event,
			domain.Plan{Ops: ops{
				relink("R-LINK-11", holidays, imported(photo, replaced, holidays)),
				{Kind: domain.OpImport, Remote: "alpha", Root: personal, Src: unhashed.Path, AlbumID: holidays, Rule: "R-LINK-02", Links: []domain.Link{{
					Remote: "alpha", Root: personal, Path: unhashed.Path, Role: original, AlbumID: holidays, Size: 50,
					SHA256: "sb1", MD5: "b1", ETag: "eb1", MTime: settled, Level: domain.LevelImported, Status: domain.StatusActive,
				}}},
			}},
			delta(), nil},
		{"a photo that the user changed in place replaces its asset with one imported from it and from the Live Photo video brought back beside it, and one operation takes the old asset and that of an earlier replacement to Recently Deleted",
			remembered(state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, awayVideo), history), snapshot(withVideo(a7), restored),
			scans{"alpha": {ev, replaced}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				replacing("R-LINK-06", renewed(photo, replaced, holidays), renewed(video, ev, holidays)),
				album(domain.OpAlbumAdd, 7, holidays, "R-LINK-06"), album(domain.OpAssetDelete, 7, 0, "R-STATUS-08"),
			}},
			domain.SnapshotDelta{Assets: assets{withVideo(a7)}, Albums: albums, History: []domain.AssetHistory{history}}, nil},
		{"a photo that the user changed while its asset moved to another event goes there with its Live Photo video, and the asset is replaced from their new place",
			state(assets{withVideo(a7)}, pairs{alphaHolidays, alphaTrip}, photo, video), snapshot(withVideo(in7Trip)),
			scans{"alpha": {ev, replaced}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05"),
				moved(video, personal, tripDir+"/IMG_0001.mov", trip, "R-COMPONENT-04"),
				replacing("R-LINK-14", renewed(photo, replacedTrip, trip), renewed(video, movTrip, trip)),
				album(domain.OpAlbumAdd, 7, trip, "R-LINK-14"), album(domain.OpAssetDelete, 7, 0, "R-STATUS-08"),
			}},
			delta(withVideo(in7Trip)), nil},
		{"two earlier replacements left half done take their old assets to Recently Deleted in one operation, beside the other changes of the asset",
			remembered(state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), history, history5), snapshot(a7, restored, restored5),
			scans{"alpha": {tripFile}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{
				holding(album(domain.OpAlbumAdd, 7, trip, "R-LINK-05"), took(photo, tripFile, trip)),
				album(domain.OpAlbumRemove, 7, holidays, "R-LINK-05"), album(domain.OpAssetDelete, 7, 0, "R-STATUS-08"),
			}},
			domain.SnapshotDelta{Assets: assets{a7}, Albums: albums, History: []domain.AssetHistory{history5, history}}, nil},
		{"a hidden asset whose file the user changed in place is not replaced, with a notice",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(hidden7), scans{"alpha": {replaced}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{{Code: "plan.hidden_not_replaced", Params: []string{"IMG_0001.HEIC", "uuid-7"}}}},
			delta(hidden7), nil},
		{"a photo and its Live Photo video that the user changed while their asset is gone from the library come back to iCloud as one new asset, and nothing replaces the asset that is gone",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), snapshot(), scans{"alpha": {changedMov, replaced}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{relink("R-LINK-11", holidays, imported(photo, replaced, holidays), imported(video, changedMov, holidays))}}, delta(), nil},
		{"an earlier replacement left half done takes its old asset to Recently Deleted while the asset that replaced it is gone from the library and has no link",
			remembered(domain.State{Assets: assets{a7}, Albums: albums}, history), snapshot(restored), nil, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{album(domain.OpAssetDelete, 7, 0, "R-STATUS-08")}},
			domain.SnapshotDelta{Albums: albums, History: []domain.AssetHistory{history}}, nil},
		{"a conflict that the state does not keep is settled by the conflict policy with a notice: under icloud_wins the file follows the asset to its new event",
			state(assets{a7}, pairs{betaHolidays, betaTrip}, other), snapshot(in7Trip), scans{"beta": {partyFile, holidaysFolder}}, specs{beta}, "", iCloudWins, event,
			domain.Plan{
				Ops:     ops{creating(party), moved(partyOther, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")},
				Albums:  []domain.Album{partyAlbum},
				Pairs:   pairs{pair(party, "beta", personal, partyDir, domain.OriginRemote)},
				Notices: []domain.Notice{{Code: "plan.conflict_settled", Params: []string{"beta", "IMG_0001.HEIC", "uuid-7", "icloud_wins"}}},
			},
			delta(in7Trip), nil},
		{"upload-only: new files of the cloud stay out of iCloud and an edit stays where it is, and notices count them",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7), scans{"alpha": {new4, e1, edit2, new3}}, specs{alphaUp}, "", p, event,
			domain.Plan{Notices: []domain.Notice{counted("plan.edits_left", "alpha", "1"), counted("plan.not_imported", "alpha", "2")}}, delta(a7), nil},
		{"new files that one remote imports wait in another for a later run with a notice of their number, while that remote imports a new file of its own, brings a changed file back by its link and sets an edit aside",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, other), snapshot(),
			scans{"alpha": {new4, new3}, "beta": {new5, edit2, new4, replaced, new3}}, specs{alpha, beta}, "", edits, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpQuarantine, Remote: "beta", Root: personal, Src: edit2.Path, DstRoot: personal, Dst: quarantined + edit2.Path, Rule: "R-STATUS-10"},
					importNew("alpha", new3), importNew("alpha", new4),
					relink("R-LINK-11", holidays, imported(other, replaced, holidays)), importNew("beta", new5),
				},
				Notices: []domain.Notice{counted("plan.waiting_import", "beta", "2")},
			},
			delta(), nil},
		{"an edit stays where it is while a lesser copy that matching finds goes to quarantine, and a notice counts the edit",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, m8), scans{"alpha": {edit2, lesser, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpQuarantine, Remote: "alpha", Root: personal, Src: lesser.Path, DstRoot: personal, Dst: parked.QuarantinePath,
						Asset: 8, Rule: "R-OBJECT-10", Links: []domain.Link{parked}},
					swapped,
					album(domain.OpAlbumAdd, 8, holidays, "R-LINK-03"),
				},
				Notices: []domain.Notice{counted("plan.edits_left", "alpha", "1")},
			},
			delta(a7, m8), nil},
		{"while the personal root sets no albums, files that the user moved there leave the albums of their assets as they are, and a notice counts the moves that the base takes",
			state(assets{a7, h8}, pairs{betaHolidays, betaTrip}, other, kept8), snapshot(a7, h8), scans{"beta": {trip2, tripFile}}, specs{beta}, "", withoutPersonal, event,
			domain.Plan{
				Links:   []domain.Link{took(other, tripFile, holidays), took(kept8, trip2, holidays)},
				Notices: []domain.Notice{counted("plan.albums_kept", "beta", "2")},
			},
			delta(a7, h8), nil},
		{"an album renamed in iCloud takes its pair to the folder of its new title, the files of its folder follow with their Live Photo video and sidecar, and a new asset of the album goes up there",
			state(assets{withVideo(a7)}, pairs{alphaHolidays}, photo, video), inVacation(snapshot(withVideo(a7), n8)),
			scans{"alpha": {xmp, ev, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					moved(photo, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11"),
					moved(video, personal, vacationDir+"/IMG_0001.mov", holidays, "R-COMPONENT-04"),
					{Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: xmp.Path, DstRoot: personal, Dst: vacationDir + "/IMG_0001.xmp", Asset: 7, Rule: "R-COMPONENT-04"},
					put(8, vacationDir+"/IMG_0001 (1).HEIC", original, holidays, 200), put(8, vacationDir+"/IMG_0001 (1).mov", live, holidays, 30),
				},
				Pairs: pairs{vacationPair},
			},
			domain.SnapshotDelta{Assets: assets{withVideo(a7), n8}, Albums: vacationAlbums}, nil},
		{"an album renamed in iCloud takes its pair along, while the file of an asset that left the album goes to the folder of its new event",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), inVacation(snapshot(in7Trip)), scans{"alpha": {e1}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")}, Pairs: pairs{vacationPair}},
			domain.SnapshotDelta{Assets: assets{in7Trip}, Albums: vacationAlbums}, nil},
		{"one remote alone syncs: an album renamed in iCloud takes its pair and its file to the folder of its new title there, and the pair of the other remote stays",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), inVacation(snapshot(a7)), scans{"alpha": {e1}, "beta": {e1}}, specs{alpha, beta}, "alpha", p, event,
			domain.Plan{Ops: ops{moved(photo, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11")}, Pairs: pairs{vacationPair}},
			domain.SnapshotDelta{Assets: assets{a7}, Albums: vacationAlbums}, nil},
		{"a full run after one remote alone took an album renamed in iCloud takes the pair and the file of the other remote to the folder of its new title",
			domain.State{Assets: assets{a7}, Albums: vacationAlbums, AlbumPairs: pairs{vacationPair, betaHolidays}, Links: []domain.Link{atVacation, other}},
			inVacation(snapshot(a7)), scans{"alpha": {file(personal, vacationDir+"/IMG_0001.HEIC", 100, "a1", settled)}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops:   ops{moved(other, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11")},
				Pairs: pairs{pair(holidays, "beta", personal, vacationDir, domain.OriginICloud)},
			},
			domain.SnapshotDelta{Assets: assets{a7}, Albums: vacationAlbums}, nil},
		{"an album renamed in iCloud takes along its pair in the shared root, which keeps its origin, and the file there follows",
			state(assets{a7, s9}, pairs{alphaHolidays, pair(holidays, "alpha", shared, holidaysDir, domain.OriginRemote)}, photo, link9), inVacation(snapshot(a7, s9)),
			scans{"alpha": {file9, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					moved(photo, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11"),
					{Kind: domain.OpMove, Remote: "alpha", Root: shared, Src: file9.Path, DstRoot: shared, Dst: moved9.Path, Asset: 9, Rule: "R-LAYOUT-11", Links: []domain.Link{moved9}},
				},
				Pairs: pairs{vacationPair, pair(holidays, "alpha", shared, vacationDir, domain.OriginRemote)},
			},
			domain.SnapshotDelta{Assets: assets{a7, s9}, Albums: vacationAlbums}, nil},
		{"a pair that the run makes for the folder of the old title of a renamed album is named otherwise and keeps that folder, while the pair of the state follows the album",
			state(assets{a7, s9}, pairs{alphaHolidays}, photo, link9), inVacation(snapshot(a7, s9)), scans{"alpha": {file9, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:   ops{moved(photo, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11")},
				Pairs: pairs{{AlbumID: holidays, Remote: "alpha", Root: shared, DirPath: holidaysDir, CreatedBy: domain.OriginRemote}, vacationPair},
			},
			domain.SnapshotDelta{Assets: assets{a7, s9}, Albums: vacationAlbums}, nil},
		{"a folder of an album that the user renamed in the cloud renames the album in iCloud and takes its pair along, and a new asset of the album goes up there, while new assets of another album and of the shared root go to the folders of their pairs",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip, sharedHolidays}, photo), snapshot(a7, h8, new9, new10), scans{"alpha": {vacationFile}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					renaming, sharedPut, put(8, vacationDir+"/IMG_0002.HEIC", original, holidays, 200),
					put(10, tripDir+"/IMG_0010.HEIC", original, trip, 1000),
				},
				Pairs: pairs{vacationPair},
				Links: []domain.Link{took(photo, vacationFile, holidays)},
			},
			delta(a7, h8, new9, new10), nil},
		{"a folder that the user renamed in one remote renames the album, and the folder of another remote follows the new title in the same run",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), snapshot(a7), scans{"alpha": {vacationFile}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops:   ops{renaming, moved(other, personal, vacationDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11")},
				Pairs: pairs{vacationPair, pair(holidays, "beta", personal, vacationDir, domain.OriginICloud)},
				Links: []domain.Link{took(photo, vacationFile, holidays)},
			},
			delta(a7), nil},
		{"a folder of an album that the user moved to another year in the cloud moves the album into a year folder that iCloud adds, takes its pair along, and the folder of another remote follows in the same run",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), snapshot(a7), scans{"alpha": {laterFile, yearFolder}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpAlbumCreate, AlbumID: folder2023, Rule: "R-LAYOUT-06"}, refiling,
					moved(other, personal, laterDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11"),
				},
				Albums: []domain.Album{{ID: folder2023, Kind: domain.KindFolder, Title: "2023", FolderTitles: []string{}}},
				Pairs:  pairs{pair(holidays, "alpha", personal, laterDir, domain.OriginICloud), pair(holidays, "beta", personal, laterDir, domain.OriginICloud)},
				Links:  []domain.Link{took(photo, laterFile, holidays)},
			},
			delta(a7), nil},
		{"a year folder that the user renamed in one remote takes the new title in iCloud, and the folder of its album in another remote follows in the same run",
			state(assets{a7}, pairs{alphaHolidays, betaHolidays}, photo, other), snapshot(a7), scans{"alpha": {laterFile}, "beta": {e1}}, specs{alpha, beta}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpAlbumRename, AlbumID: year, Dst: "2023", Rule: "R-LAYOUT-14"},
					moved(other, personal, laterDir+"/IMG_0001.HEIC", holidays, "R-LAYOUT-11"),
				},
				Pairs: pairs{pair(holidays, "alpha", personal, laterDir, domain.OriginICloud), pair(holidays, "beta", personal, laterDir, domain.OriginICloud)},
				Links: []domain.Link{took(photo, laterFile, holidays)},
			},
			delta(a7), nil},
		{"a year folder that the user renamed takes the new title in iCloud, and a new asset of an event in it goes up to the folder of its pair",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, h8), scans{"alpha": {laterFile}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpAlbumRename, AlbumID: year, Dst: "2023", Rule: "R-LAYOUT-14"},
					put(8, laterDir+"/IMG_0002.HEIC", original, holidays, 200),
				},
				Pairs: pairs{pair(holidays, "alpha", personal, laterDir, domain.OriginICloud)},
				Links: []domain.Link{took(photo, laterFile, holidays)},
			},
			delta(a7, h8), nil},
		{"an event without a description gets the bare day",
			domain.State{Albums: bareAlbums}, onBareDay(snapshot(bare1)), nil, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:   ops{put(1, bareDir+"/IMG_0001.HEIC", original, bareDay, 100)},
				Pairs: pairs{pair(bareDay, "alpha", personal, bareDir, domain.OriginICloud)},
			},
			domain.SnapshotDelta{Assets: assets{bare1}, Albums: bareAlbums}, nil},
		{"a former day folder reads as an event without a description",
			state(nil, nil), snapshot(), scans{"alpha": {dayNew}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops: ops{
					{Kind: domain.OpAlbumCreate, AlbumID: party, Rule: "R-LAYOUT-06"},
					relink("R-LINK-02", party, imported(domain.Link{Remote: "alpha", Role: original, Status: domain.StatusActive}, dayNew, party)),
				},
				Albums: []domain.Album{{ID: party, Kind: domain.KindEvent, Title: "05/01", ParentID: year, FolderTitles: []string{"2024"}}},
				Pairs:  pairs{pair(party, "alpha", personal, bareDir, domain.OriginRemote)},
			},
			delta(), nil},
		{"upload-only: a new file that matching links in the folder of another album moves to the album of its asset with its Live Photo video, and both links go with the moves",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(a7, live8trip), scans{"alpha": {e2v, e2, e1}}, specs{alphaUp}, "", p, event,
			domain.Plan{Ops: ops{
				{Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: e2.Path, DstRoot: personal, Dst: tripDir + "/IMG_0002.HEIC",
					Asset: 8, Rule: "R-OBJECT-05", Links: []domain.Link{took(paired, file(personal, tripDir+"/IMG_0002.HEIC", 200, "b2", settled), trip)}},
				{Kind: domain.OpMove, Remote: "alpha", Root: personal, Src: e2v.Path, DstRoot: personal, Dst: tripDir + "/IMG_0002.mov",
					Asset: 8, Rule: "R-COMPONENT-04", Links: []domain.Link{pairedVideo}},
			}},
			delta(a7, live8trip), nil},
		{"new files in the root, in the year folder and in the directory of a thematic album are skipped with a notice for each directory", // covers: R-LAYOUT-03
			state(nil, pairs{alphaHolidays}), snapshot(), scans{"alpha": {newInCats, newInYear, newAtRoot}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir("."), skippedDir(looseDir), skippedDir(catsDir)}}, delta(), nil},
		{"a new file in a directory below an event is skipped", // covers: R-LAYOUT-09
			state(nil, pairs{alphaHolidays}), snapshot(), scans{"alpha": {newInRaw}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir(rawDir)}}, delta(), nil},
		{"a linked file that the user moved below its event keeps its asset as the state has it there, by its content",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip), scans{"alpha": {rawFile}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir(rawDir)}}, delta(in7Trip), nil},
		{"without a hash a linked file that the user moved below its event reads as deleted",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(a7), scans{"alpha": {rawFile}}, specs{blind}, "", p, event,
			domain.Plan{
				Ops:     ops{{Kind: domain.OpAssetDelete, Asset: 7, Rule: "R-STATUS-02", Links: []domain.Link{lost}}},
				Notices: []domain.Notice{skippedDir(rawDir)},
			},
			delta(a7), nil},
		{"a copy below its event leaves the asset in the run while its file stays where it is",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip), scans{"alpha": {rawFile, e1}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:     ops{moved(photo, personal, tripDir+"/IMG_0001.HEIC", trip, "R-OBJECT-05")},
				Notices: []domain.Notice{skippedDir(rawDir)},
			},
			delta(in7Trip), nil},
		{"a slash in a description stays in one directory",
			domain.State{Albums: slashAlbums}, slashShot, nil, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:   ops{put(1, slashDir+"/IMG_0001.HEIC", original, slash, 100)},
				Pairs: pairs{pair(slash, "alpha", personal, slashDir, domain.OriginICloud)},
			},
			domain.SnapshotDelta{Assets: assets{slash1}, Albums: slashAlbums}, nil},
		{"an asset that left its only event, the only one of its day, does not join it again", // covers: R-LAYOUT-05
			state(assets{left7}, pairs{alphaHolidays}), snapshot(left7), nil, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedAsset("IMG_0001.HEIC", "uuid-7")}}, delta(left7), nil},
		{"an asset in no album, with no event on its day, is skipped with a notice", // covers: R-LAYOUT-02
			state(nil, pairs{alphaHolidays}), snapshot(alone1), nil, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedAsset("IMG_0001.HEIC", "uuid-1")}}, delta(alone1), nil},
		{"a linked file that the user moved into a directory of edits keeps its asset as the state has it there",
			state(assets{a7}, pairs{alphaHolidays, alphaTrip}, photo), snapshot(in7Trip), scans{"alpha": {editedFile}}, specs{alpha}, "", p, event,
			domain.Plan{}, delta(in7Trip), nil},
		{"upload-only: a file of the user that matching finds for an asset in no event stays where it is, without a link, and is reported",
			state(nil, pairs{alphaHolidays}), snapshot(alone2), scans{"alpha": {lone2}}, specs{alphaUp}, "", p, event,
			domain.Plan{Notices: []domain.Notice{counted("plan.not_imported", "alpha", "1")}}, delta(alone2), nil},
		{"a new file in the directory of the pair of an album that left the templates goes into that album",
			state(nil, pairs{alphaHolidays}), themedSnapshot, scans{"alpha": {new3}}, specs{alpha}, "", p, event,
			domain.Plan{Ops: ops{importNew("alpha", new3)}}, domain.SnapshotDelta{Albums: themedAlbums}, nil},
		{"an asset that the state holds after its links have left gets no first placement",
			state(assets{n1}, pairs{alphaHolidays}), snapshot(n1), nil, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedAsset("IMG_0001.HEIC", "uuid-1")}}, delta(n1), nil},
		{"a new asset does not take the path of a linked file that the user moved below its event",
			state(assets{a7}, pairs{alphaHolidays}, photo), snapshot(a7, n8), scans{"alpha": {rawFile}}, specs{alpha}, "", p, event,
			domain.Plan{
				Ops:     ops{put(8, holidaysDir+"/IMG_0001 (1).HEIC", original, holidays, 200), put(8, holidaysDir+"/IMG_0001 (1).mov", live, holidays, 30)},
				Notices: []domain.Notice{skippedDir(rawDir)},
			},
			delta(a7, n8), nil},
		{"a linked file named as an edit in the directory of a thematic album keeps its asset as the state has it there",
			state(assets{a7}, pairs{alphaHolidays}, inCatsEdit), snapshot(a7), scans{"alpha": {catsEdit}}, specs{alpha}, "", p, event,
			domain.Plan{Notices: []domain.Notice{skippedDir(catsDir)}}, delta(a7), nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.Parse(c.tmpl)
			if err != nil {
				t.Fatalf("template.Parse(%q): %v", c.tmpl, err)
			}
			in := plan.Input{
				State: c.state, Snapshot: c.snapshot, Scans: c.scans, Remotes: c.remotes, OnlyRemote: c.only, Policies: c.p,
				Template: tmpl, Fields: fieldsOf(c.snapshot), Now: now, RunID: "run-2",
			}
			input := fmt.Sprintf("%+v", in)
			got, d, err := plan.Build(in)
			if !reflect.DeepEqual(got, c.plan) || !reflect.DeepEqual(d, c.delta) || !errors.Is(err, c.err) {
				t.Errorf("Build =\n%s\nwant\n%s", show(got, d, err), show(c.plan, c.delta, c.err))
			}
			for i := range got.Ops {
				for j := i + 1; j < len(got.Ops); j++ {
					if got.Ops[i].Compare(got.Ops[j]) == 0 {
						t.Errorf("ops %d and %d of the plan are equal by Compare", i+1, j+1)
					}
				}
			}
			again, more, failed := plan.Build(reversed(in))
			if !reflect.DeepEqual(again, got) || !reflect.DeepEqual(more, d) || fmt.Sprint(failed) != fmt.Sprint(err) {
				t.Errorf("with the input and the remotes reversed:\n%s\nwant\n%s", show(again, more, failed), show(got, d, err))
			}
			for _, o := range slices.Concat(got.Ops, again.Ops) {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			stored, seen := map[domain.LinkID]bool{}, map[domain.LinkID]bool{}
			for _, l := range c.state.Links {
				stored[l.ID] = true
			}
			links := slices.Clone(got.Links)
			for _, o := range got.Ops {
				links = append(links, o.Links...)
			}
			for _, l := range links {
				// The upload that replaces the file of a new link carries the link once
				// more, as an upload.
				if l.ID == 0 || stored[l.ID] || l.Level == domain.LevelUploaded {
					continue
				}
				if seen[l.ID] {
					t.Errorf("new link %d is more than once among the links of the plan", l.ID)
				}
				seen[l.ID] = true
			}
			for _, o := range got.Ops {
				for i := range o.Links {
					o.Links[i].Path = "changed"
				}
			}
			for _, a := range got.Albums {
				for i := range a.FolderTitles {
					a.FolderTitles[i] = "changed"
				}
			}
			if after := fmt.Sprintf("%+v", in); after != input {
				t.Errorf("Build changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
