package layout_test

import (
	"fmt"
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

// covers: R-LAYOUT-06
func TestDirFor(t *testing.T) {
	t.Parallel()

	const rule = "R-LAYOUT-06"
	std := domain.DefaultPolicies()
	photos, cloud := std.Templates.Photos, std.Templates.Cloud
	own := tpl(photos, []string{"{YYYY}", "{MM}-{DD} {Description}"}, "")
	dots := tpl(photos, []string{"..", "{YYYY}", "{MM}-{DD} - {Description}"}, "")
	events := tpl([]string{"{YYYY}", "Events", "{MM}/{DD} {Description}"}, cloud, "")
	photosDir := tpl(photos, []string{"{YYYY}", "Photos", "{MM}-{DD} - {Description}"}, "")
	monthly := tpl([]string{"{YYYY}-{MM}", "{DD} {Description}"}, cloud, "")
	yy := tpl(photos, []string{"{YY}", "{MM}-{DD} {Description}"}, "")
	byName := tpl([]string{"{YYYY}", "{DD} {Month} {Description}"}, cloud, "ru")
	textBoth := tpl([]string{"Events", "{YYYY}", "{MM}/{DD} {Description}"}, []string{"Photos", "{YYYY}", "{MM}-{DD} - {Description}"}, "")
	split := tpl([]string{"{YYYY}", "{MM}", "{DD} {Description}"}, cloud, "")
	folder := domain.KindFolder
	in := func(titles ...string) []string { return titles }

	cases := []struct {
		name  string
		p     domain.Policies
		album domain.Album
		want  string
	}{
		{"an event", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, "2024/05-01 - Holidays"},
		{"an event without a description", std, domain.Album{FolderTitles: in("2024"), Title: "05/01"}, "2024/05-01"},
		{"the cloud template of the user", own, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, "2024/05-01 Holidays"},
		{"a description with slashes and dots stays one level", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 ../a/.."}, "2024/05-01 - ..∕a∕.．"},
		{"a cloud level that sanitizing changes gives none", dots, domain.Album{FolderTitles: in("2024"), Title: "05/01 x"}, ""},
		{"a description of control characters gives none", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 \x00"}, ""},
		{"the text before an empty description", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 "}, ""},
		{"an album without a date", std, domain.Album{FolderTitles: in("2024"), Title: "Holidays"}, ""},
		{"an album deeper than the template", std, domain.Album{FolderTitles: in("2024", "Trips"), Title: "05/01 Kazan"}, ""},
		{"a top-level album", std, domain.Album{Title: "Pets"}, ""},
		{"an album at the depth of a folder", std, domain.Album{Title: "2024"}, ""},
		{"a year folder", std, domain.Album{Kind: folder, Title: "2024"}, "2024"},
		{"a folder at the depth of an event", std, domain.Album{Kind: folder, FolderTitles: in("2024"), Title: "05/01 Trip"}, ""},
		{"a folder outside the template", std, domain.Album{Kind: folder, Title: "Hobby"}, ""},
		{"a folder above a level of text", events, domain.Album{Kind: folder, FolderTitles: in("2024"), Title: "Events"}, "2024"},
		{"a folder that a level of text follows", events, domain.Album{Kind: folder, Title: "2024"}, ""},
		{"a level of text after the folder in the cloud", photosDir, domain.Album{Kind: folder, Title: "2024"}, "2024/Photos"},
		{"a folder with a field that the cloud levels above an event lack", monthly, domain.Album{Kind: folder, Title: "2024-05"}, ""},
		{"a folder level of a field that the cloud levels above an event lack", split, domain.Album{Kind: folder, FolderTitles: in("2024"), Title: "05"}, ""},
		{"an event of that template", monthly, domain.Album{FolderTitles: in("2024-05"), Title: "01 x"}, "2024/05-01 - x"},
		{"{YY} out of its years", yy, domain.Album{FolderTitles: in("1965"), Title: "05/01 x"}, ""},
		{"{YY} within its years", yy, domain.Album{FolderTitles: in("1969"), Title: "05/01 x"}, "69/05-01 x"},
		{"a month by name", byName, domain.Album{FolderTitles: in("2024"), Title: "01 Май x"}, "2024/05-01 - x"},
		{"the state fields and a stored kind are not read", std, domain.Album{
			ID: 7, UUID: "uuid-1", Key: "key-1", KeySource: domain.KeyCloudGUID, InTrash: true,
			Start: time.Date(2024, 5, 2, 9, 0, 0, 0, time.UTC), ParentID: 3, ParentUUID: "uuid-0", Kind: domain.KindThematic,
			FolderTitles: in("2024"), Title: "05/02 Old",
		}, "2024/05-02 - Old"},
		{"templates that Parse refuses", domain.Policies{}, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, ""},
		{"a description that sanitizing changes", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 a:b"}, "2024/05-01 - a꞉b"},
		{"a folder of text on both sides", textBoth, domain.Album{Kind: folder, Title: "Events"}, "Photos"},
		{"a folder of other text", textBoth, domain.Album{Kind: folder, Title: "Other"}, ""},
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
			if got == "" {
				return
			}
			levels := strings.Split(got, "/")
			for _, level := range levels {
				if !naming.IsValidPart(level) || level == "." || level == ".." {
					t.Errorf("DirFor(%+v) = %q: level %q is not a valid name", c.album, got, level)
				}
			}
			if c.album.Kind != folder && len(levels) != len(c.p.Templates.Cloud) {
				t.Errorf("DirFor(%+v) = %q: %d levels, want %d as in the cloud template", c.album, got, len(levels), len(c.p.Templates.Cloud))
			}
		})
	}
}

// covers: R-LAYOUT-06
// covers: R-LAYOUT-09
func TestAlbumFor(t *testing.T) {
	t.Parallel()

	const r06, r09 = "R-LAYOUT-06", "R-LAYOUT-09"
	std := domain.DefaultPolicies()
	photos, cloud := std.Templates.Photos, std.Templates.Cloud
	own := tpl(photos, []string{"{YYYY}", "{MM}-{DD} {Description}"}, "")
	dots := tpl(photos, []string{"..", "{YYYY}", "{MM}-{DD} - {Description}"}, "")
	events := tpl([]string{"{YYYY}", "Events", "{MM}/{DD} {Description}"}, cloud, "")
	photosDir := tpl(photos, []string{"{YYYY}", "Photos", "{MM}-{DD} - {Description}"}, "")
	monthly := tpl([]string{"{YYYY}-{MM}", "{DD} {Description}"}, cloud, "")
	yy := tpl(photos, []string{"{YY}", "{MM}-{DD} {Description}"}, "")
	nameDirs := tpl(photos, []string{"{YYYY}", "{DD} {Month} {Description}"}, "ru")
	word := tpl(photos, []string{"{YYYY} год", "{MM}-{DD} {Description}"}, "")
	lead := tpl(photos, []string{"Photos", "{YYYY}", "{MM}-{DD} - {Description}"}, "")
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
		want       domain.Album
		ok         bool
	}{
		{r06, "an event directory", std, "2024/05-01 - Holidays", album(in("2024"), "05/01 Holidays"), true},
		{r09, "a former day folder is an event without a description", std, "2024/05-01", album(in("2024"), "05/01"), true},
		{r09, "the text before an empty description", std, "2024/05-01 - ", none, false},
		{r06, "a year directory is a folder", std, "2024", folder(nil, "2024"), true},
		{r09, "the root", std, "", none, false},
		{r09, "the root as path.Dir gives it", std, ".", none, false},
		{r09, "a directory below an event", std, "2024/05-01 - Holidays/raw", none, false},
		{r09, "a directory outside the template", std, "Albums/Pets", none, false},
		{r09, "an irregular name", std, "2024/Vacation", none, false},
		{r06, "the description as it is, look-alikes too", std, "2024/05-01 - a∕b", album(in("2024"), "05/01 a∕b"), true},
		{r06, "the description keeps its case", std, "2024/05-01 - HOLIDAYS", album(in("2024"), "05/01 HOLIDAYS"), true},
		{r06, "text of the template in another case", word, "2024 ГОД", folder(nil, "2024"), true},
		{r06, "the same folder by the text of the template", word, "2024 год", folder(nil, "2024"), true},
		{r09, "a month name in NFD is not read", nameDirs, "2024/01 Май x", none, false},
		{r06, "a month name in NFC", nameDirs, "2024/01 Май x", album(in("2024"), "05/01 x"), true},
		{r06, "a level of text above an event", photosDir, "2024/Photos", folder(nil, "2024"), true},
		{r09, "the directory that the level of text follows", photosDir, "2024", none, false},
		{r06, "a folder that a level of text follows in Photos", events, "2024", folder(in("2024"), "Events"), true},
		{r09, "a year that Photos cannot write alone", monthly, "2024", none, false},
		{r06, "the cloud template of the user", own, "2024/05-01 Holidays", album(in("2024"), "05/01 Holidays"), true},
		{r06, "text that the template lacks goes to the description", own, "2024/05-01 - Holidays", album(in("2024"), "05/01 - Holidays"), true},
		{r06, "{YY} reads by the rule of time.Parse", yy, "70/05-01 x", album(in("1970"), "05/01 x"), true},
		{r09, "a look-alike of the text of the template", dots, "．．/2024/05-01 - x", none, false},
		{r09, "February 29 of another year", std, "2023/02-29 - x", none, false},
		{r09, "a level of text without a folder in Photos", lead, "Photos", none, false},
	}
	for _, c := range cases {
		t.Run(c.rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[c.rule]; !ok {
				t.Errorf("%s is not in domain.Rules", c.rule)
			}
			got, ok := layout.AlbumFor(c.dir, c.p)
			// A top-level album has no folder titles, empty or nil alike.
			if len(got.FolderTitles) == 0 {
				got.FolderTitles = nil
			}
			if ok != c.ok || !reflect.DeepEqual(got, c.want) {
				t.Errorf("AlbumFor(%q) = %+v, %t; want %+v, %t", c.dir, got, ok, c.want, c.ok)
			}
		})
	}
}

// covers: R-LAYOUT-06
func TestDirProperties(t *testing.T) {
	t.Parallel()

	std := domain.DefaultPolicies()
	photos, cloud := std.Templates.Photos, std.Templates.Cloud
	layouts := rapid.SampledFrom([]domain.Policies{
		std,
		tpl(photos, []string{"{YYYY}", "{MM}-{DD} {Description}"}, ""),
		tpl([]string{"{YYYY}", "Events", "{MM}/{DD} {Description}"}, cloud, ""),
		tpl(photos, []string{"{YYYY}", "Photos", "{MM}-{DD} - {Description}"}, ""),
		tpl([]string{"{YYYY}-{MM}", "{DD} {Description}"}, cloud, ""),
	})
	text := rapid.StringOfN(rapid.OneOf(rapid.SampledFrom([]rune("/ .:05-\x00")), rapid.Rune()), 0, 30, -1)
	folders := rapid.OneOf(rapid.SampledFrom([]string{"2024", "1999", "Events", "Trips"}), text)
	days := rapid.SampledFrom([]string{"01/01", "02/29", "05/01", "12/31"})
	words := rapid.SampledFrom([]string{"Holidays", "Trips", "Отпуск", "a∕b"})
	titles := rapid.OneOf(days, rapid.Custom(func(t *rapid.T) string { return days.Draw(t, "day") + " " + words.Draw(t, "word") }), words, text)

	t.Run("levels are valid, at most one per template level", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			p := layouts.Draw(t, "templates")
			a := domain.Album{FolderTitles: rapid.SliceOfN(folders, 0, 3).Draw(t, "folders"), Title: titles.Draw(t, "title")}
			if rapid.Bool().Draw(t, "isFolder") {
				a.Kind = domain.KindFolder
			}

			dir := layout.DirFor(a, p)
			if dir == "" {
				return
			}
			levels := strings.Split(dir, "/")
			for _, level := range levels {
				if !naming.IsValidPart(level) || level == "." || level == ".." {
					t.Fatalf("DirFor(%+v, %+v) = %q: level %q is not a valid name", a, p.Templates, dir, level)
				}
			}
			if n := len(p.Templates.Cloud); len(levels) > n || a.Kind != domain.KindFolder && len(levels) != n {
				t.Fatalf("DirFor(%+v, %+v) = %q: %d levels for a cloud template of %d", a, p.Templates, dir, len(levels), n)
			}
		})
	})

	t.Run("an event comes back", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			y := rapid.IntRange(1, 9999).Draw(t, "year")
			m := rapid.IntRange(1, 12).Draw(t, "month")
			d := rapid.IntRange(1, time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()).Draw(t, "day")
			desc := rapid.String().Draw(t, "description")
			a := domain.Album{FolderTitles: []string{fmt.Sprintf("%04d", y)}, Title: fmt.Sprintf("%02d/%02d", m, d)}
			day := fmt.Sprintf("%02d-%02d", m, d)
			raw := []string{a.FolderTitles[0], day}
			if desc != "" {
				a.Title += " " + desc
				raw[1] += " - " + desc
			}
			want := raw[0] + "/" + naming.SanitizePart(raw[1])
			// Sanitizing drops control characters and cuts a long name without
			// splitting a letter from its marks: when that takes the whole
			// description, the text before it no longer reads, so no directory
			// holds the album.
			if desc != "" && !strings.HasPrefix(naming.SanitizePart(raw[1]), day+" - ") {
				want = ""
			}

			dir := layout.DirFor(a, std)
			if dir != want {
				t.Fatalf("DirFor(%+v) = %q, want %q", a, dir, want)
			}
			if dir == "" {
				return
			}
			got, ok := layout.AlbumFor(dir, std)
			if !ok || layout.DirFor(got, std) != dir {
				t.Fatalf("AlbumFor(%q) = %+v, %t; want an album that DirFor gives that directory", dir, got, ok)
			}
			if naming.SanitizePart(raw[1]) == raw[1] && (got.Title != a.Title || !slices.Equal(got.FolderTitles, a.FolderTitles)) {
				t.Fatalf("AlbumFor(%q) = %+v; want %+v back", dir, got, a)
			}
		})
	})

	t.Run("a year folder comes back", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			f := domain.Album{Kind: domain.KindFolder, Title: fmt.Sprintf("%04d", rapid.IntRange(1, 9999).Draw(t, "year"))}

			dir := layout.DirFor(f, std)
			got, ok := layout.AlbumFor(dir, std)
			if !ok || got.Kind != domain.KindFolder || got.Title != f.Title || len(got.FolderTitles) != 0 {
				t.Fatalf("AlbumFor(DirFor(%+v) = %q) = %+v, %t; want the folder back", f, dir, got, ok)
			}
		})
	})
}
