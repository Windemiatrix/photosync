package anon

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

func relationSet() *Set {
	a0 := photo(1, "IMG_4821.HEIC", "2023-07-14T18:03:27.455000+03:00", 2734118)
	a0.LivePhoto = true
	a0.Albums = []AlbumRef{{FolderNames: []string{"2023"}, Title: "07/14 Мырдон"}}
	a1 := photo(2, "PXL_20210305_101112345.jpg", "1970-01-01T03:00:00+03:00", 1000)
	a1.Burst, a1.BurstPhotos = true, []string{uuidN(9)}
	a2 := photo(3, "IMG_0001.HEIC", "2019-12-31T23:59:59+03:00", 5000)
	a2.Albums = []AlbumRef{{FolderNames: []string{"2019"}, Title: "12/31 НГ"}, {FolderNames: []string{}, Title: "Путешествия / Море"}}
	a3 := photo(4, "IMG_0001.JPG", "2019-12-31T23:59:59.5+03:00", 4000)
	burstOf(&a2, &a3)
	a3.Albums = []AlbumRef{{FolderNames: []string{}, Title: "Семья / Лето"}, {FolderNames: []string{}, Title: "Family"}}
	f0 := cloudFile("2023/07-14 - Мырдон/IMG_4821.HEIC", "2023-07-14T18:03:27.455", 2734118)
	f0.ContentID, f0.LivePair, f0.SHA1 = "C1", LivePairCID, "aa11"
	f1 := cloudFile("2023/07-14 - Мырдон/IMG_4821.MOV", "2023-07-14T18:03:27", 3000)
	f1.ContentID, f1.LivePair = "C1", LivePairCID
	f3 := cloudFile("2019/12-30/img_0001.jpg", "2019-12-30T10:00:00", 4001)
	f3.SHA1 = "bb22"
	f7 := cloudFile("2022/06 - Trip/Закат (2).jpg", "2022-06-01T12:00:00", 8000)
	f7.SHA1 = "bb22"
	files := []File{
		f0, f1,
		cloudFile("2019/12-31 - НГ/IMG_0001 (1).JPG", "", 4000),
		f3,
		cloudFile("2021/03-05/PXL_20210305_101112345.jpg", "2021-03-05T13:11:12", 1000),
		cloudFile("Путешествия/Море/Ёлка.mov", "1970-01-02T00:00:00", 7000),
		cloudFile("2022/06 - Trip/Notes.xmp", "", 10),
		f7,
		cloudFile("2022/06 - Trip/IMG_0007_edited.JPG", "2022-06-02T12:00:00", 8100),
		cloudFile("2022/06 - Trip/d41d8cd98f00b204e9800998ecf8427e.jpg", "", 8200),
	}
	s := &Set{Assets: []Asset{a0, a1, a2, a3}, Files: files}
	for i := 0; i < len(s.Assets)+len(s.Files); i++ {
		s.Src = append(s.Src, i*3)
	}
	return s
}

func cloneSet(s *Set) *Set {
	c := &Set{Assets: append([]Asset(nil), s.Assets...), Files: append([]File(nil), s.Files...), Src: append([]int(nil), s.Src...)}
	for i := range c.Assets {
		c.Assets[i].Albums = append([]AlbumRef(nil), c.Assets[i].Albums...)
	}
	return c
}

func relationNames(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		if v.Count <= 0 {
			out = append(out, "bad count "+v.Relation)
		}
		out = append(out, v.Relation)
	}
	sort.Strings(out)
	return out
}

func replaceStemDigits(p, digits string) string {
	dir, base := FolderOf(p), BaseName(p)
	stem, ext := Splitext(base)
	i := strings.LastIndex(stem, "_")
	return dir + "/" + stem[:i+1] + digits + ext
}

func TestCheckRelationsCorrectImage(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	orig := relationSet()
	img, err := m.Set(orig)
	if err != nil {
		t.Fatal(err)
	}
	if vs := CheckRelations(orig, img, m.YearShift()); len(vs) != 0 {
		t.Errorf("CheckRelations = %v", vs)
	}
	if vs := CheckRelations(orig, img, m.YearShift()+4); len(vs) == 0 {
		t.Error("a wrong shift is not reported")
	}
}

// A changed stem of a file next to its pair breaks both the name and the
// folder partitions by nature.
func TestCheckRelationsBreakers(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	shift := m.YearShift()
	y := func(year int) string { return strconv.Itoa(year + shift) }
	tests := []struct {
		name string
		brk  func(orig, img *Set)
		want []string
	}{
		{"extension case", func(_, img *Set) { img.Files[6].Path = strings.TrimSuffix(img.Files[6].Path, ".xmp") + ".XMP" }, []string{RelFields}},
		{"live flag", func(_, img *Set) { img.Assets[0].LivePhoto = false }, []string{RelFields}},
		// The burst group derives from the flag: it is lost with it.
		{"burst flag", func(_, img *Set) { img.Assets[1].Burst = false }, []string{RelFields, RelIDs}},
		{"has_raw flag", func(_, img *Set) { img.Assets[0].HasRaw = true }, []string{RelFields}},
		{"ismovie flag", func(_, img *Set) { img.Assets[2].IsMovie = true }, []string{RelFields}},
		{"live_pair dropped", func(_, img *Set) { img.Files[0].LivePair = "" }, []string{RelFields}},
		{"kind", func(_, img *Set) { img.Files[2].Kind = KindConvert }, []string{RelFields}},
		{"frame size", func(_, img *Set) { img.Files[2].Width = 1 }, []string{RelFields}},
		{"date one day off", func(_, img *Set) {
			img.Files[4].DateTaken = strings.Replace(img.Files[4].DateTaken, "-03-05T", "-03-06T", 1)
		}, []string{RelDate}},
		{"1970 date shifted", func(_, img *Set) { img.Files[5].DateTaken = y(1970) + img.Files[5].DateTaken[4:] }, []string{RelDate}},
		{"fraction dropped", func(_, img *Set) { img.Files[0].DateTaken = strings.TrimSuffix(img.Files[0].DateTaken, ".455") }, []string{RelDate}},
		{"date dropped", func(_, img *Set) { img.Assets[3].Date = "" }, []string{RelDate}},
		{"event month", func(_, img *Set) {
			img.Files[4].Path = strings.Replace(img.Files[4].Path, "/03-05/", "/04-05/", 1)
		}, []string{RelFolderInfo}},
		{"suffix dropped", func(_, img *Set) { img.Files[7].Path = strings.Replace(img.Files[7].Path, " (2)", "", 1) }, []string{RelStrings}},
		{"NFD became NFC", func(orig, img *Set) {
			orig.Files[5].Path = nfd(orig.Files[5].Path)
			img.Files[5].Path = nfc(img.Files[5].Path)
		}, []string{RelStrings}},
		{"two UUIDs gave one", func(_, img *Set) { img.Assets[1].UUID = img.Assets[0].UUID }, []string{RelIDs}},
		{"content id split", func(_, img *Set) { img.Files[1].ContentID = "00000000ffffffffffffffffffffffff" }, []string{RelIDs}},
		{"same lower name, different images", func(_, img *Set) { img.Files[3].Path = replaceStemDigits(img.Files[3].Path, "9999") }, []string{RelNames}},
		{"MOV stem changed", func(_, img *Set) { img.Files[1].Path = replaceStemDigits(img.Files[1].Path, "0002") }, []string{RelFolders, RelNames}},
		// The folder partition alone: one more space after " - " keeps the
		// event month and day and the album of the folder.
		{"MOV folder spacing", func(_, img *Set) {
			img.Files[1].Path = strings.Replace(img.Files[1].Path, "07-14 - ", "07-14 -  ", 1)
		}, []string{RelFolders}},
		{"1970 fix moved", func(_, img *Set) {
			img.Assets[1].OriginalFilename = strings.Replace(img.Assets[1].OriginalFilename, "0305_", "0306_", 1)
			img.Files[4].Path = FolderOf(img.Files[4].Path) + "/" + strings.Replace(BaseName(img.Files[4].Path), "0305_", "0306_", 1)
		}, []string{RelDate}},
		{"event day", func(_, img *Set) {
			img.Files[4].Path = strings.Replace(img.Files[4].Path, "/03-05/", "/03-06/", 1)
		}, []string{RelFolderInfo}},
		{"edited dropped", func(_, img *Set) { img.Files[8].Path = strings.Replace(img.Files[8].Path, "_edited", "", 1) }, []string{RelStrings}},
		// The name key without " (N)" alone: the copy keeps its tail and is
		// the only file of its folder.
		{"name key without (N) split", func(_, img *Set) {
			base := BaseName(img.Files[2].Path)
			img.Files[2].Path = FolderOf(img.Files[2].Path) + "/IMG_9999" + base[len("IMG_0000"):]
		}, []string{RelNames}},
		// Relation 4 term by term, on albums no folder holds and on file
		// names whose keys are unique, so no partition breaks with them.
		{"slash removed from a title", func(_, img *Set) {
			img.Assets[3].Albums[0].Title = strings.ReplaceAll(img.Assets[3].Albums[0].Title, "/", "-")
		}, []string{RelStrings}},
		{"ASCII title made empty", func(_, img *Set) { img.Assets[3].Albums[1].Title = "" }, []string{RelStrings}},
		{"non-ASCII name became ASCII", func(_, img *Set) {
			img.Files[7].Path = FolderOf(img.Files[7].Path) + "/Qwert (2).jpg"
		}, []string{RelStrings}},
		{"ASCII name became non-ASCII", func(_, img *Set) {
			img.Files[6].Path = FolderOf(img.Files[6].Path) + "/Жqrst.xmp"
		}, []string{RelStrings}},
		{"tzoffset changed", func(_, img *Set) { img.Assets[0].TZOffset = 0 }, []string{RelFields}},
		{"two burst groups gave one", func(_, img *Set) { img.Assets[1].BurstPhotos = []string{img.Assets[2].UUID} }, []string{RelIDs}},
		{"burst_photos element changed", func(_, img *Set) {
			img.Assets[2].BurstPhotos = []string{"00000000-0000-8000-8000-000000000000"}
		}, []string{RelIDs}},
		{"burst_photos element dropped", func(_, img *Set) { img.Assets[1].BurstPhotos = []string{} }, []string{RelFields}},
		{"burst_key flag", func(_, img *Set) { img.Assets[2].BurstKeyPhoto = false }, []string{RelFields}},
		{"two SHA-1 gave one", func(_, img *Set) { img.Files[0].SHA1 = img.Files[3].SHA1 }, []string{RelIDs}},
		{"SHA-1 duplicate split", func(_, img *Set) {
			img.Files[7].SHA1 = "00000000" + strings.Repeat("f", 32)
		}, []string{RelIDs}},
		{"album and folder split", func(_, img *Set) { img.Assets[0].Albums[0].Title += "x" }, []string{RelAlbums}},
		{"two sizes swapped", func(_, img *Set) { img.Files[2].Size, img.Files[3].Size = img.Files[3].Size, img.Files[2].Size }, []string{RelSize}},
		{"epoch gap", func(orig, img *Set) {
			orig.Files[5].DateTaken, img.Files[5].DateTaken = "1970-12-31T20:00:00", "1970-12-31T20:00:00"
			orig.Files[4].DateTaken, img.Files[4].DateTaken = "1971-01-01T02:00:00", y(1971)+"-01-01T02:00:00"
		}, []string{RelEpochGap}},
		{"file missing", func(_, img *Set) { img.Files = img.Files[:len(img.Files)-1] }, []string{RelShape}},
		{"asset missing", func(_, img *Set) { img.Assets = img.Assets[1:] }, []string{RelShape}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			orig := relationSet()
			img, err := newTestMapper(t).Set(orig)
			if err != nil {
				t.Fatal(err)
			}
			img = cloneSet(img)
			tt.brk(orig, img)
			got := relationNames(CheckRelations(orig, img, shift))
			want := append([]string(nil), tt.want...)
			sort.Strings(want)
			if !reflect.DeepEqual(got, want) {
				t.Errorf("CheckRelations = %v; want %v", got, want)
			}
		})
	}
}

func TestCheckRelationsEpochGapInOriginal(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	orig := relationSet()
	orig.Files[5].DateTaken = "1970-12-31T23:00:00"
	orig.Files[4].DateTaken = "1971-01-01T00:30:00"
	img, err := m.Set(orig)
	if err != nil {
		t.Fatal(err)
	}
	got := relationNames(CheckRelations(orig, img, m.YearShift()))
	if !reflect.DeepEqual(got, []string{RelEpochGap}) {
		t.Errorf("CheckRelations = %v", got)
	}
}
