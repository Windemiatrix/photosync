package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row is the state, the snapshot reconciled with it and the scans of a
// run. The expected files, links, albums, pairs, new files, edits, settling
// assets and notices are written from the rules on settling, matching, album
// pairs, derived copies and modes, not from the code, and must not depend on
// the order of the input.
func TestReadScans(t *testing.T) {
	t.Parallel()
	const (
		personal, shared           = domain.LibraryPersonal, domain.LibraryShared
		original, liveVideo        = domain.RoleOriginal, domain.RoleLiveVideo
		exact, sameShot, component = domain.LevelExact, domain.LevelSameShot, domain.LevelPairComponent
		folder, event              = domain.KindFolder, domain.KindEvent
		byRemote                   = domain.OriginRemote
		pairRule                   = "R-LINK-03"

		year                 domain.AlbumID = 1
		holidays, trip, gone domain.AlbumID = 10, 11, 20
		next                 domain.AlbumID = 21

		holidaysDir, tripDir = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		partyDir, beachDir   = "2024/05-04 - Party", "2024/05-05 - Beach"
		picnicDir, copiesDir = "2024/05-07 - Picnic", "2024/05-08 - Copies"
		movedDir, zooDir     = "2024/05-09 - Moved", "2024/05-10 - Zoo"
		photosDir, picsDir   = "2024/Trip photos", "2024/Trip pics"
	)
	type (
		decided = []domain.LinkDecision
		joins   = map[domain.AssetID][]domain.AlbumChange
		found   = map[domain.LinkID]domain.RemoteEntry
		reads   = map[string]plan.RemoteScan
		entries = []domain.RemoteEntry
		news    = []plan.NewFile
		scans   = map[string][]domain.RemoteEntry
		specs   = []domain.RemoteSpec
	)
	type result struct {
		files   found
		read    reads
		added   []domain.Album
		made    []domain.AlbumPair
		notices []domain.Notice
	}
	now := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	settled, young := now.Add(-time.Hour), now.Add(-5*time.Minute)
	taken := time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC)

	file := func(root domain.LibraryKind, at string, size int64, md5 string, mtime time.Time) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: size, MD5: md5, SHA256: "s" + md5, ETag: "e" + md5, MTime: mtime}
	}
	shot := func(e domain.RemoteEntry, width, height int) domain.RemoteEntry {
		e.Meta = domain.MediaMeta{Taken: taken, Width: width, Height: height}
		return e
	}
	photo := func(id domain.AssetID, name string, size int64, album domain.AlbumID) domain.Asset {
		return domain.Asset{
			ID: id, Key: domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", id), UUID: fmt.Sprintf("uuid-%d", id)},
			Library: personal, TargetLibrary: personal, OriginalName: name, Taken: taken, Width: 4032, Height: 3024,
			Resources:   []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
			Memberships: []domain.Membership{{AlbumID: album, FirstSeenRun: "run-1"}},
		}
	}
	link := func(id domain.LinkID, asset domain.AssetID, remote string, e domain.RemoteEntry, role domain.LinkRole, album domain.AlbumID) domain.Link {
		return domain.Link{
			ID: id, AssetID: asset, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: album,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, Level: exact, Status: domain.StatusActive,
		}
	}
	album := func(id domain.AlbumID, kind domain.AlbumKind, title string) domain.Album {
		if kind == folder {
			return domain.Album{ID: id, Kind: kind, Title: title}
		}
		return domain.Album{ID: id, Kind: kind, Title: title, ParentID: year, FolderTitles: []string{"2024"}}
	}
	lib := []domain.Album{album(year, folder, "2024"), album(holidays, event, "05/01 Holidays"), album(trip, event, "05/02 Trip")}
	stateAlbums := append(slices.Clone(lib), album(gone, event, "05/09 Gone"))
	pair := func(id domain.AlbumID, remote string, root domain.LibraryKind, dir string, by domain.PairOrigin) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, Remote: remote, Root: root, DirPath: dir, CreatedBy: by}
	}
	state := func(links ...domain.Link) domain.State {
		return domain.State{Albums: stateAlbums, Links: links, AlbumPairs: []domain.AlbumPair{
			pair(holidays, "alpha", personal, holidaysDir, domain.OriginICloud), pair(holidays, "beta", personal, holidaysDir, domain.OriginICloud),
		}}
	}
	delta := func(assets ...domain.Asset) domain.SnapshotDelta {
		return domain.SnapshotDelta{Assets: assets, Albums: lib}
	}
	spec := func(name string, mode domain.Mode, hash domain.HashKind) domain.RemoteSpec {
		return domain.RemoteSpec{Name: name, Mode: mode, Capabilities: domain.Capabilities{ContentHash: hash}}
	}
	alpha, plain := spec("alpha", domain.ModeTwoWay, domain.HashMD5), spec("alpha", domain.ModeTwoWay, domain.HashNone)
	beta, betaUp := spec("beta", domain.ModeDownloadOnly, domain.HashMD5), spec("beta", domain.ModeUploadOnly, domain.HashMD5)
	p := domain.DefaultPolicies()
	p.Deletes, p.UntitledEvent, p.AlbumsRoot = domain.DeletesQuarantine, "Untitled", "Albums"
	skip := p
	skip.Hidden = domain.HiddenSkip

	made := func(id domain.LinkID, asset domain.AssetID, remote string, e domain.RemoteEntry, role domain.LinkRole, level domain.MatchLevel, album domain.AlbumID) domain.LinkDecision {
		return domain.LinkDecision{
			Link: domain.Link{
				ID: id, AssetID: asset, Remote: remote, Root: e.Root, Path: e.Path, Role: role, AlbumID: album,
				Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, MTime: e.MTime, Level: level, Status: domain.StatusActive,
			},
			ICloud: domain.ChangeNew, Remote: domain.ChangeNew, Action: domain.ActionLink, Rule: pairRule,
		}
	}
	replaced := func(d domain.LinkDecision) domain.LinkDecision {
		d.Action, d.Rule = domain.ActionReplaceFile, "R-OBJECT-10"
		return d
	}
	newFile := func(e domain.RemoteEntry, action domain.LinkAction, rule string, album domain.AlbumID) plan.NewFile {
		return plan.NewFile{
			File:     match.Candidate{Main: e, Role: original},
			Decision: domain.LinkDecision{ICloud: domain.ChangeNone, Remote: domain.ChangeNew, Action: action, Rule: rule},
			AlbumID:  album,
		}
	}
	imported := func(e domain.RemoteEntry, album domain.AlbumID) plan.NewFile {
		return newFile(e, domain.ActionImport, "R-LINK-02", album)
	}
	reported := func(e domain.RemoteEntry) plan.NewFile { return newFile(e, domain.ActionNone, "R-MODE-01", 0) }
	into := func(album domain.AlbumID) []domain.AlbumChange {
		return []domain.AlbumChange{{AlbumID: album, Rule: pairRule}}
	}
	scanned := func(linked decided, join joins, newFiles news, edited entries, settling ...domain.AssetID) plan.RemoteScan {
		s := plan.RemoteScan{Linked: linked, Join: join, NewFiles: newFiles, Edited: edited, Settling: map[domain.AssetID]bool{}}
		for _, id := range settling {
			s.Settling[id] = true
		}
		return s
	}
	settling := func(root domain.LibraryKind, at string) domain.Notice {
		return domain.Notice{Code: "plan.settling", Params: []string{"alpha", string(root), at}}
	}
	notice := func(code string, root domain.LibraryKind, at string) domain.Notice {
		return domain.Notice{Code: code, Params: []string{string(root), at}}
	}

	e1 := file(personal, holidaysDir+"/IMG_0001.HEIC", 100, "a1", settled)
	e1z := e1
	e1z.MTime = time.Time{}
	e2, e2v := shot(file(personal, photosDir+"/IMG_0002.HEIC", 200, "b2", settled), 4032, 3024), file(personal, photosDir+"/IMG_0002.MOV", 210, "b3", settled)
	e3 := shot(file(personal, holidaysDir+"/IMG_0003.jpg", 250, "c3", settled), 1600, 1200)
	e4, f5 := file(personal, partyDir+"/IMG_0004.HEIC", 400, "d4", settled), file(personal, partyDir+"/IMG_0005.HEIC", 500, "f5", settled)
	e4v, e4r := file(personal, partyDir+"/IMG_0004.MOV", 410, "d5", settled), file(personal, partyDir+"/IMG_0004.DNG", 420, "d6", settled)
	e4x := file(personal, partyDir+"/IMG_0004.XMP", 10, "d7", settled)
	e12 := shot(file(personal, holidaysDir+"/IMG_0012.jpg", 900, "g2", settled), 8064, 6048)
	beach, sBeach := file(shared, beachDir+"/IMG_0005.HEIC", 500, "e5", settled), file(personal, beachDir+"/IMG_0005.HEIC", 500, "e5", settled)
	edit, notes := file(personal, holidaysDir+"/edited/IMG_0001.HEIC", 90, "x1", settled), file(personal, "2024/notes.txt", 5, "n1", settled)
	q13, q14 := file(personal, picsDir+"/IMG_0013.HEIC", 1300, "j3", settled), file(personal, picsDir+"/IMG_0014.HEIC", 1400, "j4", settled)
	y4, yBeach := file(personal, partyDir+"/IMG_0004.HEIC", 400, "d4", young), file(shared, beachDir+"/IMG_0005.HEIC", 500, "e5", young)
	yDir := domain.RemoteEntry{Root: personal, Path: "2024/05-06 - New", IsDir: true, MTime: young}
	picnic, picnicMOV := file(personal, picnicDir+"/IMG_0001.HEIC", 100, "a1", settled), file(personal, picnicDir+"/IMG_0001.MOV", 300, "m3", settled)
	yMOV := file(personal, picnicDir+"/IMG_0001.MOV", 330, "n3", young)
	y1, copied := file(personal, holidaysDir+"/IMG_0001.HEIC", 120, "b1", young), file(personal, copiesDir+"/IMG_0001.HEIC", 100, "a1", settled)
	yMoved, twin := file(personal, movedDir+"/IMG_0001.HEIC", 100, "a1", young), file(personal, tripDir+"/IMG_0009.HEIC", 100, "a1", settled)
	zoo := file(personal, zooDir+"/IMG_0001.HEIC", 100, "a1", settled)
	e30, e31 := file(personal, "2024/05-11 - Hidden/IMG_0030.HEIC", 300, "h0", settled), file(personal, "2024/05-12 - Trash/IMG_0031.HEIC", 310, "h1", settled)
	e32d, e32 := file(personal, holidaysDir+"/IMG_0032.HEIC", 320, "h2", settled), file(personal, "2024/05-13 - Frozen/IMG_0032.HEIC", 320, "h2", settled)
	e34 := file(personal, "2024/05-14 - Gone/IMG_0034.HEIC", 340, "h4", settled)
	e40, e41 := file(personal, holidaysDir+"/IMG_0040.HEIC", 400, "z1", settled), file(personal, holidaysDir+"/IMG_0041.HEIC", 400, "z1", settled)
	z := file(personal, tripDir+"/IMG_0040.HEIC", 400, "z1", settled)

	a7, a8, a9 := photo(7, "IMG_0001.HEIC", 100, holidays), photo(8, "IMG_0002.HEIC", 200, trip), photo(9, "IMG_0003.HEIC", 300, holidays)
	a12, a13, a14 := photo(12, "IMG_0012.HEIC", 1200, holidays), photo(13, "IMG_0013.HEIC", 1300, trip), photo(14, "IMG_0014.HEIC", 1400, trip)
	live8 := photo(8, "IMG_0002.HEIC", 200, trip)
	live8.Resources = append(live8.Resources, domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0002.MOV"})
	hidden, trashed, frozen := photo(30, "IMG_0030.HEIC", 300, holidays), photo(31, "IMG_0031.HEIC", 310, holidays), photo(32, "IMG_0032.HEIC", 320, holidays)
	hidden.Hidden, trashed.InTrash = true, true
	a40, a41 := photo(40, "IMG_0040.HEIC", 400, holidays), photo(41, "IMG_0041.HEIC", 400, holidays)
	detached := link(32, 32, "alpha", e32d, original, holidays)
	detached.Status = domain.StatusDetached

	stateTwo := state(link(1, 7, "alpha", e1, original, holidays), link(6, 7, "beta", e1, original, holidays))
	deltaTwo, scansTwo := delta(a9, a7), scans{"alpha": {e4, e3, e1}, "beta": {f5, e3, e1}}
	alphaTwo := scanned(decided{replaced(made(7, 9, "alpha", e3, original, sameShot, holidays))}, joins{9: into(holidays)}, news{imported(e4, next)}, nil)
	two := result{found{1: e1, 6: e1}, reads{
		"alpha": alphaTwo,
		"beta":  scanned(decided{made(8, 9, "beta", e3, original, sameShot, holidays)}, joins{9: into(holidays)}, news{imported(f5, next)}, nil),
	}, []domain.Album{album(next, event, "05/04 Party")},
		[]domain.AlbumPair{pair(next, "alpha", personal, partyDir, byRemote), pair(next, "beta", personal, partyDir, byRemote)}, nil}
	twoUp := result{two.files, reads{
		"alpha": alphaTwo,
		"beta":  scanned(decided{replaced(made(8, 9, "beta", e3, original, sameShot, holidays))}, nil, news{reported(f5)}, nil),
	}, two.added, two.made, nil}
	party := imported(e4, next)
	party.File.Live, party.File.RAW, party.File.Sidecars = &e4v, &e4r, entries{e4x}

	same := func(x, y result) bool {
		sameScan := func(a, b plan.RemoteScan) bool {
			return slices.Equal(a.Linked, b.Linked) && maps.EqualFunc(a.Join, b.Join, slices.Equal[[]domain.AlbumChange]) &&
				reflect.DeepEqual(a.NewFiles, b.NewFiles) && slices.Equal(a.Edited, b.Edited) && maps.Equal(a.Settling, b.Settling)
		}
		sameNotice := func(a, b domain.Notice) bool { return a.Code == b.Code && slices.Equal(a.Params, b.Params) }
		return maps.Equal(x.files, y.files) && maps.EqualFunc(x.read, y.read, sameScan) &&
			fmt.Sprintf("%+v", x.added) == fmt.Sprintf("%+v", y.added) && slices.Equal(x.made, y.made) &&
			slices.EqualFunc(x.notices, y.notices, sameNotice)
	}
	show := func(x result) string {
		var out []string
		for _, id := range slices.Sorted(maps.Keys(x.files)) {
			out = append(out, fmt.Sprintf("file %d %+v", id, x.files[id]))
		}
		for _, name := range slices.Sorted(maps.Keys(x.read)) {
			s := x.read[name]
			for _, d := range s.Linked {
				out = append(out, fmt.Sprintf("%s link %+v %s/%s %s %s", name, d.Link, d.ICloud, d.Remote, d.Action, d.Rule))
			}
			for _, id := range slices.Sorted(maps.Keys(s.Join)) {
				out = append(out, fmt.Sprintf("%s join %d %v", name, id, s.Join[id]))
			}
			for _, n := range s.NewFiles {
				f := fmt.Sprintf("%s new %+v %s", name, n.File.Main, n.File.Role)
				for _, e := range []*domain.RemoteEntry{n.File.Live, n.File.RAW} {
					if e != nil {
						f += " with " + e.Path
					}
				}
				for _, e := range n.File.Sidecars {
					f += " sidecar " + e.Path
				}
				out = append(out, fmt.Sprintf("%s #%d %s/%s %s %s", f, n.AlbumID, n.Decision.ICloud, n.Decision.Remote, n.Decision.Action, n.Decision.Rule))
			}
			for _, e := range s.Edited {
				out = append(out, fmt.Sprintf("%s edit %+v", name, e))
			}
			for _, id := range slices.Sorted(maps.Keys(s.Settling)) {
				out = append(out, fmt.Sprintf("%s settling %d", name, id))
			}
		}
		for _, a := range x.added {
			out = append(out, fmt.Sprintf("album %+v", a))
		}
		for _, pr := range x.made {
			out = append(out, fmt.Sprintf("pair %d %s %s:%s %s", pr.AlbumID, pr.Remote, pr.Root, pr.DirPath, pr.CreatedBy))
		}
		for _, n := range x.notices {
			out = append(out, fmt.Sprintf("notice %s %q", n.Code, n.Params))
		}
		return fmt.Sprintf("%q", out)
	}

	for _, c := range []struct {
		name    string
		state   domain.State
		delta   domain.SnapshotDelta
		scans   scans
		remotes specs
		p       domain.Policies
		want    result
	}{
		{"nothing at all", domain.State{}, domain.SnapshotDelta{}, nil, nil, p, result{}},
		{"two remotes take link and album IDs in the order of their names, and download-only keeps a lesser copy",
			stateTwo, deltaTwo, scansTwo, specs{beta, alpha}, p, two},
		{"upload-only replaces a lesser copy in place, but its new link joins no album and its new file is only reported",
			stateTwo, deltaTwo, scansTwo, specs{betaUp, alpha}, p, twoUp},
		{"one remote: a pair by the files of an album, both roots, an edit, an unknown file, a better cloud copy and a whole new shot",
			state(link(1, 7, "alpha", e1, original, holidays)), delta(a12, a8, a7),
			scans{"alpha": {notes, e4x, beach, edit, e4r, e2, e4, e12, e4v, e1}}, specs{alpha}, p,
			result{found{1: e1}, reads{"alpha": scanned(
				decided{made(2, 12, "alpha", e12, original, sameShot, holidays), made(3, 8, "alpha", e2, original, exact, trip)},
				joins{8: into(trip), 12: into(holidays)}, news{party, imported(beach, next+1)}, entries{edit})},
				[]domain.Album{album(next, event, "05/04 Party"), album(next+1, event, "05/05 Beach")},
				[]domain.AlbumPair{pair(trip, "alpha", personal, photosDir, byRemote), pair(next, "alpha", personal, partyDir, byRemote),
					pair(next+1, "alpha", shared, beachDir, byRemote)},
				[]domain.Notice{notice("plan.unknown_file", personal, "2024/notes.txt"), notice("plan.cloud_better", personal, holidaysDir+"/IMG_0012.jpg")}}},
		{"new files not settled yet wait outside the run under either root, and a directory and a file without MTime never wait",
			state(link(1, 7, "alpha", e1z, original, holidays), link(5, 7, "alpha", sBeach, original, 0), link(2, 7, "beta", y4, original, 0)),
			delta(a7), scans{"alpha": {yBeach, yDir, e1z, sBeach, y4}}, specs{beta, plain}, p,
			result{found{1: e1z, 5: sBeach}, reads{"alpha": scanned(nil, nil, nil, nil), "beta": scanned(nil, nil, nil, nil)},
				[]domain.Album{album(next, event, "05/05 Beach")}, []domain.AlbumPair{pair(next, "alpha", personal, beachDir, byRemote)},
				[]domain.Notice{settling(personal, partyDir+"/IMG_0004.HEIC"), settling(shared, beachDir+"/IMG_0005.HEIC")}}},
		{"a file of an asset not settled yet keeps the asset out of the run in the remote, and its settled file stays named",
			state(link(3, 7, "alpha", picnicMOV, liveVideo, 0), link(1, 7, "alpha", picnic, original, 0)), delta(a7),
			scans{"alpha": {yMOV, picnic}}, specs{alpha}, p,
			result{found{1: picnic}, reads{"alpha": scanned(nil, nil, nil, nil, 7)}, nil, nil,
				[]domain.Notice{settling(personal, picnicDir+"/IMG_0001.MOV")}}},
		{"a file replaced and not settled yet lends its old content to no move",
			state(link(1, 7, "alpha", e1, original, holidays)), delta(a7), scans{"alpha": {copied, y1}}, specs{alpha}, p,
			result{nil, reads{"alpha": scanned(nil, nil, news{imported(copied, next)}, nil, 7)},
				[]domain.Album{album(next, event, "05/08 Copies")}, []domain.AlbumPair{pair(next, "alpha", personal, copiesDir, byRemote)},
				[]domain.Notice{settling(personal, holidaysDir+"/IMG_0001.HEIC")}}},
		{"a file moved and not settled yet keeps its asset out of the run by its content, though the asset is gone from Photos",
			state(link(2, 9, "beta", twin, original, 0), link(1, 7, "alpha", e1, original, holidays)), delta(),
			scans{"alpha": {yMoved}}, specs{alpha}, p,
			result{nil, reads{"alpha": scanned(nil, nil, nil, nil, 7)}, nil, nil, []domain.Notice{settling(personal, movedDir+"/IMG_0001.HEIC")}}},
		{"only the files of assets that the run decides on make albums, each where it lies now",
			state(link(30, 30, "alpha", e30, original, 0), link(31, 31, "alpha", e31, original, 0), detached,
				link(33, 32, "alpha", e32, original, 0), link(34, 34, "alpha", e34, original, 0), link(1, 7, "alpha", e1, original, holidays)),
			delta(frozen, trashed, hidden, a7), scans{"alpha": {e34, e32, e32d, e31, e30, zoo}}, specs{alpha}, skip,
			result{found{1: zoo, 30: e30, 31: e31, 32: e32d, 33: e32, 34: e34}, reads{"alpha": scanned(nil, nil, nil, nil)},
				[]domain.Album{album(next, event, "05/10 Zoo")}, []domain.AlbumPair{pair(next, "alpha", personal, zooDir, byRemote)}, nil}},
		{"a Live video linked beside its photo does not count among the files of an album",
			state(link(4, 14, "alpha", q14, original, 0), link(3, 13, "alpha", q13, original, 0),
				link(2, 8, "alpha", e2v, liveVideo, 0), link(1, 8, "alpha", e2, original, 0)),
			delta(a14, a13, a8), scans{"alpha": {q14, e2v, q13, e2}}, specs{alpha}, p,
			result{found{1: e2, 2: e2v, 3: q13, 4: q14}, reads{"alpha": scanned(nil, nil, nil, nil)},
				[]domain.Album{album(next, event, "Trip photos")},
				[]domain.AlbumPair{pair(trip, "alpha", personal, picsDir, byRemote), pair(next, "alpha", personal, photosDir, byRemote)}, nil}},
		{"a Live video matched with its photo does not count either, and the asset joins the album of their directory once",
			state(link(2, 14, "alpha", q14, original, 0), link(1, 13, "alpha", q13, original, 0)), delta(live8, a14, a13),
			scans{"alpha": {e2v, q13, e2, q14}}, specs{alpha}, p,
			result{found{1: q13, 2: q14}, reads{"alpha": scanned(
				decided{made(3, 8, "alpha", e2, original, exact, next), made(4, 8, "alpha", e2v, liveVideo, component, next)},
				joins{8: into(next)}, nil, nil)},
				[]domain.Album{album(next, event, "Trip photos")},
				[]domain.AlbumPair{pair(trip, "alpha", personal, picsDir, byRemote), pair(next, "alpha", personal, photosDir, byRemote)}, nil}},
		{"ambiguous moves and unknown files are reported in turn, and a new file goes into the album of its directory",
			state(link(2, 41, "alpha", e41, original, holidays), link(1, 40, "alpha", e40, original, holidays)), delta(a41, a40),
			scans{"alpha": {notes, z}}, specs{alpha}, p,
			result{nil, reads{"alpha": scanned(nil, nil, news{imported(z, trip)}, nil)}, nil,
				[]domain.AlbumPair{pair(trip, "alpha", personal, tripDir, byRemote)},
				[]domain.Notice{notice("plan.ambiguous_move", personal, holidaysDir+"/IMG_0040.HEIC"),
					notice("plan.ambiguous_move", personal, holidaysDir+"/IMG_0041.HEIC"), notice("plan.unknown_file", personal, "2024/notes.txt")}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v %+v %+v", c.state, c.delta, c.scans, c.remotes, c.p)
			var got result
			got.files, got.read, got.added, got.made, got.notices = plan.ReadScans(c.state, c.delta, c.scans, c.remotes, c.p, now)
			if !same(got, c.want) {
				t.Errorf("ReadScans = %s; want %s", show(got), show(c.want))
			}
			for _, s := range c.want.read {
				var rules []string
				for _, d := range s.Linked {
					rules = append(rules, d.Rule)
				}
				for _, cs := range s.Join {
					for _, x := range cs {
						rules = append(rules, x.Rule)
					}
				}
				for _, n := range s.NewFiles {
					rules = append(rules, n.Decision.Rule)
				}
				for _, id := range rules {
					if _, ok := domain.Rules[id]; !ok {
						t.Errorf("%s is not in domain.Rules", id)
					}
				}
			}
			rs, rd, rr := c.state, c.delta, slices.Clone(c.remotes)
			rs.Links, rs.Albums, rs.AlbumPairs = slices.Clone(rs.Links), slices.Clone(rs.Albums), slices.Clone(rs.AlbumPairs)
			rd.Assets, rd.Albums = slices.Clone(rd.Assets), slices.Clone(rd.Albums)
			slices.Reverse(rs.Links)
			slices.Reverse(rs.Albums)
			slices.Reverse(rs.AlbumPairs)
			slices.Reverse(rd.Assets)
			slices.Reverse(rd.Albums)
			slices.Reverse(rr)
			rsc := scans{}
			for name, es := range c.scans {
				rsc[name] = slices.Clone(es)
				slices.Reverse(rsc[name])
			}
			var again result
			again.files, again.read, again.added, again.made, again.notices = plan.ReadScans(rs, rd, rsc, rr, c.p, now)
			if !same(again, got) {
				t.Errorf("with the input reversed: %s; want %s", show(again), show(got))
			}
			for _, s := range got.read {
				for i := range s.Linked {
					s.Linked[i].Link.Path = "changed"
				}
				for i := range s.NewFiles {
					s.NewFiles[i].File.Main.Path = "changed"
				}
				for i := range s.Edited {
					s.Edited[i].Path = "changed"
				}
			}
			for _, a := range got.added {
				for i := range a.FolderTitles {
					a.FolderTitles[i] = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v %+v", c.state, c.delta, c.scans, c.remotes, c.p); after != input {
				t.Errorf("ReadScans changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
