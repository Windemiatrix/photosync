package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row gives the albums of the state and of a run, the pairs of the run
// and the whole listing of one remote. The expected pairs of its personal
// root follow the rule on albums renamed in iCloud, not the code, and must
// not depend on the order of the input nor on which of the two roots is the
// one asked for; a second run, whose state took the albums and the pairs of
// the first, moves no pair.
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
		printingDir, petsDir, figuresDir  = "Albums/Hobby/3D printing", "Albums/Hobby/Pets", "Albums/Hobby/Models/Figures"
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
	pair := func(id domain.AlbumID, remote string, root domain.LibraryKind, dir string) domain.AlbumPair {
		return domain.AlbumPair{AlbumID: id, Remote: remote, Root: root, DirPath: dir, CreatedBy: domain.OriginICloud}
	}
	mine := func(id domain.AlbumID, dir string) domain.AlbumPair { return pair(id, "disk", personal, dir) }
	prs := func(x ...domain.AlbumPair) []domain.AlbumPair { return x }
	file := func(root domain.LibraryKind, at string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 100}
	}
	files := func(e ...domain.RemoteEntry) []domain.RemoteEntry { return e }
	renamed := album(holidays, event, year, "05/01 Vacation", "2024")
	listed := files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, tripDir+"/IMG_0002.HEIC"))
	trashed := album(trip, event, year, "05/03 Trip", "2023")
	trashed.InTrash = true
	cycle := slices.Clone(lib)
	cycle[0].ParentID, cycle[1].ParentID = hobby, year
	p := domain.DefaultPolicies()
	p.UntitledEvent, p.AlbumsRoot = "Untitled", "Albums"
	for _, c := range []struct {
		name    string
		before  []domain.Album
		albums  []domain.Album
		pairs   []domain.AlbumPair
		listing []domain.RemoteEntry
		mode    domain.Mode
		want    []domain.AlbumPair
	}{
		{"nothing in, nothing out", nil, nil, nil, nil, twoWay, nil},
		{"a pair stays while its album keeps its title and its folders",
			lib, run(), prs(mine(holidays, holidaysDir), mine(trip, tripDir)), listed, twoWay, nil},
		// covers: R-LAYOUT-11
		{"an album renamed in iCloud takes its pair to the directory of its new title, and the pair of an album that kept its title stays",
			lib, run(renamed), prs(mine(trip, tripDir), mine(holidays, holidaysDir)), listed, twoWay, prs(mine(holidays, vacationDir))},
		{"an album moved to a new year folder in iCloud takes its pair there",
			lib, run(album(later, folder, 0, "2025"), album(holidays, event, later, "05/01 Holidays", "2025")), prs(mine(holidays, holidaysDir)), listed, twoWay,
			prs(mine(holidays, "2025/05-01 - Holidays"))},
		{"a folder renamed in iCloud takes along the pairs of the albums in it and in its folders, in the order of their directories",
			lib, run(album(hobby, folder, 0, "Crafts"), album(models, folder, hobby, "Models", "Crafts"), album(printing, thematic, hobby, "3D printing", "Crafts"),
				album(pets, thematic, hobby, "Pets", "Crafts"), album(figures, thematic, models, "Figures", "Crafts", "Models")),
			prs(mine(pets, petsDir), mine(figures, figuresDir), mine(printing, printingDir)), files(file(personal, petsDir+"/IMG_0003.HEIC")), twoWay,
			prs(mine(printing, "Albums/Crafts/3D printing"), mine(figures, "Albums/Crafts/Models/Figures"), mine(pets, "Albums/Crafts/Pets"))},
		{"a directory that the user named otherwise keeps its name when its album is renamed",
			lib, run(renamed), prs(mine(holidays, "2024/Holiday trip")), files(file(personal, "2024/Holiday trip/IMG_0001.HEIC")), twoWay, nil},
		{"a pair stays when the listing holds a file below its new directory, even below the directory of another pair",
			lib, run(renamed), prs(mine(holidays, holidaysDir), mine(trip, vacationDir+"/raw")),
			files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, vacationDir+"/raw/IMG_0009.HEIC")), twoWay, nil},
		{"a pair stays when another pair of the remote and root holds its new directory",
			lib, run(album(holidays, event, year, "05/02 Trip", "2024")), prs(mine(holidays, holidaysDir), mine(trip, tripDir)),
			files(file(personal, holidaysDir+"/IMG_0001.HEIC")), twoWay, nil},
		{"of two albums renamed to one title the one whose directory comes first takes the new directory",
			lib, run(album(printing, thematic, hobby, "Toys", "Hobby"), album(pets, thematic, hobby, "Toys", "Hobby")),
			prs(mine(pets, petsDir), mine(printing, printingDir)), nil, twoWay, prs(mine(printing, "Albums/Hobby/Toys"))},
		{"an album renamed in case alone keeps its directory, which a client blind to case takes for the same",
			lib, run(album(holidays, event, year, "05/01 holidays", "2024")), prs(mine(holidays, holidaysDir)), files(file(personal, tripDir+"/IMG_0002.HEIC")), twoWay, nil},
		{"the pairs and the listing of another remote or root hold no directory here, and only the pairs of the remote and root move",
			lib, run(renamed), prs(pair(trip, "other", personal, vacationDir), pair(trip, "disk", shared, vacationDir), mine(holidays, holidaysDir)),
			files(file(shared, vacationDir+"/IMG_0001.HEIC"), file(personal, holidaysDir+"/IMG_0001.HEIC")), twoWay, prs(mine(holidays, vacationDir))},
		{"download-only moves no pair", lib, run(renamed), prs(mine(holidays, holidaysDir)), listed, downloadOnly, nil},
		{"upload-only moves pairs as two-way does", lib, run(renamed), prs(mine(holidays, holidaysDir)), listed, uploadOnly, prs(mine(holidays, vacationDir))},
		{"the pairs of a folder, of an album in Recently Deleted and of an album gone from the library stay",
			lib, run(album(year, folder, 0, "2023"), trashed), prs(mine(year, "2024"), mine(trip, tripDir), mine(gone, "2024/05-09 - Gone")), listed, twoWay, nil},
		{"a cycle of folders in a state that went wrong ends the walk, and the pair stays",
			cycle, run(renamed), prs(mine(holidays, holidaysDir)), listed, twoWay, nil},
		{"a pair stays when its new directory differs in case alone from a directory of the listing",
			lib, run(renamed), prs(mine(holidays, holidaysDir)), files(file(personal, holidaysDir+"/IMG_0001.HEIC"), file(personal, "2024/05-01 - vacation/IMG_0009.HEIC")),
			twoWay, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := domain.RemoteSpec{Name: "disk", Mode: c.mode}
			input := fmt.Sprintf("%+v %+v %+v %+v", c.before, c.albums, c.pairs, c.listing)
			got := plan.MovePairs(c.before, c.albums, c.pairs, c.listing, r, personal, p)
			if !slices.Equal(got, c.want) {
				t.Errorf("MovePairs = %+v; want %+v", got, c.want)
			}
			before, albums, pairs, listing := slices.Clone(c.before), slices.Clone(c.albums), slices.Clone(c.pairs), slices.Clone(c.listing)
			slices.Reverse(before)
			slices.Reverse(albums)
			slices.Reverse(pairs)
			slices.Reverse(listing)
			if back := plan.MovePairs(before, albums, pairs, listing, r, personal, p); !slices.Equal(back, got) {
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
			mirrored := plan.MovePairs(c.before, c.albums, swapped, mirror, r, shared, p)
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
			if again := plan.MovePairs(c.albums, c.albums, next, c.listing, r, personal, p); len(again) > 0 {
				t.Errorf("a second run, whose state took the albums and the pairs of the first, moves %+v; want none", again)
			}
			for i := range got {
				got[i].DirPath = "changed"
			}
			if after := fmt.Sprintf("%+v %+v %+v %+v", c.before, c.albums, c.pairs, c.listing); after != input {
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
