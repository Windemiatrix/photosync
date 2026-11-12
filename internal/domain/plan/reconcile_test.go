package plan_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
	"github.com/Windemiatrix/photosync/internal/testkit"
)

// Pairs and links refer to an album by its ID from run to run, so an album
// keeps it while it lives; the answer must not depend on the order of the
// input.
func TestReconcileAlbums(t *testing.T) {
	t.Parallel()

	type row struct {
		uuid       string
		id, parent domain.AlbumID
		kind       domain.AlbumKind
		key        string
		source     domain.KeySource
	}
	const (
		folder, event, thematic      = domain.KindFolder, domain.KindEvent, domain.KindThematic
		srcPath, srcGUID, srcCloudID = domain.KeyPath, domain.KeyCloudGUID, domain.KeyCloudID
	)
	with := func(a domain.Album, change func(*domain.Album)) domain.Album {
		change(&a)
		return a
	}
	year := domain.Album{UUID: "u-f24", Kind: folder, Title: "2024"}
	holidays := domain.Album{
		UUID: "u-hol", Title: "05/01 Holidays", ParentUUID: "u-f24", FolderTitles: []string{"2024"},
		Start: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC),
	}
	pets := domain.Album{UUID: "u-pets", Title: "Pets"}
	oldYear := domain.Album{ID: 10, UUID: "u-f24", Key: `[] "2024"`, KeySource: srcPath, Kind: folder, Title: "2024"}
	oldHolidays := domain.Album{
		ID: 20, UUID: "u-hol", Key: `["2024"] "05/01 Holidays"`, KeySource: srcPath, Kind: event,
		Title: "05/01 Holidays", ParentID: 10,
	}
	oldPets := domain.Album{ID: 30, UUID: "u-pets", Key: `[] "Pets"`, KeySource: srcPath, Kind: thematic, Title: "Pets"}
	known := []domain.Album{oldYear, oldHolidays, oldPets}
	knownRows := []row{
		{"u-f24", 10, 0, folder, `[] "2024"`, srcPath},
		{"u-hol", 20, 10, event, `["2024"] "05/01 Holidays"`, srcPath},
		{"u-pets", 30, 0, thematic, `[] "Pets"`, srcPath},
	}
	trips := []domain.Album{
		{ID: 4, UUID: "u-x", Key: `[] "Trips"`, KeySource: srcPath, Kind: folder, Title: "Trips"},
		{ID: 5, UUID: "u-y", Key: `[] "Trips"`, KeySource: srcPath, Kind: thematic, Title: "Trips"},
	}

	cases := []struct {
		name        string
		state, snap []domain.Album
		want        []row
		bad         bool
	}{
		{
			name: "a new library gets IDs in the order of UUID",
			snap: []domain.Album{pets, holidays, year},
			want: []row{
				{"u-f24", 1, 0, folder, `[] "2024"`, srcPath},
				{"u-hol", 2, 1, event, `["2024"] "05/01 Holidays"`, srcPath},
				{"u-pets", 3, 0, thematic, `[] "Pets"`, srcPath},
			},
		},
		{
			name:  "a known library keeps the IDs of the state",
			state: known,
			snap:  []domain.Album{year, holidays, pets},
			want:  knownRows,
		},
		{
			name:  "a renamed album keeps its ID by UUID and takes the new path key",
			state: known,
			snap:  []domain.Album{year, holidays, with(pets, func(a *domain.Album) { a.Title = "Dogs" })},
			want:  append(slices.Clone(knownRows[:2]), row{"u-pets", 30, 0, thematic, `[] "Dogs"`, srcPath}),
		},
		{
			name:  "a moved album keeps its ID and is classified anew",
			state: known,
			snap: []domain.Album{year, holidays, with(pets, func(a *domain.Album) {
				a.ParentUUID, a.FolderTitles = "u-f24", []string{"2024"}
			})},
			want: append(slices.Clone(knownRows[:2]), row{"u-pets", 30, 10, event, `["2024"] "Pets"`, srcPath}),
		},
		{
			name:  "a library made anew: path keys find the albums",
			state: known,
			snap: []domain.Album{
				with(year, func(a *domain.Album) { a.UUID = "u-f24b" }),
				with(holidays, func(a *domain.Album) { a.UUID, a.ParentUUID = "u-holb", "u-f24b" }),
				with(pets, func(a *domain.Album) { a.UUID = "u-petsb" }),
			},
			want: []row{
				{"u-f24b", 10, 0, folder, `[] "2024"`, srcPath},
				{"u-holb", 20, 10, event, `["2024"] "05/01 Holidays"`, srcPath},
				{"u-petsb", 30, 0, thematic, `[] "Pets"`, srcPath},
			},
		},
		{
			name:  "a library made anew: a cloud GUID finds the album",
			state: []domain.Album{{ID: 7, UUID: "u-1", Key: "G-1", KeySource: srcGUID, Kind: thematic, Title: "Pets"}},
			snap:  []domain.Album{{UUID: "u-2", Key: "G-1", KeySource: srcGUID, Title: "Dogs"}},
			want:  []row{{"u-2", 7, 0, thematic, "G-1", srcGUID}},
		},
		{
			name:  "a library made anew: a PhotoKit cloud identifier finds the album",
			state: []domain.Album{{ID: 7, UUID: "u-1", Key: "C-1", KeySource: srcCloudID, Kind: thematic, Title: "Pets"}},
			snap:  []domain.Album{{UUID: "u-2", Key: "C-1", KeySource: srcCloudID, Title: "Pets"}},
			want:  []row{{"u-2", 7, 0, thematic, "C-1", srcCloudID}},
		},
		{
			name:  "a library made anew with new cloud keys: a new album",
			state: []domain.Album{{ID: 7, UUID: "u-1", Key: "G-1", KeySource: srcGUID, Kind: thematic, Title: "Pets"}},
			snap:  []domain.Album{{UUID: "u-2", Key: "G-9", KeySource: srcGUID, Title: "Pets"}},
			want:  []row{{"u-2", 8, 0, thematic, "G-9", srcGUID}},
		},
		{
			name:  "a cloud GUID appears: the UUID keeps the album",
			state: []domain.Album{oldPets},
			snap:  []domain.Album{with(pets, func(a *domain.Album) { a.Key, a.KeySource = "G-1", srcGUID })},
			want:  []row{{"u-pets", 30, 0, thematic, "G-1", srcGUID}},
		},
		{
			name:  "a PhotoKit cloud identifier gives way to a cloud GUID",
			state: []domain.Album{{ID: 7, UUID: "u-1", Key: "C-1", KeySource: srcCloudID, Kind: thematic, Title: "Pets"}},
			snap:  []domain.Album{{UUID: "u-1", Key: "G-1", KeySource: srcGUID, Title: "Pets"}},
			want:  []row{{"u-1", 7, 0, thematic, "G-1", srcGUID}},
		},
		{
			name:  "a new album takes the path and the ID of a deleted one",
			state: []domain.Album{oldPets},
			snap:  []domain.Album{{UUID: "u-new", Title: "Pets"}},
			want:  []row{{"u-new", 30, 0, thematic, `[] "Pets"`, srcPath}},
		},
		{
			name:  "a key of an album found by UUID passes to no other",
			state: []domain.Album{oldPets},
			snap:  []domain.Album{{UUID: "u-new", Title: "Pets"}, with(pets, func(a *domain.Album) { a.Title = "Dogs" })},
			want: []row{
				{"u-pets", 30, 0, thematic, `[] "Dogs"`, srcPath},
				{"u-new", 31, 0, thematic, `[] "Pets"`, srcPath},
			},
		},
		{
			name: "a renamed album may take the path key of a deleted one",
			state: []domain.Album{
				oldPets,
				{ID: 40, UUID: "u-dogs", Key: `[] "Dogs"`, KeySource: srcPath, Kind: thematic, Title: "Dogs"},
			},
			snap: []domain.Album{with(pets, func(a *domain.Album) { a.Title = "Dogs" })},
			want: []row{{"u-pets", 30, 0, thematic, `[] "Dogs"`, srcPath}},
		},
		{
			name: "a path key that two albums of the snapshot share finds neither",
			state: []domain.Album{
				oldYear,
				{ID: 40, UUID: "u-trip", Key: `["2024"] "Trips"`, KeySource: srcPath, Kind: event, Title: "Trips", ParentID: 10},
			},
			snap: []domain.Album{
				year,
				{UUID: "u-b", Title: "Trips", ParentUUID: "u-f24", FolderTitles: []string{"2024"}},
				{UUID: "u-a", Title: "Trips", ParentUUID: "u-f24", FolderTitles: []string{"2024"}, InTrash: true},
			},
			want: []row{
				{"u-f24", 10, 0, folder, `[] "2024"`, srcPath},
				{"u-a", 41, 10, event, `["2024"] "Trips"`, srcPath},
				{"u-b", 42, 10, event, `["2024"] "Trips"`, srcPath},
			},
		},
		{
			name:  "a path key that two albums of the state share finds neither",
			state: trips,
			snap:  []domain.Album{{UUID: "u-z", Title: "Trips"}},
			want:  []row{{"u-z", 6, 0, thematic, `[] "Trips"`, srcPath}},
		},
		{
			name:  "a folder and an album with one path keep their IDs by UUID",
			state: trips,
			snap:  []domain.Album{{UUID: "u-y", Title: "Trips"}, {UUID: "u-x", Kind: folder, Title: "Trips"}},
			want: []row{
				{"u-x", 4, 0, folder, `[] "Trips"`, srcPath},
				{"u-y", 5, 0, thematic, `[] "Trips"`, srcPath},
			},
		},
		{
			name: "new IDs come above the highest ID of the state, in the order of UUID",
			state: []domain.Album{
				{ID: 2, UUID: "u-z", Key: `[] "Z"`, KeySource: srcPath, Kind: thematic, Title: "Z"},
				{ID: 9, UUID: "u-9", Key: `[] "Old"`, KeySource: srcPath, Kind: thematic, Title: "Old"},
			},
			snap: []domain.Album{{UUID: "u-b", Title: "Alpha"}, {UUID: "u-z", Title: "Z"}, {UUID: "u-a", Title: "Bravo"}},
			want: []row{
				{"u-z", 2, 0, thematic, `[] "Z"`, srcPath},
				{"u-a", 10, 0, thematic, `[] "Bravo"`, srcPath},
				{"u-b", 11, 0, thematic, `[] "Alpha"`, srcPath},
			},
		},
		{
			name: "the kind comes from the year folder, and a folder may get its ID after its album",
			snap: []domain.Album{
				{UUID: "u-lake", Title: "05/03 Lake", ParentUUID: "u-wk", FolderTitles: []string{"2024", "Weekends"}},
				{UUID: "u-wk", Kind: folder, Title: "Weekends", ParentUUID: "u-f24", FolderTitles: []string{"2024"}},
				year,
				{UUID: "u-kites", Title: "Kites", ParentUUID: "u-hob", FolderTitles: []string{"Hobby"}},
				{UUID: "u-hob", Kind: folder, Title: "Hobby"},
			},
			want: []row{
				{"u-f24", 1, 0, folder, `[] "2024"`, srcPath},
				{"u-hob", 2, 0, folder, `[] "Hobby"`, srcPath},
				{"u-kites", 3, 2, thematic, `["Hobby"] "Kites"`, srcPath},
				{"u-lake", 4, 5, event, `["2024" "Weekends"] "05/03 Lake"`, srcPath},
				{"u-wk", 5, 1, folder, `["2024"] "Weekends"`, srcPath},
			},
		},
		{
			name: "two albums with one cloud key",
			snap: []domain.Album{
				{UUID: "u-1", Key: "G-1", KeySource: srcGUID, Title: "A"},
				{UUID: "u-2", Key: "G-1", KeySource: srcGUID, Title: "B"},
			},
			bad: true,
		},
		{
			name: "an album in a folder that the snapshot lacks",
			snap: []domain.Album{{UUID: "u-1", Title: "A", ParentUUID: "u-gone", FolderTitles: []string{"X"}}},
			bad:  true,
		},
		{name: "an empty library"},
		{name: "every album of the state is gone", state: known},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			before := slices.Clone(c.snap)
			got, err := plan.ReconcileAlbums(c.state, c.snap)
			if c.bad {
				if got != nil || !errors.Is(err, plan.ErrBadSnapshot) {
					t.Fatalf("ReconcileAlbums = %+v, %v; want nil and ErrBadSnapshot", got, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ReconcileAlbums: %v", err)
			}
			if !reflect.DeepEqual(c.snap, before) {
				t.Errorf("ReconcileAlbums changed the snapshot to %+v", c.snap)
			}
			var rows []row
			for _, g := range got {
				rows = append(rows, row{g.UUID, g.ID, g.ParentID, g.Kind, g.Key, g.KeySource})
				i := slices.IndexFunc(c.snap, func(s domain.Album) bool { return s.UUID == g.UUID })
				if i < 0 {
					t.Errorf("album %+v is not in the snapshot", g)
					continue
				}
				s := c.snap[i]
				s.ID, s.ParentID, s.Kind, s.Key, s.KeySource = g.ID, g.ParentID, g.Kind, g.Key, g.KeySource
				if !reflect.DeepEqual(g, s) {
					t.Errorf("album %+v; want %+v: the album of the snapshot with the IDs, kind and key", g, s)
				}
			}
			if !slices.Equal(rows, c.want) {
				t.Errorf("rows = %+v; want %+v", rows, c.want)
			}
			state, snap := slices.Clone(c.state), slices.Clone(c.snap)
			slices.Reverse(state)
			slices.Reverse(snap)
			if again, err := plan.ReconcileAlbums(state, snap); err != nil || !reflect.DeepEqual(again, got) {
				t.Errorf("with the input reversed: %+v, %v; want %+v", again, err, got)
			}
			if again, err := plan.ReconcileAlbums(got, c.snap); err != nil || !reflect.DeepEqual(again, got) {
				t.Errorf("a second run on its own result: %+v, %v; want %+v", again, err, got)
			}
		})
	}
}

// A library made anew gives every album a new UUID: an album keeps its ID by
// its cloud key while iCloud keeps the key, and by its path when it has none.
func TestReconcileAlbumsRebuild(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(domain.LibrarySnapshot{})
	year, err := lib.CreateFolder(ctx, "2024", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lib.CreateAlbum(ctx, "05/01 Holidays", year); err != nil {
		t.Fatal(err)
	}
	lib.SyncCloud()
	if _, err := lib.CreateAlbum(ctx, "Pets", ""); err != nil {
		t.Fatal(err)
	}
	var runs [3]map[string]domain.Album
	var state []domain.Album
	for n, rebuild := range []func(){func() {}, func() { lib.Rebuild(true) }, func() { lib.Rebuild(false) }} {
		rebuild()
		snap, err := lib.Snapshot(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		if state, err = plan.ReconcileAlbums(state, snap.Albums); err != nil {
			t.Fatal(err)
		}
		runs[n] = map[string]domain.Album{}
		for _, a := range state {
			runs[n][a.Title] = a
		}
	}
	for _, c := range []struct {
		title        string
		source       domain.KeySource
		keepsOnRekey bool
	}{
		{"2024", domain.KeyCloudGUID, false},
		{"05/01 Holidays", domain.KeyCloudGUID, false},
		{"Pets", domain.KeyPath, true},
	} {
		first, kept, rekeyed := runs[0][c.title], runs[1][c.title], runs[2][c.title]
		switch {
		case first.ID == 0 || kept.ID == 0 || rekeyed.ID == 0:
			t.Errorf("%q is missing from a run: %+v, %+v, %+v", c.title, first, kept, rekeyed)
		case kept.UUID == first.UUID:
			t.Errorf("%q kept its UUID %q in a library made anew", c.title, kept.UUID)
		case kept.ID != first.ID || kept.KeySource != c.source:
			t.Errorf("after a rebuild that keeps cloud keys %q is %+v; want ID %d and key source %s", c.title, kept, first.ID, c.source)
		case (rekeyed.ID == kept.ID) != c.keepsOnRekey:
			t.Errorf("after a rebuild with new cloud keys %q has ID %d, was %d; want the same ID: %v", c.title, rekeyed.ID, kept.ID, c.keepsOnRekey)
		}
	}
	for n, run := range runs {
		if len(run) != 3 || run["05/01 Holidays"].ParentID != run["2024"].ID {
			t.Errorf("run %d: %+v; want three albums, the album in its folder", n, run)
		}
	}
}

// An asset keeps its ID while iCloud or Photos knows it, because links and
// history refer to it by that ID from run to run; the answer must not depend
// on the order of the input, and a run on the state that the delta makes
// changes nothing.
func TestReconcile(t *testing.T) {
	t.Parallel()

	type row struct {
		uuid           string
		id             domain.AssetID
		target         domain.LibraryKind
		albums, hashes string
	}
	const personal, shared = domain.LibraryPersonal, domain.LibraryShared
	shot := func(guid, uuid string, change ...func(*domain.Asset)) domain.Asset {
		a := domain.Asset{
			Key: domain.AssetKey{CloudAssetGUID: guid, UUID: uuid}, Library: personal, OriginalName: "IMG_0001.HEIC",
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100}},
		}
		for _, f := range change {
			f(&a)
		}
		return a
	}
	kept := func(id domain.AssetID, guid, uuid string, change ...func(*domain.Asset)) domain.Asset {
		a := shot(guid, uuid)
		a.ID, a.TargetLibrary = id, personal
		a.Resources = []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100, SHA256: "s1", MD5: "m1"}}
		for _, f := range change {
			f(&a)
		}
		return a
	}
	inShared := func(a *domain.Asset) { a.Library = shared }
	trashed := func(a *domain.Asset) { a.InTrash = true }
	waits := func(l domain.LibraryKind) func(*domain.Asset) { return func(a *domain.Asset) { a.TargetLibrary = l } }
	in := func(uuids ...string) func(*domain.Asset) { return func(a *domain.Asset) { a.AlbumUUIDs = uuids } }
	seen := func(ms ...domain.Membership) func(*domain.Asset) { return func(a *domain.Asset) { a.Memberships = ms } }
	files := func(rs ...domain.Resource) func(*domain.Asset) { return func(a *domain.Asset) { a.Resources = rs } }
	pets := domain.Album{ID: 10, UUID: "al-pets", Key: `[] "Pets"`, KeySource: domain.KeyPath, Kind: domain.KindThematic, Title: "Pets"}
	zoo := domain.Album{ID: 12, UUID: "al-zoo", Key: `[] "Zoo"`, KeySource: domain.KeyPath, Kind: domain.KindThematic, Title: "Zoo"}
	petsNow, zooNow := domain.Album{UUID: "al-pets", Title: "Pets"}, domain.Album{UUID: "al-zoo", Title: "Zoo"}
	trips := domain.Album{UUID: "al-trip", Title: "Trips"}
	entry := func(deleted bool) []domain.AssetHistory {
		return []domain.AssetHistory{{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "G-old", UUID: "u-old"}, Deleted: deleted}}
	}
	replaced := func(deleted bool) domain.State {
		return domain.State{Assets: []domain.Asset{kept(7, "G-new", "u-new")}, History: entry(deleted)}
	}
	replacement := []row{{"u-new", 7, personal, "", "s1/m1"}}
	known := []row{{"u-7", 7, personal, "", "s1/m1"}}

	cases := []struct {
		name    string
		state   domain.State
		snap    domain.LibrarySnapshot
		want    []row
		history []domain.AssetHistory
		notices []domain.Notice
		bad     bool
	}{
		{
			name: "a new library gets IDs in the order of the asset keys",
			snap: domain.LibrarySnapshot{Assets: []domain.Asset{shot("", "u-c"), shot("G-2", "u-a", inShared), shot("", "u-b"), shot("G-1", "u-z")}},
			want: []row{{"u-z", 1, personal, "", "/"}, {"u-a", 2, shared, "", "/"}, {"u-b", 3, personal, "", "/"}, {"u-c", 4, personal, "", "/"}},
		},
		{
			name: "a known library keeps the IDs, target libraries, first runs and hashes of the state",
			state: domain.State{
				Albums: []domain.Album{pets},
				Assets: []domain.Asset{kept(7, "G-7", "u-7", seen(domain.Membership{AlbumID: 10, FirstSeenRun: "r-1"}))},
			},
			snap: domain.LibrarySnapshot{Albums: []domain.Album{petsNow}, Assets: []domain.Asset{shot("G-7", "u-7", in("al-pets"))}},
			want: []row{{"u-7", 7, personal, "10@r-1", "s1/m1"}},
		},
		{
			name:  "a cloud GUID finds an asset of a library made anew",
			state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-7", "u-9")}},
			want:  []row{{"u-9", 7, personal, "", "s1/m1"}},
		},
		{
			name:  "a library made anew with new cloud GUIDs gives a new asset",
			state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-9", "u-9")}},
			want:  []row{{"u-9", 8, personal, "", "/"}},
		},
		{
			name:  "a UUID finds an asset that has got its cloud GUID",
			state: domain.State{Assets: []domain.Asset{kept(7, "", "u-7")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-7", "u-7")}},
			want:  known,
		},
		{
			name:  "a UUID finds an asset that has lost its cloud GUID",
			state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("", "u-7")}},
			want:  known,
		},
		{
			name:  "an asset without a cloud GUID is gone and a new one has none either",
			state: domain.State{Assets: []domain.Asset{kept(7, "", "u-7")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("", "u-8")}},
			want:  []row{{"u-8", 8, personal, "", "/"}},
		},
		{
			name:  "a cloud GUID outweighs the UUID of another asset",
			state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7"), kept(8, "", "u-8")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-7", "u-8")}},
			want:  []row{{"u-8", 7, personal, "", "s1/m1"}},
		},
		{
			name:  "new IDs come above the highest ID of the state, and the assets go in the order of ID",
			state: domain.State{Assets: []domain.Asset{kept(9, "G-a", "u-1"), kept(7, "G-b", "u-2")}},
			snap:  domain.LibrarySnapshot{Assets: []domain.Asset{shot("", "u-4"), shot("G-b", "u-2"), shot("G-c", "u-3"), shot("G-a", "u-1")}},
			want: []row{
				{"u-2", 7, personal, "", "s1/m1"}, {"u-1", 9, personal, "", "s1/m1"},
				{"u-3", 10, personal, "", "/"}, {"u-4", 11, personal, "", "/"},
			},
		},
		{
			name: "an asset that waits to change its library keeps the target of the state, any other follows its library",
			state: domain.State{Assets: []domain.Asset{
				kept(7, "G-7", "u-7", waits(shared)), kept(8, "G-8", "u-8"), kept(9, "G-9", "u-9", waits(shared)),
			}},
			snap: domain.LibrarySnapshot{Assets: []domain.Asset{
				shot("G-7", "u-7"), shot("G-8", "u-8", inShared), shot("G-9", "u-9", inShared), shot("G-6", "u-6", inShared),
			}},
			want: []row{
				{"u-7", 7, shared, "", "s1/m1"}, {"u-8", 8, shared, "", "s1/m1"},
				{"u-9", 9, shared, "", "s1/m1"}, {"u-6", 10, shared, "", "/"},
			},
		},
		{
			name: "a resource keeps its hashes while its role and size stay",
			state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7", files(
				domain.Resource{Role: domain.ResourceRAWPair, Filename: "IMG_0001.DNG", Size: 100, SHA256: "sr", MD5: "mr"},
				domain.Resource{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100, SHA256: "s1", MD5: "m1"},
				domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 50, SHA256: "sv", MD5: "mv"},
			))}},
			snap: domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-7", "u-7", files(
				domain.Resource{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100},
				domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 60},
				domain.Resource{Role: domain.ResourceRAWPair, Filename: "IMG_0001.DNG", Size: 100},
			))}},
			want: []row{{"u-7", 7, personal, "", "s1/m1 / sr/mr"}},
		},
		{
			name: "an asset keeps the first run of an album it stays in and takes this run for a new one",
			state: domain.State{
				Albums: []domain.Album{pets, zoo},
				Assets: []domain.Asset{kept(7, "G-7", "u-7", seen(
					domain.Membership{AlbumID: 12, FirstSeenRun: "r-0"}, domain.Membership{AlbumID: 10, FirstSeenRun: "r-1"},
				))},
			},
			snap: domain.LibrarySnapshot{
				Albums: []domain.Album{petsNow, trips, zooNow},
				Assets: []domain.Asset{shot("G-7", "u-7", in("al-trip", "al-pets"))},
			},
			want: []row{{"u-7", 7, personal, "10@r-1 13@r-2", "s1/m1"}},
		},
		{
			name: "an asset in Recently Deleted stays in its albums",
			state: domain.State{
				Albums: []domain.Album{pets},
				Assets: []domain.Asset{kept(7, "G-7", "u-7", seen(domain.Membership{AlbumID: 10, FirstSeenRun: "r-1"}))},
			},
			snap: domain.LibrarySnapshot{Albums: []domain.Album{petsNow}, Assets: []domain.Asset{shot("G-7", "u-7", in("al-pets"), trashed)}},
			want: []row{{"u-7", 7, personal, "10@r-1", "s1/m1"}},
		},
		{
			name:    "a replacement is not complete while the old asset is outside Recently Deleted",
			state:   replaced(false),
			snap:    domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-new", "u-new"), shot("G-old", "u-old")}},
			want:    replacement,
			history: entry(false),
		},
		{
			name:    "the old asset in Recently Deleted completes a replacement",
			state:   replaced(false),
			snap:    domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-new", "u-new"), shot("G-old", "u-old", trashed)}},
			want:    replacement,
			history: entry(true),
		},
		{
			name:    "an old asset that the user brings back from Recently Deleted is not new and has a notice",
			state:   replaced(true),
			snap:    domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-new", "u-new"), shot("G-old", "u-old")}},
			want:    replacement,
			history: entry(true),
			notices: []domain.Notice{{Code: "plan.replaced_restored", Params: []string{"IMG_0001.HEIC", "u-old"}}},
		},
		{
			name:    "an old asset stays deleted in Recently Deleted",
			state:   replaced(true),
			snap:    domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-new", "u-new"), shot("G-old", "u-old", trashed)}},
			want:    replacement,
			history: entry(true),
		},
		{
			name:    "a library made anew: the cloud GUID finds the old asset and its entry takes the new UUID",
			state:   replaced(false),
			snap:    domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-new", "u-new2"), shot("G-old", "u-old2")}},
			want:    []row{{"u-new2", 7, personal, "", "s1/m1"}},
			history: []domain.AssetHistory{{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "G-old", UUID: "u-old2"}, Deleted: false}},
		},
		{
			name: "old assets gone from the library are deleted, in the order of asset ID and key",
			state: domain.State{
				Assets: []domain.Asset{kept(7, "G-7", "u-7"), kept(9, "G-9", "u-9")},
				History: []domain.AssetHistory{
					{AssetID: 9, Key: domain.AssetKey{CloudAssetGUID: "G-a", UUID: "u-a"}},
					{AssetID: 7, Key: domain.AssetKey{UUID: "u-c"}},
					{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "G-b", UUID: "u-b"}},
				},
			},
			snap: domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-7", "u-7"), shot("G-9", "u-9")}},
			want: []row{{"u-7", 7, personal, "", "s1/m1"}, {"u-9", 9, personal, "", "s1/m1"}},
			history: []domain.AssetHistory{
				{AssetID: 7, Key: domain.AssetKey{CloudAssetGUID: "G-b", UUID: "u-b"}, Deleted: true},
				{AssetID: 7, Key: domain.AssetKey{UUID: "u-c"}, Deleted: true},
				{AssetID: 9, Key: domain.AssetKey{CloudAssetGUID: "G-a", UUID: "u-a"}, Deleted: true},
			},
		},
		{
			name: "two assets with one cloud GUID",
			snap: domain.LibrarySnapshot{Assets: []domain.Asset{shot("G-1", "u-1"), shot("G-1", "u-2")}},
			bad:  true,
		},
		{
			name: "an asset in an album that the snapshot lacks",
			snap: domain.LibrarySnapshot{Albums: []domain.Album{petsNow}, Assets: []domain.Asset{shot("G-1", "u-1", in("al-gone"))}},
			bad:  true,
		},
		{
			name: "two albums with one cloud key",
			snap: domain.LibrarySnapshot{Albums: []domain.Album{
				{UUID: "al-1", Key: "G-9", KeySource: domain.KeyCloudGUID, Title: "A"},
				{UUID: "al-2", Key: "G-9", KeySource: domain.KeyCloudGUID, Title: "B"},
			}},
			bad: true,
		},
		{name: "an empty library"},
		{name: "every asset of the state is gone", state: domain.State{Assets: []domain.Asset{kept(7, "G-7", "u-7")}}},
	}
	deep := func(assets []domain.Asset) []domain.Asset {
		assets = slices.Clone(assets)
		for i := range assets {
			a := &assets[i]
			a.Resources, a.AlbumUUIDs, a.Memberships = slices.Clone(a.Resources), slices.Clone(a.AlbumUUIDs), slices.Clone(a.Memberships)
		}
		return assets
	}
	strip := func(a domain.Asset) domain.Asset {
		a.ID, a.TargetLibrary, a.Memberships, a.Resources = 0, "", nil, slices.Clone(a.Resources)
		for i := range a.Resources {
			a.Resources[i].SHA256, a.Resources[i].MD5 = "", ""
		}
		return a
	}
	project := func(a domain.Asset) row {
		var albums, hashes []string
		for _, m := range a.Memberships {
			albums = append(albums, fmt.Sprintf("%d@%s", m.AlbumID, m.FirstSeenRun))
		}
		for _, r := range a.Resources {
			hashes = append(hashes, r.SHA256+"/"+r.MD5)
		}
		return row{a.Key.UUID, a.ID, a.TargetLibrary, strings.Join(albums, " "), strings.Join(hashes, " ")}
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()

			state, snap := c.state, c.snap
			state.Assets, state.Albums, state.History = deep(c.state.Assets), slices.Clone(c.state.Albums), slices.Clone(c.state.History)
			snap.Assets, snap.Albums = deep(c.snap.Assets), slices.Clone(c.snap.Albums)
			got, notices, err := plan.Reconcile(c.state, c.snap, "r-2")
			if c.bad {
				if !reflect.DeepEqual(got, domain.SnapshotDelta{}) || notices != nil || !errors.Is(err, plan.ErrBadSnapshot) {
					t.Fatalf("Reconcile = %+v, %+v, %v; want nothing and ErrBadSnapshot", got, notices, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Reconcile: %v", err)
			}
			if !reflect.DeepEqual(c.state, state) || !reflect.DeepEqual(c.snap, snap) {
				t.Errorf("Reconcile changed its input to %+v and %+v", c.state, c.snap)
			}
			var rows []row
			for _, g := range got.Assets {
				rows = append(rows, project(g))
				i := slices.IndexFunc(c.snap.Assets, func(s domain.Asset) bool { return s.Key == g.Key })
				if i < 0 {
					t.Errorf("asset %+v is not in the snapshot", g)
				} else if s := c.snap.Assets[i]; !reflect.DeepEqual(strip(g), strip(s)) {
					t.Errorf("asset %+v; want %+v: the asset of the snapshot with what the state adds", g, s)
				}
			}
			if !slices.Equal(rows, c.want) {
				t.Errorf("assets = %+v; want %+v", rows, c.want)
			}
			if !slices.Equal(got.History, c.history) {
				t.Errorf("History = %+v; want %+v", got.History, c.history)
			}
			if !reflect.DeepEqual(notices, c.notices) {
				t.Errorf("notices = %+v; want %+v", notices, c.notices)
			}
			if albums, _ := plan.ReconcileAlbums(c.state.Albums, c.snap.Albums); !reflect.DeepEqual(got.Albums, albums) {
				t.Errorf("Albums = %+v; want those of ReconcileAlbums, %+v", got.Albums, albums)
			}
			slices.Reverse(state.Assets)
			slices.Reverse(state.Albums)
			slices.Reverse(state.History)
			slices.Reverse(snap.Assets)
			slices.Reverse(snap.Albums)
			if again, n, err := plan.Reconcile(state, snap, "r-2"); err != nil || !reflect.DeepEqual(again, got) || !reflect.DeepEqual(n, notices) {
				t.Errorf("with the input reversed: %+v, %+v, %v; want %+v, %+v", again, n, err, got, notices)
			}
			next := domain.State{Assets: got.Assets, Albums: got.Albums, History: got.History}
			if again, n, err := plan.Reconcile(next, c.snap, "r-3"); err != nil || !reflect.DeepEqual(again, got) || !reflect.DeepEqual(n, notices) {
				t.Errorf("a second run on its delta: %+v, %+v, %v; want %+v, %+v", again, n, err, got, notices)
			}
		})
	}
}

// iCloud gives an asset its cloud GUID only once it syncs the library, and a
// library made anew from iCloud gives every asset a new UUID: neither may
// make an asset new, or the plan would upload it again and delete its file.
//
// covers: S16
// covers: S17
func TestReconcileSyncAndRebuild(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	lib := testkit.NewFakePhotoLibrary(domain.LibrarySnapshot{
		Assets: []domain.Asset{
			{Key: domain.AssetKey{UUID: "u-1"}, Library: domain.LibraryPersonal, OriginalName: "IMG_0001.HEIC", AlbumUUIDs: []string{"u-pets"}},
			{Key: domain.AssetKey{UUID: "u-2"}, Library: domain.LibraryPersonal, OriginalName: "IMG_0002.HEIC"},
		},
		Albums: []domain.Album{{UUID: "u-pets", Title: "Pets"}},
	})
	var runs [3]map[string]domain.Asset
	var state domain.State
	for n, change := range []func(){func() {}, lib.SyncCloud, func() { lib.Rebuild(true) }} {
		change()
		snap, err := lib.Snapshot(ctx, "")
		if err != nil {
			t.Fatal(err)
		}
		delta, _, err := plan.Reconcile(state, snap, []string{"r-0", "r-1", "r-2"}[n])
		if err != nil {
			t.Fatal(err)
		}
		state = domain.State{Assets: delta.Assets, Albums: delta.Albums, History: delta.History}
		runs[n] = map[string]domain.Asset{}
		for _, a := range delta.Assets {
			runs[n][a.OriginalName] = a
		}
	}
	for _, name := range []string{"IMG_0001.HEIC", "IMG_0002.HEIC"} {
		first, synced, rebuilt := runs[0][name], runs[1][name], runs[2][name]
		switch {
		case first.ID == 0 || synced.ID != first.ID || rebuilt.ID != first.ID:
			t.Errorf("%s has the IDs %d, %d, %d; want one ID in every run", name, first.ID, synced.ID, rebuilt.ID)
		case first.Key.CloudAssetGUID != "" || synced.Key.CloudAssetGUID == "":
			t.Errorf("%s has the keys %+v, %+v; want a cloud GUID only after the sync", name, first.Key, synced.Key)
		case rebuilt.Key.UUID == synced.Key.UUID || rebuilt.Key.CloudAssetGUID != synced.Key.CloudAssetGUID:
			t.Errorf("%s has the keys %+v, %+v; want a new UUID and the same cloud GUID after the rebuild", name, synced.Key, rebuilt.Key)
		}
	}
	for n, run := range runs {
		if got := run["IMG_0001.HEIC"].Memberships; len(run) != 2 || !slices.Equal(got, []domain.Membership{{AlbumID: 1, FirstSeenRun: "r-0"}}) {
			t.Errorf("run %d: %+v; want two assets, IMG_0001.HEIC in album 1 since run r-0", n, run)
		}
	}
}
