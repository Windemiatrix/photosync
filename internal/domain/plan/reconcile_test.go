package plan_test

import (
	"errors"
	"reflect"
	"slices"
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
