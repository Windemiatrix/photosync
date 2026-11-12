package plan_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Each row starts from files that lie where the albums of the asset want
// them and changes its albums, its library or a file. The expected changes
// are written from the rules on albums and directories, not from the code,
// and must not depend on the order of the input.
func TestDecidePlacement(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, old, pets, recipes, gone domain.AlbumID = 10, 11, 13, 20, 21, 99

		personal, shared     = domain.LibraryPersonal, domain.LibraryShared
		move, copyFile       = domain.ActionMoveFile, domain.ActionCopyFile
		upload, toQuarantine = domain.ActionUpload, domain.ActionQuarantineFile
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: old, UUID: "album-13", Title: "05/03 Old", ParentID: 1, FolderTitles: []string{"2024"}, InTrash: true},
		{ID: pets, UUID: "album-20", Title: "Pets"},
		{ID: recipes, UUID: "album-21", Title: "Recipes"},
	}
	asset := func(library, target domain.LibraryKind, ids ...domain.AlbumID) domain.Asset {
		a := domain.Asset{
			ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
			Library: library, TargetLibrary: target, OriginalName: "IMG_0001.HEIC",
			Taken: time.Date(2024, 5, 1, 12, 0, 0, 0, time.UTC),
		}
		for _, id := range ids {
			a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
		}
		return a
	}
	in := func(ids ...domain.AlbumID) domain.Asset { return asset(personal, personal, ids...) }
	joined := func(a domain.Asset, id domain.AlbumID, run string) domain.Asset {
		a.Memberships = append(slices.Clone(a.Memberships), domain.Membership{AlbumID: id, FirstSeenRun: run})
		return a
	}
	file := func(id domain.LinkID, remote string, album domain.AlbumID, path string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: personal, Path: path,
			Role: domain.RoleOriginal, AlbumID: album, Status: domain.StatusActive,
		}
	}
	with := func(l domain.Link, change func(*domain.Link)) domain.Link {
		change(&l)
		return l
	}
	links := func(l ...domain.Link) []domain.Link { return l }
	e1 := file(1, "disk", holidays, "2024/05-01 - Holidays/IMG_0001.HEIC")
	e2 := file(2, "disk", trip, "2024/05-02 - Trip/IMG_0001.HEIC")
	day := file(3, "disk", 0, "2024/05-01/IMG_0001.HEIC")
	pet := file(4, "disk", pets, "Albums/Pets/IMG_0001.HEIC")
	boxE1 := file(5, "box", holidays, "2024/05-01 - Holidays/IMG_0001.HEIC")
	boxPet := file(6, "box", pets, "Albums/Pets/IMG_0001.HEIC")
	oldE := file(8, "disk", old, "2024/05-03 - Old/IMG_0001.HEIC")
	recipe := file(16, "disk", recipes, "Albums/Recipes/IMG_0001.HEIC")
	sharedE1 := with(e1, func(l *domain.Link) { l.ID, l.Root = 9, shared })
	sharedE2 := with(e2, func(l *domain.Link) { l.Root = shared })
	live := with(e2, func(l *domain.Link) {
		l.ID, l.Role, l.Path = 11, domain.RoleLiveVideo, "2024/05-02 - Trip/IMG_0001.MOV"
	})
	outE2 := with(e2, func(l *domain.Link) {
		l.ID, l.Status = 12, domain.StatusQuarantinedRemote
		l.QuarantinePath = ".photosync/quarantine/2024-06-01/2024/05-02 - Trip/IMG_0001.HEIC"
	})
	outPet := with(pet, func(l *domain.Link) {
		l.ID, l.Status = 13, domain.StatusQuarantinedRemote
		l.QuarantinePath = ".photosync/quarantine/2024-06-01/Albums/Pets/IMG_0001.HEIC"
	})
	source := with(e1, func(l *domain.Link) {
		l.ID, l.Role, l.Path = 14, domain.RoleSource, "2024/05-01 - Holidays/MVI_0001.MPG"
	})
	e1b := with(e1, func(l *domain.Link) { l.ID, l.Path = 15, "2024/05-01 - Holidays/IMG_0001 (1).HEIC" })

	type row struct {
		action domain.LinkAction
		link   domain.LinkID
		album  domain.AlbumID
		rule   string
	}
	for _, c := range []struct {
		name  string
		asset domain.Asset
		links []domain.Link
		want  []row
	}{
		{"every file where it belongs", in(holidays, trip), links(e1, e2), nil}, // covers: R-LAYOUT-08
		{"added to an event: a copy", in(holidays, trip), links(e1), // covers: R-OBJECT-03
			[]row{{copyFile, 1, trip, "R-OBJECT-03"}}},
		{"removed from one of two events: its file to quarantine", in(holidays), links(e1, e2), // covers: R-OBJECT-04
			[]row{{toQuarantine, 2, 0, "R-OBJECT-04"}}},
		{"moved from one event to another", in(trip), links(e1), // covers: R-OBJECT-05
			[]row{{move, 1, trip, "R-OBJECT-05"}}},
		{"from the day folder to an event: a move, not a copy", in(holidays), links(day),
			[]row{{move, 3, holidays, "R-OBJECT-05"}}},
		{"from a thematic album to an event: a move, not a copy", in(pets, holidays), links(pet),
			[]row{{move, 4, holidays, "R-OBJECT-05"}}},
		{"from the day folder to two events: a move and a copy", in(holidays, trip), links(day),
			[]row{{move, 3, holidays, "R-OBJECT-05"}, {copyFile, 3, trip, "R-OBJECT-03"}}},
		{"removed from every event: the thematic album", in(pets), links(e1), // covers: R-OBJECT-06
			[]row{{move, 1, pets, "R-OBJECT-06"}}},
		{"removed from every album: the day folder", in(), links(e1),
			[]row{{move, 1, 0, "R-OBJECT-06"}}},
		{"removed from two events at once: one move, one quarantine", in(), links(e2, e1),
			[]row{{move, 1, 0, "R-OBJECT-06"}, {toQuarantine, 2, 0, "R-OBJECT-04"}}},
		{"of two thematic albums the one joined first", joined(in(recipes), pets, "run-2"), links(day),
			[]row{{move, 3, recipes, "R-OBJECT-06"}}},
		{"a thematic copy beside an event file stays", in(holidays, pets), links(e1, pet), nil},
		{"a thematic copy beside the file of the primary thematic album stays", joined(in(pets), recipes, "run-2"), links(pet, recipe), nil},
		{"removed from the thematic album of a copy: the copy to quarantine", in(holidays), links(e1, pet),
			[]row{{toQuarantine, 4, 0, "R-OBJECT-04"}}},
		{"removed from every event with a thematic copy: the event file to quarantine", in(pets), links(e1, pet),
			[]row{{toQuarantine, 1, 0, "R-OBJECT-04"}}},
		{"a thematic album without a file stays in iCloud only", in(holidays, pets), links(e1), nil},
		{"the thematic copy of another remote: a copy here too", in(holidays, pets), links(e1, boxE1, boxPet),
			[]row{{copyFile, 1, pets, "R-OBJECT-03"}}},
		{"new to this remote: an upload for each event", in(holidays, trip), links(boxE1),
			[]row{{upload, 0, holidays, "R-LINK-01"}, {upload, 0, trip, "R-LINK-01"}}},
		{"new to this remote, the primary placement copied in another: one upload", in(pets), links(boxPet),
			[]row{{upload, 0, pets, "R-LINK-01"}}},
		{"no link and no album: the day folder, the first placement being done before", in(), nil,
			[]row{{upload, 0, 0, "R-LINK-01"}}},
		{"shared in Photos: every file to the shared root", asset(shared, shared, holidays, trip), links(e1, e2), // covers: R-OBJECT-07
			[]row{{move, 1, holidays, "R-OBJECT-07"}, {move, 2, trip, "R-OBJECT-07"}}},
		{"unshared in Photos", in(holidays), links(sharedE1),
			[]row{{move, 9, holidays, "R-OBJECT-07"}}},
		{"waits in the personal library for the shared one of its file", asset(personal, shared, holidays), links(sharedE1), nil},
		{"reaches the shared library of its file", asset(shared, shared, holidays), links(sharedE1), nil},
		{"a file put in the other root takes the others along", asset(personal, shared, holidays, trip), links(e1, sharedE2),
			[]row{{move, 1, holidays, "R-OBJECT-07"}}},
		{"shared in Photos and removed from an event", asset(shared, shared, holidays), links(e1, e2),
			[]row{{move, 1, holidays, "R-OBJECT-07"}, {toQuarantine, 2, 0, "R-OBJECT-04"}}},
		{"shared in Photos and removed from every album", asset(shared, shared), links(e1),
			[]row{{move, 1, 0, "R-OBJECT-07"}}},
		{"one path in both roots: the file in the root of the asset moves", in(), links(sharedE1, e1),
			[]row{{move, 1, 0, "R-OBJECT-06"}, {toQuarantine, 9, 0, "R-OBJECT-04"}}},
		{"an album in Recently Deleted or gone from the library holds no file", in(holidays, old, gone), links(e1, oldE),
			[]row{{toQuarantine, 8, 0, "R-OBJECT-04"}}},
		{"a component away from its file and a link out of sync stay as they are", in(holidays), links(e1, live, outE2), nil},
		{"a thematic album with only a link out of sync", in(holidays, pets), links(e1, outPet), nil},
		{"a source file stands for the asset", in(trip), links(source),
			[]row{{move, 14, trip, "R-OBJECT-05"}}},
		{"two files of the asset in one album both stay", in(holidays), links(e1, e1b), nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			before := slices.Clone(c.links)
			got := plan.DecidePlacement(c.asset, c.links, "disk", albums)
			var rows []row
			for _, p := range got {
				rows = append(rows, row{p.Action, p.Link.ID, p.AlbumID, p.Rule})
				if p.Link != (domain.Link{}) && !slices.Contains(c.links, p.Link) {
					t.Errorf("%+v names a link that is not in the input", p)
				}
			}
			if !slices.Equal(rows, c.want) {
				t.Errorf("DecidePlacement = %+v; want %+v", rows, c.want)
			}
			if !slices.Equal(c.links, before) {
				t.Errorf("DecidePlacement changed its links to %+v", c.links)
			}
			for _, w := range c.want {
				if _, ok := domain.Rules[w.rule]; !ok {
					t.Errorf("%s is not in domain.Rules", w.rule)
				}
			}
			reversed, reordered, all := c.asset, slices.Clone(c.links), slices.Clone(albums)
			reversed.Memberships = slices.Clone(c.asset.Memberships)
			slices.Reverse(reversed.Memberships)
			slices.Reverse(reordered)
			slices.Reverse(all)
			if again := plan.DecidePlacement(reversed, reordered, "disk", all); !slices.Equal(again, got) {
				t.Errorf("with the input reversed: %+v; want %+v", again, got)
			}
		})
	}
}
