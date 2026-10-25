package anon

import (
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	liveAsset = iota
	pixel1970Asset
	burstKeyAsset
	burstMemberAsset
)

const (
	liveStillFile = iota
	liveVideoFile
	suffixCopyFile
	lowerNameFile
	pixelFile
	epochVideoFile
	xmpFile
	nonASCIICopyFile
	editedFile
)

func relationSet() *Set {
	a0 := photo(1, "IMG_4821.HEIC", "2023-07-14T18:03:27.455000+03:00", 2734118)
	a0.LivePhoto = true
	a0.Albums = []AlbumRef{{FolderNames: []string{"2023"}, Title: "07/14 Мырдон"}}
	a1 := photo(2, "PXL_20210305_101112345.jpg", "1970-01-01T03:00:00+03:00", 1000)
	// A burst whose other frame is not in the set.
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
		{"extension case", func(_, img *Set) {
			img.Files[xmpFile].Path = strings.TrimSuffix(img.Files[xmpFile].Path, ".xmp") + ".XMP"
		}, []string{RelFields}},
		{"live flag", func(_, img *Set) { img.Assets[liveAsset].LivePhoto = false }, []string{RelFields}},
		// The burst group derives from the flag: it is lost with it.
		{"burst flag", func(_, img *Set) { img.Assets[pixel1970Asset].Burst = false }, []string{RelFields, RelIDs}},
		{"has_raw flag", func(_, img *Set) { img.Assets[liveAsset].HasRaw = true }, []string{RelFields}},
		{"ismovie flag", func(_, img *Set) { img.Assets[burstKeyAsset].IsMovie = true }, []string{RelFields}},
		{"live_pair dropped", func(_, img *Set) { img.Files[liveStillFile].LivePair = "" }, []string{RelFields}},
		{"kind", func(_, img *Set) { img.Files[suffixCopyFile].Kind = KindConvert }, []string{RelFields}},
		{"frame size", func(_, img *Set) { img.Files[suffixCopyFile].Width = 1 }, []string{RelFields}},
		{"date one day off", func(_, img *Set) {
			img.Files[pixelFile].DateTaken = strings.Replace(img.Files[pixelFile].DateTaken, "-03-05T", "-03-06T", 1)
		}, []string{RelDate}},
		{"1970 date shifted", func(_, img *Set) {
			img.Files[epochVideoFile].DateTaken = y(1970) + img.Files[epochVideoFile].DateTaken[4:]
		}, []string{RelDate}},
		{"fraction dropped", func(_, img *Set) {
			img.Files[liveStillFile].DateTaken = strings.TrimSuffix(img.Files[liveStillFile].DateTaken, ".455")
		}, []string{RelDate}},
		{"date dropped", func(_, img *Set) { img.Assets[burstMemberAsset].Date = "" }, []string{RelDate}},
		{"event month", func(_, img *Set) {
			img.Files[pixelFile].Path = strings.Replace(img.Files[pixelFile].Path, "/03-05/", "/04-05/", 1)
		}, []string{RelFolderInfo}},
		{"suffix dropped", func(_, img *Set) {
			img.Files[nonASCIICopyFile].Path = strings.Replace(img.Files[nonASCIICopyFile].Path, " (2)", "", 1)
		}, []string{RelStrings}},
		{"NFD became NFC", func(orig, img *Set) {
			orig.Files[epochVideoFile].Path = nfd(orig.Files[epochVideoFile].Path)
			img.Files[epochVideoFile].Path = nfc(img.Files[epochVideoFile].Path)
		}, []string{RelStrings}},
		{"two UUIDs gave one", func(_, img *Set) { img.Assets[pixel1970Asset].UUID = img.Assets[liveAsset].UUID }, []string{RelIDs}},
		{"content id split", func(_, img *Set) {
			img.Files[liveVideoFile].ContentID = "00000000ffffffffffffffffffffffff"
		}, []string{RelIDs}},
		{"same lower name, different images", func(_, img *Set) {
			img.Files[lowerNameFile].Path = replaceStemDigits(img.Files[lowerNameFile].Path, "9999")
		}, []string{RelNames}},
		// A changed stem of a file next to its pair breaks both the name and the
		// folder partitions by nature.
		{"MOV stem changed", func(_, img *Set) {
			img.Files[liveVideoFile].Path = replaceStemDigits(img.Files[liveVideoFile].Path, "0002")
		}, []string{RelFolders, RelNames}},
		// The folder partition alone: one more space after " - " keeps the
		// event month and day and the album of the folder.
		{"MOV folder spacing", func(_, img *Set) {
			img.Files[liveVideoFile].Path = strings.Replace(img.Files[liveVideoFile].Path, "07-14 - ", "07-14 -  ", 1)
		}, []string{RelFolders}},
		{"1970 fix moved", func(_, img *Set) {
			img.Assets[pixel1970Asset].OriginalFilename = strings.Replace(img.Assets[pixel1970Asset].OriginalFilename, "0305_", "0306_", 1)
			img.Files[pixelFile].Path = FolderOf(img.Files[pixelFile].Path) + "/" + strings.Replace(BaseName(img.Files[pixelFile].Path), "0305_", "0306_", 1)
		}, []string{RelDate}},
		{"event day", func(_, img *Set) {
			img.Files[pixelFile].Path = strings.Replace(img.Files[pixelFile].Path, "/03-05/", "/03-06/", 1)
		}, []string{RelFolderInfo}},
		{"edited dropped", func(_, img *Set) {
			img.Files[editedFile].Path = strings.Replace(img.Files[editedFile].Path, "_edited", "", 1)
		}, []string{RelStrings}},
		// The name key without " (N)" alone: the copy keeps its tail and is
		// the only file of its folder.
		{"name key without (N) split", func(_, img *Set) {
			base := BaseName(img.Files[suffixCopyFile].Path)
			img.Files[suffixCopyFile].Path = FolderOf(img.Files[suffixCopyFile].Path) + "/IMG_9999" + base[len("IMG_0000"):]
		}, []string{RelNames}},
		// Relation 4 term by term, on albums no folder holds and on file
		// names whose keys are unique, so no partition breaks with them.
		{"slash removed from a title", func(_, img *Set) {
			img.Assets[burstMemberAsset].Albums[0].Title = strings.ReplaceAll(img.Assets[burstMemberAsset].Albums[0].Title, "/", "-")
		}, []string{RelStrings}},
		{"ASCII title made empty", func(_, img *Set) { img.Assets[burstMemberAsset].Albums[1].Title = "" }, []string{RelStrings}},
		{"non-ASCII name became ASCII", func(_, img *Set) {
			img.Files[nonASCIICopyFile].Path = FolderOf(img.Files[nonASCIICopyFile].Path) + "/Qwert (2).jpg"
		}, []string{RelStrings}},
		{"ASCII name became non-ASCII", func(_, img *Set) {
			img.Files[xmpFile].Path = FolderOf(img.Files[xmpFile].Path) + "/Жqrst.xmp"
		}, []string{RelStrings}},
		{"tzoffset changed", func(_, img *Set) { img.Assets[liveAsset].TZOffset = 0 }, []string{RelFields}},
		{"two burst groups gave one", func(_, img *Set) {
			img.Assets[pixel1970Asset].BurstPhotos = []string{img.Assets[burstKeyAsset].UUID}
		}, []string{RelIDs}},
		{"burst_photos element changed", func(_, img *Set) {
			img.Assets[burstKeyAsset].BurstPhotos = []string{"00000000-0000-8000-8000-000000000000"}
		}, []string{RelIDs}},
		{"burst_photos element dropped", func(_, img *Set) { img.Assets[pixel1970Asset].BurstPhotos = []string{} }, []string{RelFields}},
		{"burst_key flag", func(_, img *Set) { img.Assets[burstKeyAsset].BurstKeyPhoto = false }, []string{RelFields}},
		{"two SHA-1 gave one", func(_, img *Set) { img.Files[liveStillFile].SHA1 = img.Files[lowerNameFile].SHA1 }, []string{RelIDs}},
		{"SHA-1 duplicate split", func(_, img *Set) {
			img.Files[nonASCIICopyFile].SHA1 = "00000000" + strings.Repeat("f", 32)
		}, []string{RelIDs}},
		{"album and folder split", func(_, img *Set) { img.Assets[liveAsset].Albums[0].Title += "x" }, []string{RelAlbums}},
		{"two sizes swapped", func(_, img *Set) {
			img.Files[suffixCopyFile].Size, img.Files[lowerNameFile].Size = img.Files[lowerNameFile].Size, img.Files[suffixCopyFile].Size
		}, []string{RelSize}},
		{"epoch gap", func(orig, img *Set) {
			orig.Files[epochVideoFile].DateTaken, img.Files[epochVideoFile].DateTaken = "1970-12-31T20:00:00", "1970-12-31T20:00:00"
			orig.Files[pixelFile].DateTaken, img.Files[pixelFile].DateTaken = "1971-01-01T02:00:00", y(1971)+"-01-01T02:00:00"
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
	orig.Files[epochVideoFile].DateTaken = "1970-12-31T23:00:00"
	orig.Files[pixelFile].DateTaken = "1971-01-01T00:30:00"
	img, err := m.Set(orig)
	if err != nil {
		t.Fatal(err)
	}
	got := relationNames(CheckRelations(orig, img, m.YearShift()))
	if !reflect.DeepEqual(got, []string{RelEpochGap}) {
		t.Errorf("CheckRelations = %v", got)
	}
}
