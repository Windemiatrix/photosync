package layout_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

// The policies of the tests: one place, so that a row reads as a path.
func dirPolicies() domain.Policies {
	return domain.Policies{NestedSeparator: " / ", UntitledEvent: "Untitled", AlbumsRoot: "Albums"}
}

// covers: R-LAYOUT-06
func TestDirFor(t *testing.T) {
	t.Parallel()

	const rule = "R-LAYOUT-06"
	std := dirPolicies()
	alt := domain.Policies{NestedSeparator: " > ", UntitledEvent: "Без названия", AlbumsRoot: "Альбомы"}
	bad := domain.Policies{NestedSeparator: " / ", UntitledEvent: "Untitled", AlbumsRoot: "a/b"}
	folder := domain.KindFolder
	in := func(titles ...string) []string { return titles }

	cases := []struct {
		name  string
		p     domain.Policies
		album domain.Album
		want  string
	}{
		{"an event: the day becomes MM-DD - Title", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, "2024/05-01 - Holidays"},
		{"an event without a day keeps its title", std, domain.Album{FolderTitles: in("2024"), Title: "Vacation"}, "2024/Vacation"},
		{"an event in a folder of a year folder", std, domain.Album{FolderTitles: in("2024", "Trips"), Title: "05/01 Kazan"}, "2024/Trips/05-01 - Kazan"},
		{"a nested part is a level", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays / raw"}, "2024/05-01 - Holidays/raw"},
		{"a bare day gets the untitled name", std, domain.Album{FolderTitles: in("2024"), Title: "05/01"}, "2024/05-01 - Untitled"},
		{"every nested part follows the day rule", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Trip / 05/02 Day two"}, "2024/05-01 - Trip/05-02 - Day two"},
		{"an irregular title is kept", std, domain.Album{FolderTitles: in("2024"), Title: "06 - Sea"}, "2024/06 - Sea"},
		{"folder titles are sanitized, not split", std, domain.Album{FolderTitles: in("2024", "a/b"), Title: "c:d"}, "2024/a∕b/c꞉d"},
		{"a thematic album goes under the albums root", std, domain.Album{Title: "Pets"}, "Albums/Pets"},
		{"a thematic album in a folder", std, domain.Album{FolderTitles: in("Hobby"), Title: "3D printing"}, "Albums/Hobby/3D printing"},
		{"a top-level album titled with a year is thematic", std, domain.Album{Title: "2024"}, "Albums/2024"},
		{"a year folder inside another folder is no year folder", std, domain.Album{FolderTitles: in("Archive", "2024"), Title: "Camp"}, "Albums/Archive/2024/Camp"},
		{"a thematic album follows the day rule", std, domain.Album{Title: "05/01 Notes"}, "Albums/05-01 - Notes"},
		{"a thematic album splits into nested parts", std, domain.Album{Title: "Trips / Kazan"}, "Albums/Trips/Kazan"},
		{"a thematic title is sanitized", std, domain.Album{Title: ".hidden"}, "Albums/．hidden"},
		{"a year folder", std, domain.Album{Kind: folder, Title: "2024"}, "2024"},
		{"a folder in a year folder", std, domain.Album{Kind: folder, FolderTitles: in("2024"), Title: "Trips"}, "2024/Trips"},
		{"a folder outside the years", std, domain.Album{Kind: folder, Title: "Hobby"}, "Albums/Hobby"},
		{"a folder title takes no day rule", std, domain.Album{Kind: folder, FolderTitles: in("2024"), Title: "05/01 Trips"}, "2024/05∕01 Trips"},
		{"a folder title does not split", std, domain.Album{Kind: folder, Title: "Trips / Kazan"}, "Albums/Trips ∕ Kazan"},
		{"a folder titled with a year inside another folder", std, domain.Album{Kind: folder, FolderTitles: in("Archive"), Title: "2024"}, "Albums/Archive/2024"},
		{"the separator is a policy", alt, domain.Album{Title: "A > b"}, "Альбомы/A/b"},
		{"the untitled name is a policy", alt, domain.Album{FolderTitles: in("2024"), Title: "05/01"}, "2024/05-01 - Без названия"},
		{"another separator does not split", alt, domain.Album{FolderTitles: in("2024"), Title: "05/01 A / b"}, "2024/05-01 - A ∕ b"},
		{"the albums root is sanitized", bad, domain.Album{Title: "Pets"}, "a∕b/Pets"},
		{"a stored thematic kind is an album", std, domain.Album{Kind: domain.KindThematic, Title: "Pets"}, "Albums/Pets"},
		{"the state fields are not read", std, domain.Album{
			ID: 7, UUID: "uuid-1", Key: "key-1", KeySource: domain.KeyCloudGUID, InTrash: true,
			Start: time.Date(2024, 5, 2, 9, 0, 0, 0, time.UTC), ParentID: 3, ParentUUID: "uuid-0",
			FolderTitles: in("2024"), Title: "05/02 Old",
		}, "2024/05-02 - Old"},
		{"a stored event kind is an album", std, domain.Album{Kind: domain.KindEvent, FolderTitles: in("2024"), Title: "05/01 Holidays"}, "2024/05-01 - Holidays"},
	}
	for _, c := range cases {
		t.Run(rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[rule]; !ok {
				t.Errorf("%s is not in domain.Rules", rule)
			}
			got := layout.DirFor(c.album, c.p)
			if got != c.want {
				t.Errorf("DirFor(%+v) = %q, want %q", c.album, got, c.want)
			}
			for _, level := range strings.Split(got, "/") {
				if !naming.IsValidPart(level) {
					t.Errorf("DirFor(%+v) = %q: level %q is not a valid name", c.album, got, level)
				}
			}
		})
	}
}

// covers: R-LAYOUT-06
// covers: R-LAYOUT-09
func TestAlbumFor(t *testing.T) {
	t.Parallel()

	const r06, r09 = "R-LAYOUT-06", "R-LAYOUT-09"
	std := dirPolicies()
	alt := domain.Policies{NestedSeparator: " > ", UntitledEvent: "Без названия", AlbumsRoot: "Альбомы"}
	bad := domain.Policies{NestedSeparator: " / ", UntitledEvent: "Untitled", AlbumsRoot: "a/b"}
	set := func(dirs ...string) map[string]bool {
		m := map[string]bool{}
		for _, d := range dirs {
			m[d] = true
		}
		return m
	}
	in := func(titles ...string) []string { return titles }
	album := func(folders []string, title string) domain.Album {
		return domain.Album{FolderTitles: folders, Title: title}
	}
	folder := func(folders []string, title string) domain.Album {
		return domain.Album{Kind: domain.KindFolder, FolderTitles: folders, Title: title}
	}
	none := domain.Album{}

	cases := []struct {
		rule, name string
		p          domain.Policies
		dir        string
		albumDirs  map[string]bool
		want       domain.Album
		ok         bool
	}{
		{r09, "the root", std, "", set(""), none, false},
		{r09, "the root as path.Dir gives it", std, ".", set("."), none, false},
		{r09, "a year folder is a folder, its files join no album", std, "2024", set("2024"), folder(nil, "2024"), true},
		{r06, "the albums root", std, "Albums", set("Albums"), none, false},
		{r09, "a day folder is no album", std, "2024/05-01", set("2024/05-01"), none, false},
		{r09, "a subdirectory of a day folder is a part of an untitled event", std, "2024/05-01/raw", set("2024/05-01", "2024/05-01/raw"), album(in("2024"), "05/01 / raw"), true},
		{r09, "deeper under a day folder", std, "2024/05-01/raw/best", nil, album(in("2024"), "05/01 / raw / best"), true},
		{r09, "a day of a leap year only", std, "2023/02-29", set("2023/02-29"), album(in("2023"), "02-29"), true},
		{r09, "a leap day is a day folder", std, "2024/02-29", set("2024/02-29"), none, false},
		{r06, "MM-DD - Title becomes MM/DD Title", std, "2024/05-01 - Holidays", set("2024/05-01 - Holidays"), album(in("2024"), "05/01 Holidays"), true},
		{r09, "Year/Title is an event", std, "2024/Vacation", set("2024/Vacation"), album(in("2024"), "Vacation"), true},
		{r09, "a month folder is an event", std, "2024/05", set("2024/05"), album(in("2024"), "05"), true},
		{r06, "the untitled name gives a bare day", std, "2024/05-01 - Untitled", set("2024/05-01 - Untitled"), album(in("2024"), "05/01"), true},
		{r06, "an irregular name is kept", std, "2024/09-24 Flight home", set("2024/09-24 Flight home"), album(in("2024"), "09-24 Flight home"), true},
		{r06, "a directory under an album is a nested part", std, "2024/Trips/05-01 - Kazan", set("2024/Trips", "2024/Trips/05-01 - Kazan"), album(in("2024"), "Trips / 05/01 Kazan"), true},
		{r06, "a nested part needs no files of its own", std, "2024/05-01 - Holidays/raw", set("2024/05-01 - Holidays"), album(in("2024"), "05/01 Holidays / raw"), true},
		{r06, "a directory without files above an album is a folder", std, "2024/Trips/05-01 - Kazan", set("2024/Trips/05-01 - Kazan"), album(in("2024", "Trips"), "05/01 Kazan"), true},
		{r06, "a directory with no album at or above it is a folder", std, "2024/Trips", set("2024/Trips/05-01 - Kazan"), folder(in("2024"), "Trips"), true},
		{r06, "a directory under the albums root is a thematic album", std, "Albums/Pets", set("Albums/Pets"), album(nil, "Pets"), true},
		{r06, "a folder under the albums root", std, "Albums/Hobby", set("Albums/Hobby/3D printing"), folder(nil, "Hobby"), true},
		{r06, "a day is decoded under the albums root too", std, "Albums/05-01 - Notes", set("Albums/05-01 - Notes"), album(nil, "05/01 Notes"), true},
		{r06, "a thematic album in a folder", std, "Albums/Hobby/3D printing", set("Albums/Hobby/3D printing"), album(in("Hobby"), "3D printing"), true},
		{r06, "a top-level directory is a thematic album", std, "Archive", set("Archive"), album(nil, "Archive"), true},
		{r06, "a nested part of a top-level album", std, "Archive/2019", set("Archive", "Archive/2019"), album(nil, "Archive / 2019"), true},
		{r06, "an album in a top-level folder", std, "Archive/2019", set("Archive/2019"), album(in("Archive"), "2019"), true},
		{r06, "a directory without files keeps its name as a folder", std, "2024/05-01 - Holidays", nil, folder(in("2024"), "05-01 - Holidays"), true},
		{r06, "look-alikes are not decoded", std, "Albums/a∕b", set("Albums/a∕b"), album(nil, "a∕b"), true},
		{r06, "the separator and the root are policies", alt, "Альбомы/A/b", set("Альбомы/A"), album(nil, "A > b"), true},
		{r09, "the untitled name is a policy", alt, "2024/05-01/x", nil, album(in("2024"), "05/01 > x"), true},
		{r06, "the albums root compares sanitized", bad, "a∕b/Pets", set("a∕b/Pets"), album(nil, "Pets"), true},
		{r06, "the albums root compares exactly", std, "albums/Pets", set("albums/Pets"), album(in("albums"), "Pets"), true},
		{r06, "the untitled name of the policies gives a bare day", alt, "2024/05-01 - Без названия", set("2024/05-01 - Без названия"), album(in("2024"), "05/01"), true},
	}
	for _, c := range cases {
		t.Run(c.rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[c.rule]; !ok {
				t.Errorf("%s is not in domain.Rules", c.rule)
			}
			got, ok := layout.AlbumFor(c.dir, c.albumDirs, c.p)
			// A top-level album has no folder titles, empty or nil alike.
			if len(got.FolderTitles) == 0 {
				got.FolderTitles = nil
			}
			if ok != c.ok || !reflect.DeepEqual(got, c.want) {
				t.Errorf("AlbumFor(%q, %v) = %+v, %t; want %+v, %t", c.dir, c.albumDirs, got, ok, c.want, c.ok)
			}
		})
	}
}

// covers: R-LAYOUT-05
func TestDayDir(t *testing.T) {
	t.Parallel()

	const rule = "R-LAYOUT-05"
	east := func(seconds int) *int { return &seconds }

	cases := []struct {
		name   string
		taken  time.Time
		offset *int
		want   string
	}{
		{"the local date, not the UTC one", time.Date(2024, 1, 1, 4, 50, 0, 0, time.UTC), east(-5 * 3600), "2023/12-31"},
		{"a wall clock without a zone", time.Date(2024, 5, 1, 23, 30, 0, 0, time.UTC), nil, "2024/05-01"},
		{"a zone of 5:45", time.Date(2024, 5, 1, 20, 0, 0, 0, time.UTC), east(5*3600 + 45*60), "2024/05-02"},
		{"months and days have two digits", time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC), nil, "2024/01-02"},
	}
	for _, c := range cases {
		t.Run(rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[rule]; !ok {
				t.Errorf("%s is not in domain.Rules", rule)
			}
			got := layout.DayDir(domain.Asset{Taken: c.taken, TZOffset: c.offset})
			if got != c.want {
				t.Errorf("DayDir(%v) = %q, want %q", c.taken, got, c.want)
			}
			if a, ok := layout.AlbumFor(got, map[string]bool{got: true}, dirPolicies()); ok {
				t.Errorf("the day folder %q stands for album %+v, want none", got, a)
			}
		})
	}
}

// covers: R-LAYOUT-06
func TestDirProperties(t *testing.T) {
	t.Parallel()

	p := dirPolicies()
	bools := rapid.SampledFrom([]bool{false, true})
	years := rapid.SampledFrom([]string{"1999", "2024"})
	words := rapid.SampledFrom([]string{"Trips", "Отпуск", "a∕b", "Trip - 2024", "06 - Sea", "09-24 Flight home", "Albums", "_"})
	days := rapid.SampledFrom([]string{"01/01", "02/29", "05/01", "12/31"})
	part := rapid.Custom(func(t *rapid.T) string {
		switch rapid.IntRange(0, 2).Draw(t, "shape") {
		case 0:
			return days.Draw(t, "day")
		case 1:
			return days.Draw(t, "day") + " " + words.Draw(t, "name")
		}
		return words.Draw(t, "name")
	})
	text := rapid.OneOf(
		rapid.StringOfN(rapid.OneOf(rapid.SampledFrom([]rune("/ .:05-\x00")), rapid.Rune()), 0, 30, -1),
		rapid.SampledFrom([]string{" / ", "05/01", "05/01 / raw", "CON", ".", ".."}),
	)

	t.Run("levels are valid, one per folder and title part", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			inYear := bools.Draw(t, "inYear")
			var a domain.Album
			if inYear {
				a.FolderTitles = []string{years.Draw(t, "year")}
			}
			for range rapid.IntRange(0, 2).Draw(t, "folders") {
				a.FolderTitles = append(a.FolderTitles, text.Draw(t, "folder"))
			}
			if !inYear && len(a.FolderTitles) > 0 {
				a.FolderTitles[0] = "x" + a.FolderTitles[0]
			}
			a.Title = text.Draw(t, "title")
			if len(a.FolderTitles) > 0 && bools.Draw(t, "isFolder") {
				a.Kind = domain.KindFolder
			}

			dir := layout.DirFor(a, p)
			levels := strings.Split(dir, "/")
			for _, level := range levels {
				if !naming.IsValidPart(level) {
					t.Fatalf("DirFor(%+v) = %q: level %q is not a valid name", a, dir, level)
				}
			}
			marker, parts, top := 1, strings.Count(a.Title, " / ")+1, "Albums"
			if inYear {
				marker, top = 0, a.FolderTitles[0]
			}
			if a.Kind == domain.KindFolder {
				parts = 1
			}
			if want := marker + len(a.FolderTitles) + parts; len(levels) != want {
				t.Fatalf("DirFor(%+v) = %q: %d levels, want %d", a, dir, len(levels), want)
			}
			if levels[0] != top {
				t.Fatalf("DirFor(%+v) = %q: first level %q, want %q", a, dir, levels[0], top)
			}
		})
	})

	t.Run("a canonical album or folder comes back", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			var a domain.Album
			if bools.Draw(t, "inYear") {
				a.FolderTitles = []string{years.Draw(t, "year")}
			}
			for range rapid.IntRange(0, 2).Draw(t, "folders") {
				a.FolderTitles = append(a.FolderTitles, words.Draw(t, "folder"))
			}
			if bools.Draw(t, "isFolder") {
				a.Kind = domain.KindFolder
				a.Title = words.Draw(t, "title")
			} else {
				a.Title = strings.Join(rapid.SliceOfN(part, 1, 3).Draw(t, "parts"), " / ")
			}

			dir := layout.DirFor(a, p)
			albumDirs := map[string]bool{}
			if a.Kind != domain.KindFolder {
				levels := strings.Split(dir, "/")
				// A wrong DirFor can give fewer levels than parts: that fails
				// the check below instead of panicking here.
				for j := max(len(levels)-strings.Count(a.Title, " / ")-1, 0); j < len(levels); j++ {
					albumDirs[strings.Join(levels[:j+1], "/")] = true
				}
			}
			got, ok := layout.AlbumFor(dir, albumDirs, p)
			if !ok || got.Kind != a.Kind || got.Title != a.Title || !slices.Equal(got.FolderTitles, a.FolderTitles) {
				t.Fatalf("AlbumFor(%q, %v) = %+v, %t; want %+v, true", dir, albumDirs, got, ok, a)
			}
		})
	})
}
