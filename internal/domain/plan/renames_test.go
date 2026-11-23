package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row gives the albums and the pairs of a run and the whole listing of
// one remote; a pair remembers the path of the album that named its
// directory unless the row says otherwise. The expected pairs of its personal
// root follow the rule on albums renamed in iCloud, not the code, and must
// not depend on the order of the input nor on which of the two roots is the
// one asked for; a second run over the pairs that the first moved moves none.
func TestMovePairs(t *testing.T) {
	t.Parallel()
	const (
		year, hobby, models, later              domain.AlbumID = 1, 2, 3, 4
		holidays, trip, printing, pets, figures domain.AlbumID = 10, 11, 12, 13, 14
		gone                                    domain.AlbumID = 15

		personal, shared                 = domain.LibraryPersonal, domain.LibraryShared
		folder, event, thematic          = domain.KindFolder, domain.KindEvent, domain.KindThematic
		twoWay, uploadOnly, downloadOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly

		holidaysDir, tripDir, vacationDir = "2024/05-01 - Holidays", "2024/05-02 - Trip", "2024/05-01 - Vacation"
		petsDir                           = "Albums/Hobby/Pets"
	)
	album := func(id domain.AlbumID, kind domain.AlbumKind, parent domain.AlbumID, title string, folders ...string) domain.Album {
		return domain.Album{ID: id, Kind: kind, Title: title, ParentID: parent, FolderTitles: folders}
	}
	lib := []domain.Album{
		album(year, folder, 0, "2024"), album(hobby, folder, 0, "Hobby"), album(models, folder, hobby, "Models", "Hobby"),
		album(holidays, event, year, "05/01 Holidays", "2024"), album(trip, event, year, "05/02 Trip", "2024"),
		album(printing, thematic, hobby, "3D printing", "Hobby"), album(pets, thematic, hobby, "Pets", "Hobby"),
		album(figures, thematic, models, "Figures", "Hobby", "Models"), album(gone, event, year, "05/09 Gone", "2024"),
	}
	run := func(changes ...domain.Album) []domain.Album {
		albums := slices.DeleteFunc(slices.Clone(lib), func(a domain.Album) bool { return a.ID == gone })
		for _, c := range changes {
			if i := slices.IndexFunc(albums, func(a domain.Album) bool { return a.ID == c.ID }); i >= 0 {
				albums[i] = c
			} else {
				albums = append(albums, c)
			}
		}
		return albums
	}
	named := map[domain.AlbumID]string{
		year: `[] "2024"`, holidays: `["2024"] "05/01 Holidays"`, trip: `["2024"] "05/02 Trip"`, gone: `["2024"] "05/09 Gone"`,
		printing: `["Hobby"] "3D printing"`, pets: `["Hobby"] "Pets"`, figures: `["Hobby" "Models"] "Figures"`,
	}
	pair := func(id domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud, NamedAfter: named[id]}
	}
	mine := func(id domain.AlbumID, dir string) domain.AlbumPair { return pair(id, "disk", personal, dir) }
	unnamed := func(x domain.AlbumPair) domain.AlbumPair {
		x.NamedAfter = ""
		return x
	}
	prs := func(x ...domain.AlbumPair) []domain.AlbumPair { return x }
	file := func(root domain.LibraryKind, at string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 100}
	}
	files := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	renamed := album(holidays, event, year, "05/01 Vacation", "2024")
	listed := files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, tripDir+"/IMG_0002.HEIC"))
	trashed := album(trip, event, year, "05/03 Trip", "2023")
	trashed.InTrash = true
	cloudy := album(holidays, event, year, "05/01 Holidays", "2024")
	cloudy.Key, cloudy.KeySource = "guid-10", domain.KeyCloudGUID
	p := domain.DefaultPolicies()
	p.UntitledEvent, p.AlbumsRoot = "Untitled", "Albums"
	for _, c := range []struct {
		name    string
		albums  []domain.Album
		pairs   []domain.AlbumPair
		listing []domain.RemoteEntry
		mode    domain.Mode
		want    []domain.AlbumPair
	}{
		{"nothing in, nothing out", nil, nil, nil, twoWay, nil},
		{"a pair stays while its album keeps its title and its folders",
			run(), prs(mine(holidays, holidaysDir), mine(trip, tripDir)), listed, twoWay, nil},
		// covers: R-LAYOUT-11
		{"an album renamed in iCloud takes its pair to the directory of its new title, and the pair of an album that kept its title stays",
			run(renamed), prs(mine(trip, tripDir), mine(holidays, holidaysDir)), listed, twoWay, prs(mine(holidays, vacationDir))},
		{"an album moved to a new year folder in iCloud takes its pair there",
			run(album(later, folder, 0, "2025"), album(holidays, event, later, "05/01 Holidays", "2025")), prs(mine(holidays, holidaysDir)), listed, twoWay,
			prs(mine(holidays, "2025/05-01 - Holidays"))},
		{"a year folder renamed in iCloud takes along the pairs of the events in it, in the order of their directories",
			run(album(year, folder, 0, "2023"), album(holidays, event, year, "05/01 Holidays", "2023"), album(trip, event, year, "05/02 Trip", "2023")),
			prs(mine(trip, tripDir), mine(holidays, holidaysDir)), listed, twoWay,
			prs(mine(holidays, "2023/05-01 - Holidays"), mine(trip, "2023/05-02 - Trip"))},
		{"a directory that the user named otherwise keeps its name when its album is renamed",
			run(renamed), prs(unnamed(mine(holidays, "2024/Holiday trip"))), files(file(personal, "2024/Holiday trip/IMG_0001.HEIC")), twoWay, nil},
		{"a pair stays when the listing holds a file below its new directory, even below the directory of another pair",
			run(renamed), prs(mine(holidays, holidaysDir), mine(trip, vacationDir+"/raw")),
			files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, vacationDir+"/raw/IMG_0009.HEIC")), twoWay, nil},
		{"a pair stays when another pair of the remote and root holds its new directory",
			run(album(holidays, event, year, "05/02 Trip", "2024")), prs(mine(holidays, holidaysDir), mine(trip, tripDir)),
			files(file(personal, holidaysDir+"/IMG_0001.HEIC")), twoWay, nil},
		{"of two albums renamed to one title the one whose directory comes first takes the new directory",
			run(album(holidays, event, year, "05/03 Party", "2024"), album(trip, event, year, "05/03 Party", "2024")),
			prs(mine(trip, tripDir), mine(holidays, holidaysDir)), nil, twoWay, prs(mine(holidays, "2024/05-03 - Party"))},
		{"an album renamed in case alone keeps its directory, which a client blind to case takes for the same",
			run(album(holidays, event, year, "05/01 holidays", "2024")), prs(mine(holidays, holidaysDir)), files(file(personal, tripDir+"/IMG_0002.HEIC")), twoWay, nil},
		{"the pairs and the listing of another remote or root hold no directory here, and only the pairs of the remote and root move",
			run(renamed), prs(pair(trip, "other", personal, vacationDir), pair(trip, "disk", shared, vacationDir), mine(holidays, holidaysDir)),
			files(file(shared, vacationDir+"/IMG_0001.HEIC"), file(personal, holidaysDir+"/IMG_0001.HEIC")), twoWay, prs(mine(holidays, vacationDir))},
		{"download-only moves no pair", run(renamed), prs(mine(holidays, holidaysDir)), listed, downloadOnly, nil},
		{"upload-only moves pairs as two-way does", run(renamed), prs(mine(holidays, holidaysDir)), listed, uploadOnly, prs(mine(holidays, vacationDir))},
		{"the pairs of a folder, of an album in Recently Deleted and of an album gone from the library stay",
			run(album(year, folder, 0, "2023"), trashed), prs(mine(year, "2024"), mine(trip, tripDir), mine(gone, "2024/05-09 - Gone")), listed, twoWay, nil},
		{"a pair stays while its album keeps its path under a cloud key, though the policies now give the album another directory",
			run(cloudy), prs(mine(holidays, "Things/2024/05-01 - Holidays")), nil, twoWay, nil},
		{"the pair of an album without a directory stays",
			run(album(pets, thematic, hobby, "Dogs", "Hobby")), prs(mine(pets, petsDir)), nil, twoWay, nil},
		{"a pair stays when its new directory differs in case alone from a directory of the listing",
			run(renamed), prs(mine(holidays, holidaysDir)), files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, "2024/05-01 - vacation/IMG_0009.HEIC")),
			twoWay, nil},
		{"a pair that remembers no path stays when its album is renamed, even in the directory of the old path",
			run(renamed), prs(unnamed(mine(holidays, holidaysDir))), listed, twoWay, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode}
			input := fmt.Sprintf("%+v %+v %+v", c.albums, c.pairs, c.listing)
			got := plan.MovePairs(c.albums, c.pairs, c.listing, r, personal, p)
			if !slices.Equal(got, c.want) {
				t.Errorf("MovePairs = %+v; want %+v", got, c.want)
			}
			albums, pairs, listing := slices.Clone(c.albums), slices.Clone(c.pairs), slices.Clone(c.listing)
			slices.Reverse(albums)
			slices.Reverse(pairs)
			slices.Reverse(listing)
			if back := plan.MovePairs(albums, pairs, listing, r, personal, p); !slices.Equal(back, got) {
				t.Errorf("with the input reversed: %+v; want %+v", back, got)
			}
			swap := func(k domain.LibraryKind) domain.LibraryKind {
				if k == personal {
					return shared
				}
				return personal
			}
			swapped, mirror := slices.Clone(c.pairs), slices.Clone(c.listing)
			for i := range swapped {
				swapped[i].Root = swap(swapped[i].Root)
			}
			for i := range mirror {
				mirror[i].Root = swap(mirror[i].Root)
			}
			mirrored := plan.MovePairs(c.albums, swapped, mirror, r, shared, p)
			for i := range mirrored {
				mirrored[i].Root = swap(mirrored[i].Root)
			}
			if !slices.Equal(mirrored, got) {
				t.Errorf("with the roots swapped, and back: %+v; want %+v", mirrored, got)
			}
			next := slices.Clone(c.pairs)
			for _, x := range got {
				next[slices.IndexFunc(next, func(y domain.AlbumPair) bool {
					return y.AlbumID == x.AlbumID && y.Remote == x.Remote && y.Root == x.Root
				})] = x
			}
			if again := plan.MovePairs(c.albums, next, c.listing, r, personal, p); len(again) > 0 {
				t.Errorf("a second run over the pairs that the first moved moves %+v; want none", again)
			}
			for i := range got {
				got[i].DirPath = "changed"
			}
			if after := fmt.Sprintf("%+v %+v %+v", c.albums, c.pairs, c.listing); after != input {
				t.Errorf("MovePairs changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}

// Each row gives the links of an asset where their files lie now, the pairs
// of the state that a run moves and the placements that the run already
// makes in one remote. The expected placements follow the rule on albums
// renamed in iCloud, not the code, in the order of the links.
func TestFollowPairs(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip domain.AlbumID = 10, 11

		personal, shared            = domain.LibraryPersonal, domain.LibraryShared
		original, source, liveVideo = domain.RoleOriginal, domain.RoleSource, domain.RoleLiveVideo
		holidaysDir, tripDir        = "2024/05-01 - Holidays", "2024/05-02 - Trip"
	)
	link := func(id domain.LinkID, remote string, root domain.LibraryKind, at string, role domain.LinkRole, album domain.AlbumID) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: at, Role: role, AlbumID: album,
			Size: 100, Level: domain.LevelExact, Status: domain.StatusActive,
		}
	}
	follow := func(l domain.Link) domain.Placement {
		return domain.Placement{Action: domain.ActionMoveFile, Link: l, AlbumID: l.AlbumID, Rule: "R-LAYOUT-11"}
	}
	photo, mpeg := link(1, "disk", personal, holidaysDir+"/IMG_0001.HEIC", original, holidays), link(2, "disk", personal, holidaysDir+"/IMG_0002.MPG", source, holidays)
	video, parked := link(3, "disk", personal, holidaysDir+"/IMG_0001.MOV", liveVideo, holidays), link(4, "disk", personal, holidaysDir+"/IMG_0004.HEIC", original, holidays)
	parked.Status = domain.StatusQuarantinedRemote
	stranger, below := link(5, "disk", personal, holidaysDir+"/IMG_0005.HEIC", original, trip), link(6, "disk", personal, holidaysDir+"/raw/IMG_0006.HEIC", original, holidays)
	inShared, elsewhere := link(7, "disk", shared, holidaysDir+"/IMG_0007.HEIC", original, holidays), link(8, "other", personal, holidaysDir+"/IMG_0008.HEIC", original, holidays)
	leaving, copied := link(9, "disk", personal, holidaysDir+"/IMG_0009.HEIC", original, holidays), link(10, "disk", personal, holidaysDir+"/IMG_0010.HEIC", original, holidays)
	inTrip := link(11, "disk", personal, tripDir+"/IMG_0011.HEIC", original, trip)
	all := []domain.Link{photo, mpeg, video, parked, stranger, below, inShared, elsewhere, leaving, copied, inTrip}
	moved := []domain.AlbumPair{
		{AlbumID: holidays, Remote: "disk", Root: personal, DirPath: holidaysDir, CreatedBy: domain.OriginICloud},
		{AlbumID: trip, Remote: "other", Root: personal, DirPath: tripDir, CreatedBy: domain.OriginICloud},
	}
	placements := []domain.Placement{
		{Action: domain.ActionMoveFile, Link: leaving, AlbumID: trip, Rule: "R-OBJECT-05"},
		{Action: domain.ActionCopyFile, Link: copied, AlbumID: trip, Rule: "R-OBJECT-03"},
	}
	for _, c := range []struct {
		name       string
		links      []domain.Link
		moved      []domain.AlbumPair
		placements []domain.Placement
		want       []domain.Placement
	}{
		{"nothing in, nothing out", nil, nil, nil, nil},
		{"no file follows while no pair moves", all, nil, placements, nil},
		// covers: R-LAYOUT-11
		{"the files of the original and the source role of the remote that lie directly in the old directory of a moved pair of their album and root follow it, the source of a copy too, while a Live Photo video, a link in quarantine and the files of another album, below, in another root or remote, moved by a placement or under a pair that only another remote moves stay",
			all, moved, placements, []domain.Placement{follow(photo), follow(mpeg), follow(copied)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v %+v", c.links, c.moved, c.placements)
			got := plan.FollowPairs(c.links, c.moved, "disk", c.placements)
			if !slices.Equal(got, c.want) {
				t.Errorf("FollowPairs = %+v; want %+v", got, c.want)
			}
			links := slices.Clone(c.links)
			slices.Reverse(links)
			back := plan.FollowPairs(links, c.moved, "disk", c.placements)
			slices.Reverse(back)
			if !slices.Equal(back, got) {
				t.Errorf("with the links reversed, the result reversed back: %+v; want %+v", back, got)
			}
			if after := fmt.Sprintf("%+v %+v %+v", c.links, c.moved, c.placements); after != input {
				t.Errorf("FollowPairs changed its input:\n%s\nwant\n%s", after, input)
			}
		})
	}
}

// Each row gives the albums and the pairs of a run, the links of the state,
// the files of one remote where its scan finds them and its listing; a pair
// remembers the path of its album unless the row says otherwise. The
// expected pairs, albums and folders follow the rules on directories renamed
// and moved in the cloud, not the code, and must not depend on the order of
// the input; a second run, with the links where their files lie now, changes
// none.
func TestRenamedDirs(t *testing.T) {
	t.Parallel()
	const (
		cats, holidays, trip, beach domain.AlbumID = 5, 10, 11, 12
		gone, models, last          domain.AlbumID = 15, 16, 8
		hobby, kits, lego           domain.AlbumID = 8, 17, 18
		alpha, bravo                domain.AlbumID = 20, 21

		personal, shared                 = domain.LibraryPersonal, domain.LibraryShared
		event, thematic                  = domain.KindEvent, domain.KindThematic
		twoWay, uploadOnly, downloadOnly = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
		original, liveVideo              = domain.RoleOriginal, domain.RoleLiveVideo

		holidaysDir, tripDir, vacationDir = "2024/05-01 - Holidays", "2024/05-02 - Trip", "2024/05-01 - Vacation"
		beachDir, catsDir, goneDir        = "2024/05-01 - Holidays/Beach", "Albums/Cats", "Albums/Gone"
		trip2Dir, movedDir                = "2024/05-02 - Trip 2", "2024/05-09 - Moved"
		laterDir, modelsDir               = "2025/05-01 - Holidays", "Albums/Hobby/Models"
		alphaDir, bravoDir                = "2024/05-01 - A", "2024/05-02 - B"
	)
	album := func(id domain.AlbumID, kind domain.AlbumKind, title string, folders ...string) domain.Album {
		return domain.Album{ID: id, Kind: kind, Title: title, FolderTitles: folders}
	}
	folder := func(id, parent domain.AlbumID, title string, folders ...string) domain.Album {
		return domain.Album{ID: id, Kind: domain.KindFolder, Title: title, ParentID: parent, FolderTitles: folders}
	}
	catsAlbum, holidaysAlbum := album(cats, thematic, "Cats"), album(holidays, event, "05/01 Holidays", "2024")
	tripAlbum, beachAlbum := album(trip, event, "05/02 Trip", "2024"), album(beach, event, "05/01 Holidays / Beach", "2024")
	lib := []domain.Album{catsAlbum, holidaysAlbum, tripAlbum, beachAlbum}
	titled := func(a domain.Album, title string) domain.Album {
		a.Title = title
		return a
	}
	filed := func(a domain.Album, kind domain.AlbumKind, parent domain.AlbumID, folders ...string) domain.Album {
		a.Kind, a.ParentID, a.FolderTitles = kind, parent, folders
		return a
	}
	run := func(change domain.Album) []domain.Album {
		albums := slices.Clone(lib)
		albums[slices.IndexFunc(albums, func(a domain.Album) bool { return a.ID == change.ID })] = change
		return albums
	}
	named := map[domain.AlbumID]string{
		cats: `[] "Cats"`, holidays: `["2024"] "05/01 Holidays"`, trip: `["2024"] "05/02 Trip"`,
		beach: `["2024"] "05/01 Holidays / Beach"`, gone: `[] "Gone"`, models: `["Hobby"] "Models"`,
		kits: `["Hobby" "Lego"] "Kits"`, alpha: `["2024" "Events"] "05/01 A"`, bravo: `["2024" "Events"] "05/02 B"`,
	}
	pair := func(id domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud, NamedAfter: named[id]}
	}
	mine := func(id domain.AlbumID, dir string) domain.AlbumPair { return pair(id, "disk", personal, dir) }
	unnamed := func(x domain.AlbumPair) domain.AlbumPair {
		x.NamedAfter = ""
		return x
	}
	prs := func(x ...domain.AlbumPair) []domain.AlbumPair { return x }
	als := func(a ...domain.Album) []domain.Album { return a }
	link := func(id domain.LinkID, root domain.LibraryKind, at string, role domain.LinkRole) domain.Link {
		return domain.Link{ID: id, Remote: "disk", Root: root, Path: at, Role: role, Status: domain.StatusActive}
	}
	lks := func(l ...domain.Link) []domain.Link { return l }
	file := func(root domain.LibraryKind, at string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 100}
	}
	listed := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	type found = map[domain.LinkID]domain.RemoteEntry
	l1, l2, l3 := link(1, personal, holidaysDir+"/IMG_0001.HEIC", original), link(2, personal, holidaysDir+"/IMG_0002.HEIC", original), link(3, personal, tripDir+"/IMG_0003.HEIC", original)
	v1, v2, t3 := file(personal, vacationDir+"/IMG_0001.HEIC"), file(personal, vacationDir+"/IMG_0002.HEIC"), file(personal, tripDir+"/IMG_0003.HEIC")
	world, away, after := lks(l1, l2, l3), found{1: v1, 2: v2, 3: t3}, listed(v1, v2, t3)
	both := prs(mine(holidays, holidaysDir), mine(trip, tripDir))
	l4, v4, l5 := link(4, personal, beachDir+"/IMG_0004.HEIC", original), file(personal, vacationDir+"/Beach/IMG_0004.HEIC"), link(5, personal, catsDir+"/IMG_0005.HEIC", original)
	s1, sv1, t3moved := link(1, shared, holidaysDir+"/IMG_0001.HEIC", original), file(shared, vacationDir+"/IMG_0001.HEIC"), file(personal, trip2Dir+"/IMG_0003.HEIC")
	trashed := tripAlbum
	trashed.InTrash = true
	underDay, underTrip := file(personal, "2024/05-03/05-01 - Holidays/IMG_0001.HEIC"), file(personal, tripDir+"/05-01 - Holidays/Beach/IMG_0004.HEIC")
	lower, bare := file(personal, "2024/05-01 - holidays/IMG_0001.HEIC"), file(personal, "2024/05-01/IMG_0001.HEIC")
	l1later, b4later := file(personal, laterDir+"/IMG_0001.HEIC"), file(personal, laterDir+"/Beach/IMG_0004.HEIC")
	deep, catsLater := file(personal, "Albums/Pets/Home/Cats/IMG_0005.HEIC"), file(personal, "2025/Cats/IMG_0005.HEIC")
	picsLater, elsewhere := file(personal, "2025/05-02 - Trip pics/IMG_0003.HEIC"), file(personal, "2025/05-01 - Vacation/IMG_0001.HEIC")
	modelsLater, l16 := file(personal, "Albums/Models/IMG_0016.HEIC"), link(16, personal, modelsDir+"/IMG_0016.HEIC", original)
	binnedYear, modelsAlbum := folder(3, 0, "2025"), album(models, thematic, "Models", "Hobby")
	binnedYear.InTrash, modelsAlbum.ParentID = true, 8
	year24, kitsAlbum := folder(1, 0, "2024"), album(kits, thematic, "Kits", "Hobby", "Lego")
	kitsAlbum.ParentID = lego
	dated := als(year24, catsAlbum, filed(holidaysAlbum, event, 1, "2024"), filed(tripAlbum, event, 1, "2024"), filed(beachAlbum, event, 1, "2024"))
	crafts := append(slices.Clone(lib), folder(hobby, 0, "Hobby"), modelsAlbum, folder(lego, hobby, "Lego", "Hobby"), kitsAlbum)
	h23, t23, b23 := file(personal, "2023/05-01 - Holidays/IMG_0001.HEIC"), file(personal, "2023/05-02 - Trip/IMG_0003.HEIC"), file(personal, "2023/05-01 - Holidays/Beach/IMG_0004.HEIC")
	l17, m2, k2 := link(17, personal, "Albums/Hobby/Lego/Kits/IMG_0017.HEIC", original), file(personal, "Albums/Crafts/Models/IMG_0016.HEIC"), file(personal, "Albums/Crafts/Lego/Kits/IMG_0017.HEIC")
	sub, toys := file(personal, "Albums/Crafts/Sub/Models/IMG_0016.HEIC"), file(personal, "Albums/Hobby/Toys/Kits/IMG_0017.HEIC")
	p := domain.DefaultPolicies()
	p.UntitledEvent, p.AlbumsRoot = "Untitled", "Albums"
	local, events := p, p
	local.AlbumsForPersonal = false
	events.Templates.Photos = []string{"{YYYY}", "Events", "{MM}/{DD} {Description}"}
	eventsFolder := folder(2, 1, "Events", "2024")
	outingA, outingB := album(alpha, event, "05/01 A", "2024", "Events"), album(bravo, event, "05/02 B", "2024", "Events")
	outingA.ParentID, outingB.ParentID = 2, 2
	la, lb := link(20, personal, alphaDir+"/IMG_0020.HEIC", original), link(21, personal, bravoDir+"/IMG_0021.HEIC", original)
	fa, fb := file(personal, "2025/05-01 - A/IMG_0020.HEIC"), file(personal, bravoDir+"/IMG_0021.HEIC")
	for _, c := range []struct {
		name    string
		albums  []domain.Album
		pairs   []domain.AlbumPair
		links   []domain.Link
		files   found
		listing []domain.RemoteEntry
		mode    domain.Mode
		root    domain.LibraryKind
		p       domain.Policies
		moved   []domain.AlbumPair
		renamed []domain.Album
		refiled []domain.Album
		added   []domain.Album
	}{
		{"nothing in, nothing out", nil, nil, nil, nil, nil, twoWay, personal, p, nil, nil, nil, nil},
		// covers: R-LAYOUT-12
		{"a directory that the user renamed in the cloud takes its pair along and gives its album the title that the new name reads as, while a directory still listed keeps its pair",
			lib, both, world, away, after, twoWay, personal, p, prs(unnamed(mine(holidays, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"a nested part does not follow the new directory of its parent, which the template reads alone, whatever the order of the input",
			lib, prs(mine(beach, beachDir), mine(holidays, holidaysDir)), lks(l4, l1), found{1: v1, 4: v4}, listed(v4, v1), twoWay, personal, p,
			prs(unnamed(mine(holidays, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"a directory that the user named otherwise follows its files under the new name, and its album keeps its title",
			lib, prs(unnamed(mine(holidays, "2024/Holiday trip"))), lks(link(1, personal, "2024/Holiday trip/IMG_0001.HEIC", original)), found{1: v1}, listed(v1),
			twoWay, personal, p, prs(unnamed(mine(holidays, vacationDir))), nil, nil, nil},
		{"an album that the user renamed in iCloud as well keeps the title of iCloud, and only its pair follows the directory",
			run(titled(holidaysAlbum, "05/01 Beach")), both, world, away, after, twoWay, personal, p, prs(unnamed(mine(holidays, vacationDir))), nil, nil, nil},
		{"upload-only takes the pair along and leaves the album as it is",
			lib, both, world, away, after, uploadOnly, personal, p, prs(unnamed(mine(holidays, vacationDir))), nil, nil, nil},
		{"download-only renames the album as two-way does",
			lib, both, world, away, after, downloadOnly, personal, p, prs(unnamed(mine(holidays, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"while albums_for_personal is false a pair of the personal root follows its directory, and the album keeps its title",
			lib, both, world, away, after, twoWay, personal, local, prs(unnamed(mine(holidays, vacationDir))), nil, nil, nil},
		{"while albums_for_personal is false a directory renamed in the shared root renames the album",
			lib, prs(pair(holidays, "disk", shared, holidaysDir)), lks(s1), found{1: sv1}, listed(sv1), twoWay, shared, local,
			prs(unnamed(pair(holidays, "disk", shared, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"a pair stays while the listing holds a path below its directory",
			lib, both, world, away, listed(v1, v2, t3, file(personal, holidaysDir+"/raw/IMG_0009.HEIC")), twoWay, personal, p, nil, nil, nil, nil},
		{"a pair stays when the files of its directory went to two directories, and another when its file went nowhere",
			lib, both, world, found{1: v1, 2: file(personal, movedDir+"/IMG_0002.HEIC")}, listed(v1, file(personal, movedDir+"/IMG_0002.HEIC")),
			twoWay, personal, p, nil, nil, nil, nil},
		{"a pair stays when the files of its directory went to the directory of another pair",
			lib, both, world, found{1: file(personal, tripDir+"/IMG_0001.HEIC"), 2: file(personal, tripDir+"/IMG_0002.HEIC"), 3: t3},
			listed(file(personal, tripDir+"/IMG_0001.HEIC"), file(personal, tripDir+"/IMG_0002.HEIC"), t3), twoWay, personal, p, nil, nil, nil, nil},
		{"of two directories whose files went to one new directory the one first by its old path takes it",
			lib, both, world, found{1: v1, 2: v2, 3: file(personal, vacationDir+"/IMG_0003.HEIC")}, listed(v1, v2, file(personal, vacationDir+"/IMG_0003.HEIC")),
			twoWay, personal, p, prs(unnamed(mine(holidays, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"a pair stays when the files of its directory went to the directory of an event without a description",
			lib, prs(mine(cats, catsDir)), lks(l5), found{5: file(personal, "2024/05-03/IMG_0005.HEIC")}, listed(file(personal, "2024/05-03/IMG_0005.HEIC")),
			twoWay, personal, p, nil, nil, nil, nil},
		{"a pair stays when the files of its directory went straight into a year folder",
			lib, prs(mine(cats, "Cats")), lks(link(5, personal, "Cats/IMG_0005.HEIC", original)), found{5: file(personal, "2025/IMG_0005.HEIC")}, listed(file(personal, "2025/IMG_0005.HEIC")),
			twoWay, personal, p, nil, nil, nil, nil},
		// covers: R-LAYOUT-13
		{"a directory that the user moved to another year takes its pair along and moves its album into the folder of that year, which the run adds",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: l1later}, listed(l1later), twoWay, personal, p,
			prs(unnamed(mine(holidays, laterDir))), nil, als(filed(holidaysAlbum, event, 13, "2025")), als(folder(13, 0, "2025"))},
		{"the pairs of an album in Recently Deleted and of an album gone from the library stay",
			run(trashed), prs(mine(trip, tripDir), mine(gone, goneDir)), lks(l3, link(6, personal, goneDir+"/IMG_0006.HEIC", original)),
			found{3: t3moved, 6: file(personal, "Albums/Gone 2/IMG_0006.HEIC")}, listed(t3moved, file(personal, "Albums/Gone 2/IMG_0006.HEIC")),
			twoWay, personal, p, nil, nil, nil, nil},
		{"the pairs and the listing of another remote or root count for nothing here",
			lib, prs(pair(holidays, "other", personal, holidaysDir), pair(holidays, "disk", shared, holidaysDir), mine(trip, tripDir)), lks(l1, l3),
			found{1: v1, 3: t3moved}, listed(v1, t3moved, file(shared, tripDir+"/IMG_0009.HEIC")), twoWay, personal, p,
			prs(unnamed(mine(trip, trip2Dir))), als(titled(tripAlbum, "05/02 Trip 2")), nil, nil},
		{"a file that went to the other root, and the file of a link of the other root, count for nothing",
			lib, prs(mine(trip, tripDir)), lks(l3, link(6, personal, tripDir+"/IMG_0006.HEIC", original), link(7, shared, tripDir+"/IMG_0007.HEIC", original)),
			found{3: t3moved, 6: file(shared, movedDir+"/IMG_0006.HEIC"), 7: file(personal, movedDir+"/IMG_0007.HEIC")},
			listed(t3moved, file(shared, movedDir+"/IMG_0006.HEIC"), file(personal, movedDir+"/IMG_0007.HEIC")), twoWay, personal, p,
			prs(unnamed(mine(trip, trip2Dir))), als(titled(tripAlbum, "05/02 Trip 2")), nil, nil},
		{"a Live Photo video that went elsewhere does not hold the pair back",
			lib, prs(mine(holidays, holidaysDir)), lks(l1, link(8, personal, holidaysDir+"/IMG_0001.MOV", liveVideo)),
			found{1: v1, 8: file(personal, movedDir+"/IMG_0001.MOV")}, listed(v1, file(personal, movedDir+"/IMG_0001.MOV")), twoWay, personal, p,
			prs(unnamed(mine(holidays, vacationDir))), als(titled(holidaysAlbum, "05/01 Vacation")), nil, nil},
		{"a directory moved deeper than the template, under the directory of another event: the pair stays and the album keeps its title",
			lib, prs(mine(holidays, holidaysDir), mine(beach, beachDir), mine(trip, tripDir)), lks(l1, l4, l3), found{1: underDay, 4: underTrip, 3: t3},
			listed(underDay, underTrip, t3), twoWay, personal, p, nil, nil, nil, nil},
		{"a nested part moved out of its parent into the year folder: the pair stays and the album keeps its title",
			lib, prs(mine(holidays, holidaysDir), mine(beach, beachDir)), lks(l1, l4),
			found{1: file(personal, holidaysDir+"/IMG_0001.HEIC"), 4: file(personal, "2024/Beach/IMG_0004.HEIC")},
			listed(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, "2024/Beach/IMG_0004.HEIC")), twoWay, personal, p, nil, nil, nil, nil},
		{"a directory renamed only in letter case takes its pair along and gives its album the title in the new case",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: lower}, listed(lower), twoWay, personal, p,
			prs(unnamed(mine(holidays, "2024/05-01 - holidays"))), als(titled(holidaysAlbum, "05/01 holidays")), nil, nil},
		{"a directory renamed to the bare day gives its album the title without a description",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: bare}, listed(bare), twoWay, personal, p,
			prs(unnamed(mine(holidays, "2024/05-01"))), als(titled(holidaysAlbum, "05/01")), nil, nil},
		{"a directory moved into a folder that iCloud has moves its album into that folder outside Recently Deleted, the lowest ID first",
			slices.Concat(lib, als(folder(7, 0, "2025"), folder(6, 0, "2025"), folder(4, 0, "2025", "Old"), binnedYear, album(2, thematic, "2025"), folder(1, 0, "2023"))),
			prs(mine(holidays, holidaysDir)), lks(l1), found{1: l1later}, listed(l1later), twoWay, personal, p,
			prs(unnamed(mine(holidays, laterDir))), nil, als(filed(holidaysAlbum, event, 6, "2025")), nil},
		{"a directory moved under folders outside the template leads no pair, and its album keeps its folders",
			append(slices.Clone(lib), folder(6, 0, "Home")), prs(mine(cats, catsDir)), lks(l5), found{5: deep}, listed(deep), twoWay, personal, p,
			nil, nil, nil, nil},
		{"a nested part does not follow its parent into another year, while the parent goes into the folder that the run adds, whatever the order of the input",
			lib, prs(mine(beach, beachDir), mine(holidays, holidaysDir)), lks(l4, l1), found{1: l1later, 4: b4later}, listed(b4later, l1later), twoWay, personal, p,
			prs(unnamed(mine(holidays, laterDir))), nil, als(filed(holidaysAlbum, event, 13, "2025")), als(folder(13, 0, "2025"))},
		{"an album that the user moved in iCloud as well keeps the folder of iCloud, and so does the album of a directory named otherwise: only their pairs follow",
			run(album(holidays, event, "05/01 Holidays", "2023")), prs(mine(holidays, holidaysDir), unnamed(mine(trip, "2024/05-02 - Trip pics"))),
			lks(l1, link(3, personal, "2024/05-02 - Trip pics/IMG_0003.HEIC", original)), found{1: l1later, 3: picsLater}, listed(l1later, picsLater), twoWay, personal, p,
			prs(unnamed(mine(holidays, laterDir)), unnamed(mine(trip, "2025/05-02 - Trip pics"))), nil, nil, nil},
		{"a directory moved to another year under another title: the pair stays and the album keeps its folder and title",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: elsewhere}, listed(elsewhere), twoWay, personal, p, nil, nil, nil, nil},
		{"a directory outside the template leads no pair, into a year folder or out of its folders",
			append(slices.Clone(lib), folder(2, 0, "2025"), folder(8, 0, "Hobby"), modelsAlbum), prs(mine(models, modelsDir), mine(cats, catsDir)),
			lks(l16, l5), found{5: catsLater, 16: modelsLater}, listed(modelsLater, catsLater), twoWay, personal, p,
			nil, nil, nil, nil},
		{"a directory moved out of the albums root leads no pair",
			lib, prs(mine(cats, catsDir)), lks(l5), found{5: file(personal, "Cats/IMG_0005.HEIC")}, listed(file(personal, "Cats/IMG_0005.HEIC")), twoWay, personal, p,
			nil, nil, nil, nil},
		{"upload-only takes the pair of a directory moved to another year along and leaves the album where it is",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: l1later}, listed(l1later), uploadOnly, personal, p,
			prs(unnamed(mine(holidays, laterDir))), nil, nil, nil},
		{"while albums for personal is false a pair of the personal root follows a directory moved to another year, and the album stays where it is",
			lib, prs(mine(holidays, holidaysDir)), lks(l1), found{1: l1later}, listed(l1later), twoWay, personal, local,
			prs(unnamed(mine(holidays, laterDir))), nil, nil, nil},
		// covers: R-LAYOUT-14
		{"a year folder that the user renamed in the cloud takes the new title in iCloud while iCloud lacks a folder of that title, and the pairs of the events in it follow with no move of an album, while a nested part stays",
			dated, prs(mine(holidays, holidaysDir), mine(trip, tripDir), mine(beach, beachDir)), lks(l1, l3, l4), found{1: h23, 3: t23, 4: b23}, listed(h23, t23, b23, file(personal, "2024-old/IMG_0009.HEIC")), twoWay, personal, p,
			prs(unnamed(mine(holidays, "2023/05-01 - Holidays")), unnamed(mine(trip, "2023/05-02 - Trip"))),
			als(folder(1, 0, "2023")), nil, nil},
		{"a folder of the albums root that the user renamed leads no pair: the template reads no directory there",
			crafts, prs(mine(models, modelsDir), mine(kits, "Albums/Hobby/Lego/Kits")), lks(l16, l17), found{16: m2, 17: k2}, listed(m2, k2), twoWay, personal, p,
			nil, nil, nil, nil},
		{"a folder whose directory the listing still holds, if only in the other root, keeps its title, and the album moved out of it goes into a folder that the run adds",
			dated, prs(mine(holidays, holidaysDir)), lks(l1), found{1: h23}, listed(h23, file(shared, tripDir+"/IMG_0009.HEIC")), twoWay, personal, p,
			prs(unnamed(mine(holidays, "2023/05-01 - Holidays"))), nil, als(filed(holidaysAlbum, event, 13, "2023")), als(folder(13, 0, "2023"))},
		{"a folder renamed in the cloud to the title of a folder that iCloud has keeps its title, and its album moves into that folder",
			append(slices.Clone(dated), folder(9, 0, "2023")), prs(mine(holidays, holidaysDir)), lks(l1), found{1: h23}, listed(h23), twoWay, personal, p,
			prs(unnamed(mine(holidays, "2023/05-01 - Holidays"))), nil, als(filed(holidaysAlbum, event, 9, "2023")), nil},
		{"a directory moved deeper than the template leads no pair, and its album keeps its folders",
			crafts, prs(mine(models, modelsDir)), lks(l16), found{16: sub}, listed(sub), twoWay, personal, p,
			nil, nil, nil, nil},
		{"a folder inside a folder of the albums root that the user renamed leads no pair",
			crafts, prs(mine(kits, "Albums/Hobby/Lego/Kits")), lks(l17), found{17: toys}, listed(toys), twoWay, personal, p,
			nil, nil, nil, nil},
		{"a folder without a directory is not taken for gone",
			als(year24, eventsFolder, outingA, outingB), prs(mine(alpha, alphaDir), mine(bravo, bravoDir)), lks(la, lb),
			found{20: fa, 21: fb}, listed(fa, fb), twoWay, personal, events,
			prs(unnamed(mine(alpha, "2025/05-01 - A"))), nil, als(filed(outingA, event, 23, "2025", "Events")),
			als(folder(22, 0, "2025"), folder(23, 22, "Events", "2025"))},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode}
			input := fmt.Sprintf("%+v %+v %+v %+v %+v", c.albums, c.pairs, c.links, c.files, c.listing)
			moved, renamed, refiled, added := plan.RenamedDirs(c.albums, c.pairs, last, c.links, c.files, c.listing, r, c.root, c.p)
			if !slices.Equal(moved, c.moved) || fmt.Sprintf("%+v %+v %+v", renamed, refiled, added) != fmt.Sprintf("%+v %+v %+v", c.renamed, c.refiled, c.added) {
				t.Errorf("RenamedDirs = %+v, %+v, %+v, %+v; want %+v, %+v, %+v, %+v", moved, renamed, refiled, added, c.moved, c.renamed, c.refiled, c.added)
			}
			albums, pairs, links, listing := slices.Clone(c.albums), slices.Clone(c.pairs), slices.Clone(c.links), slices.Clone(c.listing)
			slices.Reverse(albums)
			slices.Reverse(pairs)
			slices.Reverse(links)
			slices.Reverse(listing)
			if back, again, refiledBack, addedBack := plan.RenamedDirs(albums, pairs, last, links, c.files, listing, r, c.root, c.p); !slices.Equal(back, moved) ||
				fmt.Sprintf("%+v %+v %+v", again, refiledBack, addedBack) != fmt.Sprintf("%+v %+v %+v", renamed, refiled, added) {
				t.Errorf("with the input reversed: %+v, %+v, %+v, %+v; want %+v, %+v, %+v, %+v", back, again, refiledBack, addedBack, moved, renamed, refiled, added)
			}
			for _, x := range moved {
				pairs[slices.IndexFunc(pairs, func(y domain.AlbumPair) bool {
					return y.AlbumID == x.AlbumID && y.Remote == x.Remote && y.Root == x.Root
				})] = x
			}
			for _, a := range slices.Concat(renamed, refiled) {
				albums[slices.IndexFunc(albums, func(b domain.Album) bool { return b.ID == a.ID })] = a
			}
			albums = append(albums, added...)
			for i, l := range links {
				if e, ok := c.files[l.ID]; ok {
					links[i].Root, links[i].Path = e.Root, e.Path
				}
			}
			if more, again, refiledAgain, addedAgain := plan.RenamedDirs(albums, pairs, last, links, c.files, listing, r, c.root, c.p); len(more)+len(again)+len(refiledAgain)+len(addedAgain) > 0 {
				t.Errorf("a second run, with the links where their files lie now, gives %+v, %+v, %+v, %+v; want none", more, again, refiledAgain, addedAgain)
			}
			for i := range moved {
				moved[i].DirPath = "changed"
			}
			for i := range renamed {
				renamed[i].Title = "changed"
			}
			for _, a := range slices.Concat(refiled, added) {
				for i := range a.FolderTitles {
					a.FolderTitles[i] = "changed"
				}
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v %+v", c.albums, c.pairs, c.links, c.files, c.listing); after != input {
				t.Errorf("RenamedDirs changed its input or shares a slice with it:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
