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

// Each row changes a file of the asset in the cloud, and in some rows the
// asset in iCloud, since the last applied run. The decisions come from
// DecideLink as a run makes them; the expected decisions and albums are
// written from the rules on links and albums, not from the code, and must
// not depend on the order of the input.
func TestDecideObject(t *testing.T) {
	t.Parallel()
	const (
		holidays, trip, party domain.AlbumID = 10, 11, 12

		personal, shared          = domain.LibraryPersonal, domain.LibraryShared
		unchanged, moved, deleted = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted
		none, forget, deleteAsset = domain.ActionNone, domain.ActionForget, domain.ActionDeleteAsset
		moveAsset, conflict       = domain.ActionMoveAsset, domain.ActionConflict
		updateBase, upload        = domain.ActionUpdateBase, domain.ActionUpload
		toTrip, toParty           = "2024/05-02 - Trip/IMG_0001.HEIC", "2024/05-03 - Party/IMG_0001.HEIC"
		besideE2, renamed         = "2024/05-02 - Trip/IMG_0001 (1).HEIC", "2024/05-01 - Holidays/Beach.HEIC"
	)
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
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
	file := func(id domain.LinkID, album domain.AlbumID, path string) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: "disk", Root: personal, Path: path,
			Role: domain.RoleOriginal, AlbumID: album, Status: domain.StatusActive,
		}
	}
	with := func(l domain.Link, change func(*domain.Link)) domain.Link {
		change(&l)
		return l
	}
	at := func(l domain.Link, root domain.LibraryKind, album domain.AlbumID, path string) domain.Link {
		l.Root, l.AlbumID, l.Path = root, album, path
		return l
	}
	decided := func(l domain.Link, icloud, remote domain.ChangeKind) domain.LinkDecision {
		d, _ := plan.DecideLink(icloud, remote, false)
		d.Link = l
		return d
	}
	decisions := func(d ...domain.LinkDecision) []domain.LinkDecision { return d }
	links := func(l ...domain.Link) []domain.Link { return l }
	e1 := file(1, holidays, "2024/05-01 - Holidays/IMG_0001.HEIC")
	e2 := file(2, trip, toTrip)
	day := file(3, 0, "2024/05-01/IMG_0001.HEIC")
	e1b := file(15, holidays, "2024/05-01 - Holidays/IMG_0001 (1).HEIC")
	live := with(e2, func(l *domain.Link) { l.ID, l.Role, l.Path = 4, domain.RoleLiveVideo, "2024/05-02 - Trip/IMG_0001.MOV" })
	box := with(e1, func(l *domain.Link) { l.ID, l.Remote = 5, "box" })
	boxE2 := with(e2, func(l *domain.Link) { l.ID, l.Remote = 6, "box" })
	sharedE1 := with(e1, func(l *domain.Link) { l.ID, l.Root = 9, shared })
	sharedE2 := with(e2, func(l *domain.Link) { l.ID, l.Root = 10, shared })
	mpg := with(e1, func(l *domain.Link) {
		l.ID, l.Role, l.Path = 14, domain.RoleSource, "2024/05-01 - Holidays/MVI_0001.MPG"
	})
	mpg2 := with(e2, func(l *domain.Link) { l.ID, l.Role, l.Path = 16, domain.RoleSource, "2024/05-02 - Trip/MVI_0001.MPG" })

	type row struct {
		link   domain.LinkID
		action domain.LinkAction
		rule   string
	}
	type change struct {
		album domain.AlbumID
		rule  string
	}
	project := func(cs []domain.AlbumChange) []change {
		var out []change
		for _, x := range cs {
			out = append(out, change{x.AlbumID, x.Rule})
		}
		return out
	}
	for _, c := range []struct {
		name        string
		asset       domain.Asset
		decisions   []domain.LinkDecision
		now         []domain.Link
		want        []row
		join, leave []change
	}{
		{"nothing changed", in(holidays, trip),
			decisions(decided(e1, unchanged, unchanged), decided(e2, unchanged, unchanged)), links(e1, e2),
			[]row{{1, none, ""}, {2, none, ""}}, nil, nil},
		{"one of two files gone: forgotten, and the asset leaves its album", in(holidays, trip), // covers: R-OBJECT-02
			decisions(decided(e2, unchanged, deleted), decided(e1, unchanged, unchanged)), links(e1),
			[]row{{1, none, ""}, {2, forget, "R-OBJECT-02"}}, nil, []change{{trip, "R-OBJECT-02"}}},
		{"one of two files gone while another remote keeps that album: the asset leaves it", in(holidays, trip),
			decisions(decided(e2, unchanged, deleted), decided(e1, unchanged, unchanged)), links(e1, boxE2),
			[]row{{1, none, ""}, {2, forget, "R-OBJECT-02"}}, nil, []change{{trip, "R-OBJECT-02"}}},
		{"a gone file whose album keeps a file in the other root: the asset stays there", in(holidays, trip),
			decisions(decided(e2, unchanged, deleted), decided(e1, unchanged, unchanged), decided(sharedE2, unchanged, unchanged)), links(e1, sharedE2),
			[]row{{1, none, ""}, {2, forget, "R-OBJECT-02"}, {10, none, ""}}, nil, nil},
		{"a gone file whose Live video stays: the asset leaves its album", in(holidays, trip),
			decisions(decided(e2, unchanged, deleted), decided(e1, unchanged, unchanged), decided(live, unchanged, unchanged)), links(e1, live),
			[]row{{1, none, ""}, {2, forget, "R-OBJECT-02"}, {4, none, ""}}, nil, []change{{trip, "R-OBJECT-02"}}},
		{"every file gone: the deletion stands", in(holidays, trip), // covers: R-OBJECT-01
			decisions(decided(e1, unchanged, deleted), decided(e2, unchanged, deleted)), nil,
			[]row{{1, deleteAsset, "R-LINK-09"}, {2, deleteAsset, "R-LINK-09"}}, nil, nil},
		{"every file gone here while another remote keeps one: the deletion stands", in(holidays),
			decisions(decided(e1, unchanged, deleted)), links(box),
			[]row{{1, deleteAsset, "R-LINK-09"}}, nil, nil},
		{"a gone file of the day folder: no album to leave", in(holidays),
			decisions(decided(e1, unchanged, unchanged), decided(day, unchanged, deleted)), links(e1),
			[]row{{1, none, ""}, {3, forget, "R-OBJECT-02"}}, nil, nil},
		{"a gone file beside another file in its album: the asset stays there", in(holidays),
			decisions(decided(e1, unchanged, unchanged), decided(e1b, unchanged, deleted)), links(e1),
			[]row{{1, none, ""}, {15, forget, "R-OBJECT-02"}}, nil, nil},
		{"a gone file of an album the asset left in iCloud: forgotten, not uploaded again", in(holidays),
			decisions(decided(e1, unchanged, unchanged), decided(e2, moved, deleted)), links(e1),
			[]row{{1, none, ""}, {2, forget, "R-OBJECT-02"}}, nil, []change{{trip, "R-OBJECT-02"}}},
		{"every file gone and the asset changed in iCloud: the decisions stand", in(holidays),
			decisions(decided(e1, unchanged, deleted), decided(e2, moved, deleted)), nil,
			[]row{{1, deleteAsset, "R-LINK-09"}, {2, upload, "R-LINK-10"}}, nil, nil},
		{"of two source files of a converted asset one gone", in(holidays, trip),
			decisions(decided(mpg, unchanged, unchanged), decided(mpg2, unchanged, deleted)), links(mpg),
			[]row{{14, none, ""}, {16, forget, "R-OBJECT-02"}}, nil, []change{{trip, "R-OBJECT-02"}}},
		{"a Live video gone beside its main file is left to it", in(holidays, trip),
			decisions(decided(e1, unchanged, unchanged), decided(e2, unchanged, unchanged), decided(live, unchanged, deleted)), links(e1, e2),
			[]row{{1, none, ""}, {2, none, ""}, {4, deleteAsset, "R-LINK-09"}}, nil, nil},
		{"a file moved to another event: the asset leaves one album and joins the other", in(holidays), // covers: R-LINK-05
			decisions(decided(e1, unchanged, moved)), links(at(e1, personal, trip, toTrip)),
			[]row{{1, moveAsset, "R-LINK-05"}}, []change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"a file renamed in its directory: no album changes", in(holidays),
			decisions(decided(e1, unchanged, moved)), links(at(e1, personal, holidays, renamed)),
			[]row{{1, moveAsset, "R-LINK-05"}}, nil, nil},
		{"a file moved to the day folder: the asset leaves its album and joins none", in(holidays),
			decisions(decided(e1, unchanged, moved)), links(at(e1, personal, 0, day.Path)),
			[]row{{1, moveAsset, "R-LINK-05"}}, nil, []change{{holidays, "R-LINK-05"}}},
		{"a file moved from the day folder into an event: the asset joins it", in(),
			decisions(decided(day, unchanged, moved)), links(at(day, personal, holidays, e1.Path)),
			[]row{{3, moveAsset, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}, nil},
		{"a file moved beside the other file of the asset: it leaves the album of its old directory", in(holidays, trip),
			decisions(decided(e1, unchanged, moved), decided(e2, unchanged, unchanged)), links(at(e1, personal, trip, besideE2), e2),
			[]row{{1, moveAsset, "R-LINK-05"}, {2, none, ""}}, []change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"a file moved to the other root: no album changes", in(holidays),
			decisions(decided(e1, unchanged, moved)), links(at(e1, shared, holidays, e1.Path)),
			[]row{{1, moveAsset, "R-LINK-05"}}, nil, nil},
		{"two files moved from one album to two others: each album once, in order", in(holidays),
			decisions(decided(e1, unchanged, moved), decided(e1b, unchanged, moved)),
			links(at(e1, personal, party, toParty), at(e1b, personal, trip, besideE2)),
			[]row{{1, moveAsset, "R-LINK-05"}, {15, moveAsset, "R-LINK-05"}},
			[]change{{trip, "R-LINK-05"}, {party, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}}},
		{"a file gone and another moved away from one album: both rules", in(holidays, trip),
			decisions(decided(e1, unchanged, deleted), decided(e1b, unchanged, moved), decided(e2, unchanged, unchanged)),
			links(at(e1b, personal, trip, besideE2), e2),
			[]row{{1, forget, "R-OBJECT-02"}, {2, none, ""}, {15, moveAsset, "R-LINK-05"}},
			[]change{{trip, "R-LINK-05"}}, []change{{holidays, "R-LINK-05"}, {holidays, "R-OBJECT-02"}}},
		{"both moved to one event: the same change", in(trip), // covers: R-LINK-13
			decisions(decided(e1, moved, moved)), links(at(e1, personal, trip, toTrip)),
			[]row{{1, updateBase, "R-LINK-13"}}, nil, nil},
		{"both moved to different events: a conflict with the albums of the cloud", in(trip), // covers: R-LINK-07
			decisions(decided(e1, moved, moved)), links(at(e1, personal, party, toParty)),
			[]row{{1, conflict, "R-LINK-07"}}, []change{{party, "R-LINK-07"}}, []change{{holidays, "R-LINK-07"}}},
		{"shared in Photos and renamed in the personal root: a conflict", asset(shared, shared, holidays),
			decisions(decided(e1, moved, moved)), links(at(e1, personal, holidays, renamed)),
			[]row{{1, conflict, "R-LINK-07"}}, nil, nil},
		{"shared in Photos and moved to the shared root: the same change", asset(shared, shared, holidays),
			decisions(decided(e1, moved, moved)), links(at(e1, shared, holidays, e1.Path)),
			[]row{{1, updateBase, "R-LINK-13"}}, nil, nil},
		{"waits for the shared library: both moved to one event in its root", asset(personal, shared, trip),
			decisions(decided(sharedE1, moved, moved)), links(at(sharedE1, shared, trip, toTrip)),
			[]row{{9, updateBase, "R-LINK-13"}}, nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			before, nowBefore := slices.Clone(c.decisions), slices.Clone(c.now)
			got := plan.DecideObject(c.asset, c.decisions, c.now, "disk", albums)
			var rows []row
			for _, d := range got.Links {
				rows = append(rows, row{d.Link.ID, d.Action, d.Rule})
				i := slices.IndexFunc(c.decisions, func(x domain.LinkDecision) bool { return x.Link == d.Link })
				if i < 0 || c.decisions[i].ICloud != d.ICloud || c.decisions[i].Remote != d.Remote {
					t.Errorf("%+v is not a decision of the input", d)
				}
			}
			if !slices.Equal(rows, c.want) {
				t.Errorf("DecideObject = %+v; want %+v", rows, c.want)
			}
			if join, leave := project(got.Join), project(got.Leave); !slices.Equal(join, c.join) || !slices.Equal(leave, c.leave) {
				t.Errorf("Join = %+v, Leave = %+v; want %+v, %+v", join, leave, c.join, c.leave)
			}
			if !slices.Equal(c.decisions, before) || !slices.Equal(c.now, nowBefore) {
				t.Errorf("DecideObject changed its input to %+v and %+v", c.decisions, c.now)
			}
			var rules []string
			for _, r := range c.want {
				rules = append(rules, r.rule)
			}
			for _, x := range slices.Concat(c.join, c.leave) {
				rules = append(rules, x.rule)
			}
			for _, id := range rules {
				if _, ok := domain.Rules[id]; id != "" && !ok {
					t.Errorf("%s is not in domain.Rules", id)
				}
			}
			for _, d := range got.Links {
				if d.ICloud != moved || d.Remote != moved {
					continue
				}
				keeps := !slices.ContainsFunc(plan.DecidePlacement(c.asset, c.now, "disk", albums), func(p domain.Placement) bool {
					return p.Link.ID == d.Link.ID && p.Action != domain.ActionCopyFile
				})
				if keeps != (d.Action == updateBase) {
					t.Errorf("link %d is %s while DecidePlacement keeps its file: %v; a conflict is the same change exactly when DecidePlacement keeps its file", d.Link.ID, d.Action, keeps)
				}
			}
			reversed, ds, now, all := c.asset, slices.Clone(c.decisions), slices.Clone(c.now), slices.Clone(albums)
			reversed.Memberships = slices.Clone(c.asset.Memberships)
			slices.Reverse(reversed.Memberships)
			slices.Reverse(ds)
			slices.Reverse(now)
			slices.Reverse(all)
			again := plan.DecideObject(reversed, ds, now, "disk", all)
			if !slices.Equal(again.Links, got.Links) || !slices.Equal(again.Join, got.Join) || !slices.Equal(again.Leave, got.Leave) {
				t.Errorf("with the input reversed: %+v; want %+v", again, got)
			}
		})
	}
}

// The first placement changes the albums of an asset in iCloud once, before
// any remote lays out its files. The expected changes are written from the
// rules on events, not from the code, and must not depend on the order of
// the input.
func TestDecideFirstPlacement(t *testing.T) {
	t.Parallel()
	const holidays, trip, party domain.AlbumID = 10, 11, 12
	albums := []domain.Album{
		{ID: 1, UUID: "album-1", Kind: domain.KindFolder, Title: "2024"},
		{ID: holidays, UUID: "album-10", Title: "05/01 Holidays", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: trip, UUID: "album-11", Title: "05/02 Trip", ParentID: 1, FolderTitles: []string{"2024"}},
		{ID: party, UUID: "album-12", Title: "05/03 Party", ParentID: 1, FolderTitles: []string{"2024"}},
	}
	remove, keep := domain.MultiAlbumRemoveOthers, domain.MultiAlbumKeepOthers
	type change struct {
		album domain.AlbumID
		rule  string
	}
	project := func(cs []domain.AlbumChange) []change {
		var out []change
		for _, x := range cs {
			out = append(out, change{x.AlbumID, x.Rule})
		}
		return out
	}
	for _, c := range []struct {
		name        string
		day         int
		in          []domain.AlbumID
		multi       domain.MultiAlbumPolicy
		join, leave []change
	}{
		{"in no album: the only event of its day", 1, nil, remove, []change{{holidays, "R-LAYOUT-05"}}, nil}, // covers: R-LAYOUT-05
		{"in no album, with no event of its day: the day folder", 4, nil, remove, nil, nil},
		{"in two events: it leaves the farther one", 1, []domain.AlbumID{holidays, trip}, remove, nil, []change{{trip, "R-LAYOUT-04"}}}, // covers: R-LAYOUT-04
		{"in two events, keeping the others", 1, []domain.AlbumID{holidays, trip}, keep, nil, nil},
		{"in three events: it leaves the others, closest first", 3, []domain.AlbumID{holidays, trip, party}, remove, nil,
			[]change{{trip, "R-LAYOUT-04"}, {holidays, "R-LAYOUT-04"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a := domain.Asset{
				ID: 7, Key: domain.AssetKey{CloudAssetGUID: "guid-7", UUID: "uuid-7"},
				Library: domain.LibraryPersonal, TargetLibrary: domain.LibraryPersonal, OriginalName: "IMG_0001.HEIC",
				Taken: time.Date(2024, 5, c.day, 12, 0, 0, 0, time.UTC),
			}
			for _, id := range c.in {
				a.Memberships = append(a.Memberships, domain.Membership{AlbumID: id, FirstSeenRun: "run-1"})
			}
			join, leave := plan.DecideFirstPlacement(a, albums, c.multi)
			if !slices.Equal(project(join), c.join) || !slices.Equal(project(leave), c.leave) {
				t.Errorf("DecideFirstPlacement = %+v, %+v; want %+v, %+v", join, leave, c.join, c.leave)
			}
			for _, x := range slices.Concat(c.join, c.leave) {
				if _, ok := domain.Rules[x.rule]; !ok {
					t.Errorf("%s is not in domain.Rules", x.rule)
				}
			}
			reversed, all := a, slices.Clone(albums)
			reversed.Memberships = slices.Clone(a.Memberships)
			slices.Reverse(reversed.Memberships)
			slices.Reverse(all)
			if j, l := plan.DecideFirstPlacement(reversed, all, c.multi); !slices.Equal(j, join) || !slices.Equal(l, leave) {
				t.Errorf("with the input reversed: %+v, %+v; want %+v, %+v", j, l, join, leave)
			}
		})
	}
}
