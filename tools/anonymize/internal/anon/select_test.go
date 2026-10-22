package anon

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func mustGraph(t *testing.T, m *Material) *graph {
	t.Helper()
	g, err := newGraph(m)
	if err != nil {
		t.Fatalf("newGraph: %v", err)
	}
	return g
}

func TestClosure(t *testing.T) {
	t.Parallel()
	trash := photo(9, "IMG_0009.HEIC", "2023-01-01T09:00:00+00:00", 9)
	trash.InTrash = true
	// Records: assets 0-2 and the trashed 3; files 4 (A1), 5 (neighbor of
	// 4), 6 (A2), 7 (neighbor of 6), 8 (kind other), 9 (another folder),
	// 10 (A3), 11 (neighbor of 10, linked to A1), 12 (alone).
	tm := testMaterial{
		assets: []Asset{
			photo(1, "IMG_0001.HEIC", "2023-01-01T10:00:00+00:00", 100),
			photo(2, "IMG_0002.HEIC", "2023-01-01T11:00:00+00:00", 200),
			photo(3, "IMG_0003.HEIC", "2023-01-01T12:00:00+00:00", 300),
			trash,
		},
		files: []File{
			cloudFile("2023/a/IMG_0001.HEIC", "", 100),
			cloudFile("2023/a/IMG_0001 (1).MOV", "", 1),
			cloudFile("2023/a/IMG_0002.JPG", "", 2),
			cloudFile("2023/a/img_0002.xmp", "", 1),
			cloudFile("2023/a/IMG_0001.txt", "", 1),
			cloudFile("2023/b/IMG_0001.HEIC", "", 100),
			cloudFile("2023/c/IMG_0003.JPG", "", 3),
			cloudFile("2023/c/IMG_0003.MOV", "", 3),
			cloudFile("2023/a/IMG_0009.HEIC", "", 9),
		},
		links: Links{
			uuidN(1): {{"2023/a/IMG_0001.HEIC", LevelExact}, {"2023/c/IMG_0003.MOV", RefLiveComponent}},
			uuidN(2): {{"2023/a/IMG_0002.JPG", LevelSameShot}},
			uuidN(3): {{"2023/c/IMG_0003.JPG", LevelSameShot}},
		},
	}
	g := mustGraph(t, tm.build())
	tests := []struct {
		name string
		seed []int
		want []int
	}{
		{"asset pulls its files, the neighbors and the chain", []int{0}, []int{0, 2, 4, 5, 10, 11}},
		{"file pulls its owner", []int{4}, []int{0, 2, 4, 5, 10, 11}},
		{"neighbor pulls its group", []int{7}, []int{1, 6, 7}},
		{"other files never enter", []int{8}, nil},
		{"trash never enters", []int{3}, nil},
		{"alone", []int{12}, []int{12}},
		{"another folder", []int{9}, []int{9}},
		{"union", []int{9, 12}, []int{9, 12}},
	}
	for _, tt := range tests {
		got := g.closure(tt.seed)
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: closure(%v) = %v; want %v", tt.name, tt.seed, got, tt.want)
		}
	}
	if n := len(g.members); n != 4 {
		t.Errorf("components = %d; want 4", n)
	}
}

func categoryByName(name string) (Category, bool) {
	for _, c := range Categories {
		if c.Name == name {
			return c, true
		}
	}
	return Category{}, false
}

type categoryCase struct {
	category string
	yes, no  testMaterial
}

func single(a Asset, f File, kind string) testMaterial {
	return testMaterial{assets: []Asset{a}, files: []File{f}, links: Links{a.UUID: {{f.Path, kind}}}}
}

func categoryCases() []categoryCase {
	d := "2023-07-14T18:03:27+03:00"
	fd := "2023-07-14T18:03:27"
	a1 := photo(1, "IMG_0001.HEIC", d, 100)
	f1 := cloudFile("2023/07-14 - A/IMG_0001.HEIC", fd, 100)
	f1j := cloudFile("2023/07-14 - A/IMG_0001.JPG", fd, 50)
	mov := cloudFile("2023/07-14 - A/IMG_0001.MOV", fd, 10)
	exact := single(a1, f1, LevelExact)
	level := func(kind string) categoryCase {
		name := "level-" + strings.ReplaceAll(strings.ReplaceAll(kind, "_", "-"), "live-component", "pair-component")
		yes := single(a1, f1, kind)
		if kind == RefLiveComponent {
			yes = testMaterial{assets: []Asset{a1}, files: []File{f1, mov}, links: Links{a1.UUID: {{f1.Path, LevelExact}, {mov.Path, RefLiveComponent}}}}
		}
		no := exact
		if kind == LevelExact {
			no = single(a1, f1, LevelSameShot)
		}
		return categoryCase{name, yes, no}
	}
	live := a1
	live.LivePhoto = true
	raw := a1
	raw.HasRaw = true
	burst := func(one bool) testMaterial {
		b1, b2 := photo(1, "IMG_0001.HEIC", d, 1), photo(2, "IMG_0002.HEIC", d, 2)
		if one {
			burstOf(&b1, &b2)
		} else {
			// Two bursts whose other frames are not in the catalog.
			b1.Burst, b1.BurstPhotos = true, []string{uuidN(3)}
			b2.Burst, b2.BurstPhotos = true, []string{uuidN(4)}
		}
		b2.Date = "2023-07-14T18:03:29+03:00"
		return testMaterial{assets: []Asset{b1, b2}}
	}
	two := func(d1, d2 string) testMaterial {
		return testMaterial{assets: []Asset{photo(1, "IMG_0001.HEIC", d1, 1), photo(2, "IMG_0002.HEIC", d2, 2)}}
	}
	stems := func(d2 string) testMaterial {
		return testMaterial{assets: []Asset{photo(1, "IMG_0001.HEIC", d, 1), photo(2, "img_0001.jpg", d2, 2)}}
	}
	fileOnly := func(fs ...File) testMaterial { return testMaterial{files: fs} }
	withDate := func(a Asset, date string) Asset { a.Date = date; return a }
	withName := func(a Asset, name string) Asset { a.OriginalFilename = name; return a }
	albums := func(n int) testMaterial {
		a := a1
		for i := 0; i < n; i++ {
			a.Albums = append(a.Albums, AlbumRef{FolderNames: []string{"2023"}, Title: fmt.Sprintf("07/%02d A", i+1)})
		}
		return testMaterial{assets: []Asset{a}}
	}
	edited := a1
	edited.HasAdjustments = true
	fileSHA := func(s1, s2 string) testMaterial {
		x, y := cloudFile("2023/a/x.jpg", "", 1), cloudFile("2023/b/y.jpg", "", 1)
		x.SHA1, y.SHA1 = s1, s2
		return fileOnly(x, y)
	}
	return []categoryCase{
		level(LevelExact), level(LevelSameShot), level(LevelSameShotWeak), level(LevelSameTimeSize), level(LevelProbable), level(RefLiveComponent),
		{"plan-replace", func() testMaterial {
			tm := single(a1, f1j, LevelSameShot)
			tm.replace = [][2]string{{a1.UUID, f1j.Path}}
			return tm
		}(), single(a1, f1j, LevelSameShot)},
		{"plan-keep-cloud", func() testMaterial {
			tm := single(a1, f1j, LevelSameShot)
			tm.keep = []string{f1j.Path}
			return tm
		}(), single(a1, f1j, LevelSameShot)},
		{"unmatched-icloud", testMaterial{assets: []Asset{a1}}, exact},
		{"unmatched-cloud", fileOnly(f1), exact},
		{"live", testMaterial{assets: []Asset{live}, files: []File{f1, mov}, links: Links{a1.UUID: {{f1.Path, LevelExact}, {mov.Path, RefLiveComponent}}}},
			single(live, f1, LevelExact)},
		{"raw-pair", testMaterial{assets: []Asset{raw}}, exact},
		{"raw-pair", fileOnly(cloudFile("2023/a/IMG_0001.DNG", "", 1), cloudFile("2023/a/IMG_0001.JPG", "", 1)),
			fileOnly(cloudFile("2023/a/IMG_0001.DNG", "", 1), cloudFile("2023/b/IMG_0001.JPG", "", 1))},
		{"burst", burst(true), burst(false)},
		{"same-second", two(d, "2023-07-14T18:03:27.900+03:00"), two(d, "2023-07-14T18:03:28+03:00")},
		{"stem-repeat", stems("2019-07-14T18:03:27+03:00"), stems("2023-01-01T00:00:00+03:00")},
		{"suffix-n", fileOnly(cloudFile("2023/a/IMG_0001 (1).JPG", "", 1)), fileOnly(cloudFile("2023/a/IMG_0001(1).JPG", "", 1))},
		{"nfd", fileOnly(cloudFile("2023/a/"+nfd("Ёлка")+".jpg", "", 1)), exact},
		{"non-ascii", fileOnly(cloudFile("2023/a/Ёлка.jpg", "", 1)), fileOnly(cloudFile("2023/Ёлка/a.jpg", "", 1))},
		{"ext-case", single(a1, cloudFile("2023/a/IMG_0001.heic", fd, 100), LevelExact), exact},
		{"fraction", single(a1, cloudFile("2023/a/IMG_0001.HEIC", fd+".455", 100), LevelSameShot), exact},
		{"tz-shift", single(a1, cloudFile("2023/a/IMG_0001.HEIC", "2023-07-14T17:03:27", 100), LevelSameShot),
			single(a1, cloudFile("2023/a/IMG_0001.HEIC", "2023-07-14T18:03:28", 100), LevelSameShot)},
		{"date-1970", testMaterial{assets: []Asset{withName(withDate(a1, "1970-01-01T00:00:00+00:00"), "PXL_20210305_101112345.jpg")}},
			testMaterial{assets: []Asset{withDate(a1, "1970-01-01T00:00:00+00:00")}}},
		{"many-folders", testMaterial{assets: []Asset{a1}, files: []File{f1, cloudFile("2023/b/IMG_0001.HEIC", fd, 100)},
			links: Links{a1.UUID: {{f1.Path, LevelExact}, {"2023/b/IMG_0001.HEIC", LevelExact}}}},
			testMaterial{assets: []Asset{a1}, files: []File{f1, f1j}, links: Links{a1.UUID: {{f1.Path, LevelExact}, {f1j.Path, LevelSameShot}}}}},
		{"album-many", albums(2), albums(1)},
		{"year-boundary", testMaterial{assets: []Asset{withDate(a1, "2023-12-31T23:00:00+03:00")}}, testMaterial{assets: []Asset{a1}}},
		{"year-boundary", single(a1, cloudFile("2022/a/IMG_0001.HEIC", fd, 100), LevelExact), exact},
		{"edited", testMaterial{assets: []Asset{edited}}, exact},
		{"edited", fileOnly(cloudFile("2023/a/IMG_0001_edited.JPG", "", 1)), fileOnly(cloudFile("2023/a/IMG_0001_edit.JPG", "", 1))},
		{"sidecar", fileOnly(cloudFile("2023/a/IMG_0001.xmp", "", 1)), exact},
		{"convert", fileOnly(cloudFile("2023/a/clip.AVI", "", 1)), exact},
		{"icloud-duplicate", testMaterial{assets: []Asset{photo(1, "IMG_0001.HEIC", d, 5), photo(2, "img_0001.heic", d, 5)}},
			testMaterial{assets: []Asset{photo(1, "IMG_0001.HEIC", d, 5), photo(2, "IMG_0001.HEIC", d, 6)}}},
		{"cloud-duplicate", fileSHA("aa", "aa"), fileSHA("aa", "bb")},
	}
}

func TestCategoryPredicates(t *testing.T) {
	t.Parallel()
	covered := map[string]bool{}
	for _, cc := range categoryCases() {
		c, ok := categoryByName(cc.category)
		if !ok {
			t.Errorf("unknown category %q", cc.category)
			continue
		}
		covered[cc.category] = true
		if cc.category != "nfd" || NormalizationAvailable {
			if got := mustGraph(t, cc.yes.build()).candidates(c); len(got) == 0 {
				t.Errorf("%s: no candidate in the yes example", cc.category)
			}
		}
		if got := mustGraph(t, cc.no.build()).candidates(c); len(got) != 0 {
			t.Errorf("%s: candidates %v in the no example", cc.category, got)
		}
	}
	for _, c := range Categories {
		if !covered[c.Name] {
			t.Errorf("category %s has no example", c.Name)
		}
	}
	if len(Categories) != 30 {
		t.Errorf("%d categories; want 30", len(Categories))
	}
}

func TestUnionCategoryPullsGroups(t *testing.T) {
	t.Parallel()
	d := "2023-07-14T18:03:27+03:00"
	b1, b2 := photo(1, "IMG_0001.HEIC", d, 1), photo(2, "IMG_0002.HEIC", "2023-07-14T18:03:29+03:00", 2)
	burstOf(&b1, &b2)
	f2 := cloudFile("2023/a/IMG_0002.HEIC", "", 2)
	tm := testMaterial{assets: []Asset{b1, b2}, files: []File{f2, cloudFile("2023/a/IMG_0002.xmp", "", 1)},
		links: Links{b2.UUID: {{f2.Path, LevelExact}}}}
	g := mustGraph(t, tm.build())
	c, _ := categoryByName("burst")
	got := g.candidates(c)
	if len(got) != 1 || !reflect.DeepEqual(got[0], []int{0, 1, 2, 3}) {
		t.Errorf("burst candidates = %v", got)
	}
}

// manyUnits returns n units of size records each: an asset alone (size 1)
// or an asset linked to one file of a folder with size-1 neighbor files.
func manyUnits(n, size int) *Material {
	tm := testMaterial{links: Links{}}
	for i := 0; i < n; i++ {
		a := photo(i+1, fmt.Sprintf("IMG_%04d.HEIC", i+1), fmt.Sprintf("2023-07-%02dT18:03:27+03:00", 1+i%28), int64(i+1))
		tm.assets = append(tm.assets, a)
		for j := 1; j < size; j++ {
			tm.files = append(tm.files, cloudFile(fmt.Sprintf("2023/u%d/IMG_0001 (%d).JPG", i, j), "", 1))
		}
		if size > 1 {
			tm.links[a.UUID] = []Link{{fmt.Sprintf("2023/u%d/IMG_0001 (1).JPG", i), LevelSameShot}}
		}
	}
	return tm.build()
}

func mustPick(t *testing.T, g *graph, mp *Mapper, category string, cands [][]int) []int {
	t.Helper()
	got, err := g.pickUnits(mp, category, cands)
	if err != nil {
		t.Fatalf("pickUnits: %v", err)
	}
	return got
}

// TestPickOrderIsKeyed checks that the pick follows the keyed hash of the
// salt and the category, not the record order.
func TestPickOrderIsKeyed(t *testing.T) {
	t.Parallel()
	g := mustGraph(t, manyUnits(40, 1))
	c, _ := categoryByName("unmatched-icloud")
	cands := g.candidates(c)
	m1, _ := NewMapper(bytes.Repeat([]byte{1}, 32))
	m2, _ := NewMapper(bytes.Repeat([]byte{2}, 32))
	p1 := mustPick(t, g, m1, c.Name, cands)
	p2 := mustPick(t, g, m2, c.Name, cands)
	p3 := mustPick(t, g, m1, "burst", cands)
	first := []int{0, 1, 2, 3, 4, 5, 6, 7}
	if len(p1) != 8 || len(p2) != 8 {
		t.Fatalf("picked %d and %d units; want 8", len(p1), len(p2))
	}
	if reflect.DeepEqual(p1, p2) {
		t.Errorf("two salts pick the same units %v", p1)
	}
	if reflect.DeepEqual(p1, p3) {
		t.Errorf("two categories pick the same units %v", p1)
	}
	if reflect.DeepEqual(p1, first) || reflect.DeepEqual(p2, first) {
		t.Error("the pick is the first units by record index")
	}
}

func TestPickUnits(t *testing.T) {
	t.Parallel()
	mp := newTestMapper(t)
	c, _ := categoryByName("unmatched-icloud")
	for _, tt := range []struct{ units, want int }{{0, 0}, {1, 1}, {2, 1}, {5, 3}, {16, 8}, {40, 8}} {
		g := mustGraph(t, manyUnits(tt.units, 1))
		cands := g.candidates(c)
		got := mustPick(t, g, mp, c.Name, cands)
		if len(got) != tt.want {
			t.Errorf("%d units: picked %d records; want %d", tt.units, len(got), tt.want)
		}
		// The order of candidates does not matter.
		for i, j := 0, len(cands)-1; i < j; i, j = i+1, j-1 {
			cands[i], cands[j] = cands[j], cands[i]
		}
		if again := mustPick(t, g, mp, c.Name, cands); !reflect.DeepEqual(again, got) {
			t.Errorf("%d units: pick depends on candidate order", tt.units)
		}
	}
	// Units of 25 records: at most two fit into 60.
	m := manyUnits(10, 25)
	g := mustGraph(t, m)
	c2, _ := categoryByName("level-same-shot")
	got := mustPick(t, g, mp, c2.Name, g.candidates(c2))
	if len(got) != 50 {
		t.Errorf("picked %d records; want 50", len(got))
	}
	// A unit over 60 records is never a candidate.
	g = mustGraph(t, manyUnits(1, 61))
	if cands := g.candidates(c2); len(cands) != 0 {
		t.Errorf("too large unit is a candidate")
	}
}

func TestAssetAndFileRules(t *testing.T) {
	t.Parallel()
	base := photo(1, "IMG_0001.HEIC", "2023-07-14T18:03:27+03:00", 1)
	if r := AssetRule(base); r != "" {
		t.Fatalf("AssetRule(base) = %q", r)
	}
	assetTests := []struct {
		rule   string
		mutate func(a *Asset)
	}{
		{RuleDateYear, func(a *Asset) { a.Date = "2031-01-01T00:00:00+00:00" }},
		{RuleDateYear, func(a *Asset) { a.Date = "1968-01-01T00:00:00+00:00" }},
		{RuleDateParse, func(a *Asset) { a.Date = "14.07.2023" }},
		{RuleYearSegment, func(a *Asset) { a.Albums = []AlbumRef{{FolderNames: []string{"2031"}, Title: "x"}} }},
		{RuleChar, func(a *Asset) { a.OriginalFilename = "me@example.com.jpg" }},
		{RuleChar, func(a *Asset) { a.Filename = `a\b.jpg` }},
		{RuleChar, func(a *Asset) { a.Albums = []AlbumRef{{Title: "55°45′N"}} }},
		{RuleChar, func(a *Asset) { a.Albums = []AlbumRef{{Title: "a\nb"}} }},
		{RuleLeadingMark, func(a *Asset) { a.OriginalFilename = "\u0301x.jpg" }},
		{RuleNonASCIIDigit, func(a *Asset) { a.OriginalFilename = "IMG_\u0661\u0662.jpg" }},
		{RuleRootPath, func(a *Asset) { a.Albums = []AlbumRef{{Title: "/Users/x"}} }},
		{RuleRootPath, func(a *Asset) { a.Albums = []AlbumRef{{FolderNames: []string{"~x"}, Title: "y"}} }},
		{RuleDrive, func(a *Asset) { a.Albums = []AlbumRef{{Title: "C:/Photos"}} }},
		{RuleDecimal, func(a *Asset) { a.Albums = []AlbumRef{{Title: "at 55.75393"}} }},
		{RuleMark, func(a *Asset) { a.OriginalFilename = "e\u0301.jpg" }},
		{RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_20301305_101112.jpg"
		}},
		{RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_20300105_101112.jpg"
		}},
		{RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_20210305_1011123.jpg"
		}},
		{"", func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_20210305_101112.jpg"
		}},
		{"", func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_20210305_101112345.MP.jpg"
		}},
		{RuleNegativeSize, func(a *Asset) { a.OriginalFilesize = -1 }},
		{RuleDecimal, func(a *Asset) { a.Albums = []AlbumRef{{Title: "at 55.75 37.62"}} }},
		{RuleCaseFold, func(a *Asset) { a.Albums = []AlbumRef{{Title: "İzmir"}} }},
		{RuleCaseFold, func(a *Asset) { a.OriginalFilename = "ΣΑΣ.jpg" }},
		{RuleCaseFold, func(a *Asset) { a.OriginalFilename = "σας.jpg" }},
		{RuleEpochGap, func(a *Asset) { a.Date = "1970-12-31T09:59:58+03:00" }},
		{"", func(a *Asset) { a.Date = "1970-12-31T09:59:57.999+03:00" }},
		{"", func(a *Asset) { a.Date = "1969-01-01T00:00:00+03:00" }},
		// The fixed date counts: PXL_ time 10:00:00 UTC + 3 h.
		{RuleEpochGap, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_19701231_100000.jpg"
		}},
		{RuleEpochGap, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_19701231_065958.jpg"
		}},
		{"", func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_19701231_065957.jpg"
		}},
		// The name date keeps its year class: a 1970 name date fixed into
		// 1971 would be kept in the name and shifted in the date.
		{RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_19701231_230000.jpg"
		}},
		// A name date the mapping does not keep (1968), fixed into 1969.
		{RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "PXL_19681231_230000.jpg"
		}},
	}
	for _, tt := range assetTests {
		a := base
		tt.mutate(&a)
		want := tt.rule
		if tt.rule == RuleMark && NormalizationAvailable {
			want = ""
		}
		if got := AssetRule(a); got != want {
			t.Errorf("AssetRule(%+v) = %q; want %q", a, got, want)
		}
	}
	// Rules that depend on Unicode normalization: a mark is not supported
	// without it; with it, a mark continues a letter run before PXL_ and
	// "i" + U+0307 is the Python lower case of "İ".
	gated := []struct {
		without, with string
		mutate        func(a *Asset)
	}{
		{RuleMark, RuleFix1970, func(a *Asset) {
			a.Date, a.OriginalFilename = "1970-01-01T00:00:00+00:00", "éPXL_20210305_101112.jpg"
		}},
		{RuleMark, RuleCaseFold, func(a *Asset) { a.Albums = []AlbumRef{{Title: "i̇zmir"}} }},
	}
	for _, tt := range gated {
		a := base
		tt.mutate(&a)
		want := tt.without
		if NormalizationAvailable {
			want = tt.with
		}
		if got := AssetRule(a); got != want {
			t.Errorf("AssetRule(%+v) = %q; want %q", a, got, want)
		}
	}
	fbase := cloudFile("2023/07-14 - A/IMG_0001.HEIC", "2023-07-14T18:03:27", 1)
	if r := FileRule(fbase); r != "" {
		t.Fatalf("FileRule(base) = %q", r)
	}
	fileTests := []struct {
		rule string
		path string
		date string
	}{
		{RuleYearSegment, "2031/x.jpg", ""},
		{RuleYearSegment, "1968/x.jpg", ""},
		{RuleEventDigits, "2023/123 A/x.jpg", ""},
		{RuleEventDigits, "2023/07-145/x.jpg", ""},
		{RuleDateYear, "2023/x.jpg", "2045-01-01T00:00:00"},
		{RuleDateParse, "2023/x.jpg", "2023:01:01 00:00:00"},
		{RuleChar, "2023/a@b/x.jpg", ""},
		{RuleDecimal, "2023/1.23456/x.jpg", ""},
		{RuleDecimal, "2023/a/at 18.03.27.png", ""},
		{RuleNonASCIIDigit, "２０２３/x.jpg", ""},
		{RuleDrive, "C:/x.jpg", ""},
		{"", "1970/01-01/x.jpg", "1970-01-01T00:00:00"},
		{RuleEpochGap, "1970/12-31/x.jpg", "1970-12-31T20:00:00"},
		{RuleEpochGap, "1970/12-31/x.jpg", "1970-12-31T09:59:58"},
		{"", "1970/12-31/x.jpg", "1970-12-31T09:59:57.999"},
		{"", "Archive/123 A/2031/x.jpg", ""},
	}
	for _, tt := range fileTests {
		f := fbase
		f.Path, f.DateTaken = tt.path, tt.date
		if got := FileRule(f); got != tt.rule {
			t.Errorf("FileRule(%q, %q) = %q; want %q", tt.path, tt.date, got, tt.rule)
		}
	}
	f := fbase
	f.Size = -5
	if got := FileRule(f); got != RuleNegativeSize {
		t.Errorf("FileRule(negative size) = %q", got)
	}
}

func TestBuild(t *testing.T) {
	t.Parallel()
	for _, cc := range categoryCases() {
		if cc.category == "nfd" && !NormalizationAvailable {
			continue
		}
		m := cc.yes.build()
		if c := CheckPartition(m); !c.Zero() {
			t.Fatalf("%s: test material: %s", cc.category, c)
		}
		res, err := Build(m, newTestMapper(t))
		if err != nil {
			t.Fatalf("%s: Build: %v", cc.category, err)
		}
		if len(res.Cases) != len(Categories) {
			t.Fatalf("%d case results; want %d", len(res.Cases), len(Categories))
		}
		found := false
		for _, cr := range res.Cases {
			if cr.Category == cc.category && cr.Case != nil {
				found = true
			}
			checkCase(t, cr)
		}
		if !found {
			t.Errorf("%s: no case", cc.category)
		}
		if res.Units.Total == 0 {
			t.Errorf("%s: units %+v", cc.category, res.Units)
		}
	}
	// The same material and salt give the same result.
	m := categoryCases()[0].yes.build()
	a, err1 := Build(m, newTestMapper(t))
	b, err2 := Build(categoryCases()[0].yes.build(), newTestMapper(t))
	if err1 != nil || err2 != nil || !reflect.DeepEqual(a, b) {
		t.Error("Build is not deterministic")
	}
	if a.Links[0] != 1 {
		t.Errorf("result = %+v", a)
	}
}

func checkCase(t *testing.T, cr CaseResult) {
	t.Helper()
	if cr.Case == nil {
		if cr.Records != 0 {
			t.Errorf("%s: empty case with %d records", cr.Category, cr.Records)
		}
		return
	}
	{
		c := cr.Case
		if c.Format != FormatVersion || c.Provenance.Category != cr.Category || c.Provenance.Kind != ProvenanceKind {
			t.Errorf("%s: header %+v", cr.Category, c.Provenance)
		}
		if cr.Records != len(c.ICloud)+len(c.Cloud) || cr.Records > 60 {
			t.Errorf("%s: %d records", cr.Category, cr.Records)
		}
		uuids, paths := map[string]bool{}, map[string]bool{}
		for _, a := range c.ICloud {
			uuids[a.UUID] = true
			if !strings.HasPrefix(a.UUID, "00000000-") || a.Albums == nil {
				t.Errorf("%s: asset %+v", cr.Category, a)
			}
		}
		for _, f := range c.Cloud {
			paths[f.Path] = true
		}
		pairs := 0
		for _, p := range c.Expected {
			if !uuids[p.UUID] || !paths[p.Path] {
				t.Errorf("%s: pair %+v outside the case", cr.Category, p)
			}
			pairs++
		}
		sum := 0
		for _, n := range cr.Levels {
			sum += n
		}
		if sum != pairs {
			t.Errorf("%s: levels %v, %d pairs", cr.Category, cr.Levels, pairs)
		}
	}
}

func TestBuildSkipsUnits(t *testing.T) {
	t.Parallel()
	bad := photo(1, "me@example.com.jpg", "2023-07-14T18:03:27+03:00", 1)
	tm := testMaterial{assets: []Asset{bad, photo(2, "IMG_0002.HEIC", "2023-07-14T18:03:27+03:00", 2)}}
	big := manyUnits(1, 61)
	m := tm.build()
	res, err := Build(m, newTestMapper(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Units != (UnitStats{Total: 2, Unsupported: 1}) || !reflect.DeepEqual(res.Rules, map[string]int{RuleChar: 1}) {
		t.Errorf("units = %+v, rules %v", res.Units, res.Rules)
	}
	res, err = Build(big, newTestMapper(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Units != (UnitStats{Total: 1, TooLarge: 1}) || len(res.Rules) != 0 {
		t.Errorf("units = %+v, rules %v", res.Units, res.Rules)
	}
	for _, cr := range res.Cases {
		if cr.Case != nil {
			t.Errorf("case %s from a too large unit", cr.Category)
		}
	}
}

// TestClustersRule: a letter run whose lower NFC form has another number of
// clusters (Hangul jamo compose) is not supported; without normalization
// the forms never differ.
func TestClustersRule(t *testing.T) {
	t.Parallel()
	jamo := string([]rune{0x1100, 0x1161})
	if !NormalizationAvailable {
		t.Skip("no Unicode normalization in this build")
	}
	if r := textRule(jamo + " x"); r != RuleClusters {
		t.Errorf("textRule(jamo) = %q; want %q", r, RuleClusters)
	}
}

// partialUnits is a material where an unsupported record shares its unit
// with supported ones: an unsupported asset with a linked file and a
// neighbor, and a supported asset whose linked file is not supported, next to
// a supported sidecar. Asset 2 alone is the only supported unit.
func partialUnits() testMaterial {
	d := "2023-07-14T18:03:27+03:00"
	bad := photo(1, "me@example.com.jpg", d, 1)
	good := photo(2, "IMG_0002.HEIC", "2023-07-15T10:00:00+03:00", 2)
	alone := photo(3, "IMG_0003.HEIC", "2023-07-16T10:00:00+03:00", 3)
	return testMaterial{
		assets: []Asset{bad, good, alone},
		files: []File{
			cloudFile("2023/a/IMG_0001.JPG", "2023-07-14T18:03:27", 1),
			cloudFile("2023/a/IMG_0001 (1).JPG", "", 1),
			cloudFile("2023/b/IMG_0002.HEIC", "2045-01-01T00:00:00", 2),
			cloudFile("2023/b/IMG_0002.xmp", "", 1),
		},
		links: Links{
			bad.UUID:  {{"2023/a/IMG_0001.JPG", LevelSameShot}},
			good.UUID: {{"2023/b/IMG_0002.HEIC", LevelExact}},
		},
	}
}

func TestUnsupportedRecordDropsItsUnit(t *testing.T) {
	t.Parallel()
	m := partialUnits().build()
	for _, i := range []int{3, 4, 6} {
		if r := FileRule(m.Files[i-3]); r != "" {
			t.Fatalf("record %d breaks %s: the test needs a supported partner", i, r)
		}
	}
	units, err := SupportedUnits(m)
	if err != nil || !reflect.DeepEqual(units, [][]int{{2}}) {
		t.Errorf("SupportedUnits = %v, %v", units, err)
	}
	res, err := Build(m, newTestMapper(t))
	if err != nil {
		t.Fatal(err)
	}
	if res.Units != (UnitStats{Total: 3, Unsupported: 2}) || !reflect.DeepEqual(res.Rules, map[string]int{RuleChar: 1, RuleDateYear: 1}) {
		t.Errorf("units = %+v, rules %v", res.Units, res.Rules)
	}
	cases := 0
	for _, cr := range res.Cases {
		if cr.Case == nil {
			continue
		}
		cases++
		if !reflect.DeepEqual(cr.Src, []int{2}) || len(cr.Case.ICloud) != 1 || len(cr.Case.Cloud) != 0 {
			t.Errorf("case %s holds records %v", cr.Category, cr.Src)
		}
	}
	if cases == 0 {
		t.Error("no case from the supported unit")
	}
}

func TestSupportedUnits(t *testing.T) {
	t.Parallel()
	bad := photo(1, "me@example.com.jpg", "2023-07-14T18:03:27+03:00", 1)
	tm := testMaterial{assets: []Asset{bad, photo(2, "IMG_0002.HEIC", "2023-07-14T18:03:27+03:00", 2)},
		files: []File{cloudFile("2023/a/x.txt", "", 1), cloudFile("2023/a/y.jpg", "", 1)}}
	m := tm.build()
	got, err := SupportedUnits(m)
	if err != nil || !reflect.DeepEqual(got, [][]int{{1}, {3}}) {
		t.Errorf("SupportedUnits = %v, %v", got, err)
	}
	s := m.Subset([]int{1, 3})
	if len(s.Assets) != 1 || len(s.Files) != 1 || !reflect.DeepEqual(s.Src, []int{1, 3}) || s.Files[0].Path != "2023/a/y.jpg" {
		t.Errorf("Subset = %+v", s)
	}
}

// TestBuildSkipsExhaustedUnits maps with a MAC of one output: the first unit
// takes it, every other unit runs out of space and is skipped, not fatal.
func TestBuildSkipsExhaustedUnits(t *testing.T) {
	t.Parallel()
	tm := testMaterial{assets: []Asset{
		photo(1, "IMG_0001.HEIC", "2023-07-14T18:03:27+03:00", 1),
		photo(2, "IMG_0002.HEIC", "2022-06-01T10:00:00+03:00", 2),
		photo(3, "IMG_0003.HEIC", "2021-05-02T11:00:00+03:00", 3),
	}}
	mp := newMapperMAC(constantMAC)
	mp.limit = 1000
	res, err := Build(tm.build(), mp)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if res.Units != (UnitStats{Total: 3, Exhausted: 2}) {
		t.Errorf("units = %+v", res.Units)
	}
	for _, cr := range res.Cases {
		if cr.Category == "unmatched-icloud" && (cr.Case == nil || cr.Records != 1) {
			t.Errorf("unmatched-icloud: %d records", cr.Records)
		}
	}
}

// TestBuildEpochGapIsUnsupported: an epoch date next to the shifted range
// drops its unit, so the epoch_gap relation never stops a run.
func TestBuildEpochGapIsUnsupported(t *testing.T) {
	t.Parallel()
	tm := testMaterial{files: []File{
		cloudFile("1971/01-01/IMG_0001.JPG", "1971-01-01T02:00:00", 1),
		cloudFile("1971/01-01/IMG_0001 (1).JPG", "1970-12-31T20:00:00", 2),
	}}
	res, err := Build(tm.build(), newTestMapper(t))
	if err != nil {
		t.Fatalf("Build = %v", err)
	}
	if res.Units.Unsupported != 1 || res.Rules[RuleEpochGap] != 1 {
		t.Errorf("units %+v, rules %v", res.Units, res.Rules)
	}
}

// TestBuildCaseFailsClosed: a case whose set breaks a relation is never
// built; the error holds the relation and its count only.
func TestBuildCaseFailsClosed(t *testing.T) {
	t.Parallel()
	tm := testMaterial{files: []File{
		cloudFile("1971/01-01/IMG_0001.JPG", "1971-01-01T02:00:00", 1),
		cloudFile("1971/01-01/IMG_0001 (1).JPG", "1970-12-31T20:00:00", 2),
	}}
	g, err := newGraph(tm.build())
	if err != nil {
		t.Fatal(err)
	}
	cr, err := g.buildCase(newTestMapper(t), "x", []int{0, 1})
	var re *RelationError
	if !errors.As(err, &re) || re.Category != "x" || !reflect.DeepEqual(re.Violations, []Violation{{RelEpochGap, 1}}) {
		t.Fatalf("buildCase = %v", err)
	}
	if cr.Case != nil {
		t.Error("a case with a broken relation")
	}
}

// TestEpochRuleBound: the last supported epoch date is just before the
// smallest one the epoch_gap relation rejects next to the first shifted
// date: the reference links a pair exactly 14 h 2 s apart.
func TestEpochRuleBound(t *testing.T) {
	t.Parallel()
	first := cloudFile("1971/01-01/IMG_0001.JPG", "1971-01-01T00:00:00", 1)
	for _, tt := range []struct {
		date string
		gap  bool
	}{
		{"1970-12-31T09:59:57.999", false},
		{"1970-12-31T09:59:58", true},
	} {
		f := cloudFile("1970/12-31/IMG_0002.JPG", tt.date, 2)
		if gap := epochGaps(nil, []File{f, first}) > 0; gap != tt.gap || (FileRule(f) == RuleEpochGap) != tt.gap {
			t.Errorf("%s: gap %v, rule %q", tt.date, gap, FileRule(f))
		}
	}
}

// TestRelationErrorMessage: a broken relation is reported with the relation
// names and counts only.
func TestRelationErrorMessage(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("build: %w", &RelationError{Category: "unmatched-cloud", Violations: []Violation{{RelEpochGap, 1}, {RelSize, 2}}})
	var re *RelationError
	if !errors.As(err, &re) || re.Error() != "case anon-unmatched-cloud: relations broken: epoch_gap=1 size=2" {
		t.Errorf("Error() = %q", err.Error())
	}
}

// TestBuildExpectedPairs checks the records and the expected pairs of every
// case against an independent construction from the material: the mapped
// source records in source order, and per asset its reference links in
// reference order with the mapped uuid and path, the level and the plan.
func TestBuildExpectedPairs(t *testing.T) {
	t.Parallel()
	live := photo(1, "IMG_0001.HEIC", "2023-07-14T18:03:27+03:00", 100)
	live.LivePhoto = true
	two := photo(2, "IMG_0002.HEIC", "2023-07-15T10:00:00+03:00", 200)
	rep := photo(3, "IMG_0003.HEIC", "2023-07-16T10:00:00+03:00", 300)
	keep := photo(4, "IMG_0004.JPG", "2023-07-17T10:00:00+03:00", 400)
	tm := testMaterial{
		assets: []Asset{live, two, rep, keep},
		files: []File{
			cloudFile("2023/07-14 - A/IMG_0001.HEIC", "2023-07-14T18:03:27", 100),
			cloudFile("2023/07-14 - A/IMG_0001.MOV", "2023-07-14T18:03:27", 10),
			cloudFile("2023/07-15 - B/IMG_0002.HEIC", "2023-07-15T10:00:00", 200),
			cloudFile("Albums/B/IMG_0002.HEIC", "2023-07-15T10:00:00", 200),
			cloudFile("2023/07-16/IMG_0003.JPG", "2023-07-16T10:00:00", 150),
			cloudFile("2023/07-17/IMG_0004.JPG", "2023-07-17T10:00:00", 900),
		},
		links: Links{
			live.UUID: {{"2023/07-14 - A/IMG_0001.HEIC", LevelExact}, {"2023/07-14 - A/IMG_0001.MOV", RefLiveComponent}},
			two.UUID:  {{"Albums/B/IMG_0002.HEIC", LevelExact}, {"2023/07-15 - B/IMG_0002.HEIC", LevelExact}},
			rep.UUID:  {{"2023/07-16/IMG_0003.JPG", LevelSameShot}},
			keep.UUID: {{"2023/07-17/IMG_0004.JPG", LevelSameShot}},
		},
		replace: [][2]string{{rep.UUID, "2023/07-16/IMG_0003.JPG"}},
		keep:    []string{"2023/07-17/IMG_0004.JPG"},
	}
	m := tm.build()
	mp := newTestMapper(t)
	res, err := Build(m, mp)
	if err != nil {
		t.Fatal(err)
	}
	plan := func(uuid, path string) string {
		if uuid == rep.UUID && path == tm.replace[0][1] {
			return PlanReplaceDecision
		}
		if path == tm.keep[0] {
			return PlanKeepCloudDecision
		}
		return ""
	}
	nA := len(m.Assets)
	byCategory := map[string]*Case{}
	for _, cr := range res.Cases {
		if cr.Case == nil {
			continue
		}
		byCategory[cr.Category] = cr.Case
		var icloud []Asset
		var cloud []File
		want := []Pair{}
		in := map[int]bool{}
		for _, r := range cr.Src {
			in[r] = true
		}
		for _, r := range cr.Src {
			if r >= nA {
				f, err := mp.File(m.Files[r-nA])
				if err != nil {
					t.Fatal(err)
				}
				cloud = append(cloud, f)
				continue
			}
			a := m.Assets[r]
			ma, err := mp.Asset(a)
			if err != nil {
				t.Fatal(err)
			}
			icloud = append(icloud, ma)
			for _, l := range m.Links[a.UUID] {
				level, _ := LevelOf(l.Kind)
				p, err := mp.Path(l.Path)
				if err != nil {
					t.Fatal(err)
				}
				want = append(want, Pair{UUID: ma.UUID, Path: p, Level: level, Plan: plan(a.UUID, l.Path)})
				found := false
				for j, f := range m.Files {
					if f.Path == l.Path && in[nA+j] {
						found = true
					}
				}
				if !found {
					t.Errorf("%s: linked file of an asset is outside the case", cr.Category)
				}
			}
		}
		if !reflect.DeepEqual(cr.Case.ICloud, icloud) || !reflect.DeepEqual(cr.Case.Cloud, cloud) {
			t.Errorf("%s: records differ from the mapped source records", cr.Category)
		}
		if !reflect.DeepEqual(cr.Case.Expected, want) {
			t.Errorf("%s: expected = %+v; want %+v", cr.Category, cr.Case.Expected, want)
		}
	}
	// The categories of this material exist and hold what they promise.
	checks := map[string]func(c *Case) bool{
		"level-pair-component": func(c *Case) bool { return len(c.Expected) == 2 && c.Expected[1].Level == LevelPairComponent },
		"plan-replace":         func(c *Case) bool { return len(c.Expected) == 1 && c.Expected[0].Plan == PlanReplaceDecision },
		"plan-keep-cloud":      func(c *Case) bool { return len(c.Expected) == 1 && c.Expected[0].Plan == PlanKeepCloudDecision },
		"many-folders": func(c *Case) bool {
			// Pairs follow the reference order, records the source order.
			return len(c.Expected) == 2 && len(c.Cloud) == 2 && c.Expected[0].Path == c.Cloud[1].Path && c.Expected[1].Path == c.Cloud[0].Path
		},
	}
	for name, ok := range checks {
		if c := byCategory[name]; c == nil || !ok(c) {
			t.Errorf("%s: case %+v", name, c)
		}
	}
}
