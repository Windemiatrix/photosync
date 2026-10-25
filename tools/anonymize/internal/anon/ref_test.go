package anon

import (
	"testing"
	"time"
)

func TestSplitext(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, stem, ext string }{
		{"IMG_0001.HEIC", "IMG_0001", ".HEIC"},
		{".hidden", ".hidden", ""},
		{"a.b.c", "a.b", ".c"},
		{"...x", "...x", ""},
		{"..x.y", "..x", ".y"},
		{"noext", "noext", ""},
		{"a.", "a", "."},
		{"dir.d/file", "dir.d/file", ""},
		{"dir/.hidden", "dir/.hidden", ""},
		{"dir/a.jpg", "dir/a", ".jpg"},
		{"", "", ""},
	}
	for _, tt := range tests {
		stem, ext := Splitext(tt.in)
		if stem != tt.stem || ext != tt.ext {
			t.Errorf("Splitext(%q) = %q, %q; want %q, %q", tt.in, stem, ext, tt.stem, tt.ext)
		}
	}
}

func TestFolderInfoOf(t *testing.T) {
	t.Parallel()
	tests := []struct {
		path string
		want FolderInfo
	}{
		{"IMG_0001.HEIC", FolderInfo{}},
		{"2024/IMG_0001.HEIC", FolderInfo{Folder: "2024", Year: "2024"}},
		{"2024/05-01 - Holidays/IMG_0001.HEIC", FolderInfo{Folder: "2024/05-01 - Holidays", Year: "2024", Event: "05-01 - Holidays", EventMonth: "05", EventDay: "01"}},
		{"2024/06 - Trip/a/b.jpg", FolderInfo{Folder: "2024/06 - Trip/a", Year: "2024", Event: "06 - Trip", EventMonth: "06"}},
		{"2024/08-07/x.jpg", FolderInfo{Folder: "2024/08-07", Year: "2024", Event: "08-07", EventMonth: "08", EventDay: "07"}},
		{"2024/Trip/x.jpg", FolderInfo{Folder: "2024/Trip", Year: "2024", Event: "Trip"}},
		{"Albums/2024/x.jpg", FolderInfo{Folder: "Albums/2024"}},
		{"202/x.jpg", FolderInfo{Folder: "202"}},
		// Python's \d matches every decimal digit.
		{"２０２３/０７-１４ - A/x.jpg", FolderInfo{Folder: "２０２３/０７-１４ - A", Year: "２０２３", Event: "０７-１４ - A", EventMonth: "０７", EventDay: "１４"}},
		{"2023/١٢ A/x.jpg", FolderInfo{Folder: "2023/١٢ A", Year: "2023", Event: "١٢ A", EventMonth: "١٢"}},
	}
	for _, tt := range tests {
		if got := FolderInfoOf(tt.path); got != tt.want {
			t.Errorf("FolderInfoOf(%q) = %+v; want %+v", tt.path, got, tt.want)
		}
	}
}

func TestKindOf(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"a.HEIC": KindPhotosOK, "a.jpg": KindPhotosOK, "a.DNG": KindPhotosOK, "a.mov": KindPhotosOK,
		"a.AVI": KindConvert, "a.mts": KindConvert, "a.xmp": KindSidecar, "a.AAE": KindSidecar,
		"a.txt": KindOther, "noext": KindOther, "a.webp": KindOther,
	}
	for name, want := range tests {
		if got := KindOf(name); got != want {
			t.Errorf("KindOf(%q) = %q; want %q", name, got, want)
		}
	}
}

func TestParseDT(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		ok   bool
		want time.Time
	}{
		{"2023-07-14T18:03:27.455000+03:00", true, time.Date(2023, 7, 14, 18, 3, 27, 455000000, time.UTC)},
		{"2023-07-14T18:03:27+03:00", true, time.Date(2023, 7, 14, 18, 3, 27, 0, time.UTC)},
		{"2023-07-14T18:03:27Z", true, time.Date(2023, 7, 14, 18, 3, 27, 0, time.UTC)},
		{"2023-07-14T18:03:27.455", true, time.Date(2023, 7, 14, 18, 3, 27, 455000000, time.UTC)},
		{"2023-07-14T18:03:27", true, time.Date(2023, 7, 14, 18, 3, 27, 0, time.UTC)},
		{"", false, time.Time{}},
		{"garbage", false, time.Time{}},
		{"2023-13-14T18:03:27", false, time.Time{}},
		{"2023:07:14 18:03:27", false, time.Time{}},
	}
	for _, tt := range tests {
		got, ok := ParseDT(tt.in)
		if ok != tt.ok || !got.Equal(tt.want) {
			t.Errorf("ParseDT(%q) = %v, %v; want %v, %v", tt.in, got, ok, tt.want, tt.ok)
		}
	}
}

func TestFix1970(t *testing.T) {
	t.Parallel()
	d1970 := time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		dt       time.Time
		tz       int
		want     time.Time
		fixed    bool
		invalid  bool
		testName string
	}{
		{"PXL_20210305_101112345.jpg", d1970, 3600, time.Date(2021, 3, 5, 11, 11, 12, 0, time.UTC), true, false, "with tzoffset"},
		{"PXL_20210305_101112345.jpg", d1970, 0, time.Date(2021, 3, 5, 10, 11, 12, 0, time.UTC), true, false, "utc"},
		{"PXL_20210305_101112345.jpg", time.Date(2021, 3, 5, 0, 0, 0, 0, time.UTC), 0, time.Time{}, false, false, "not 1970"},
		{"IMG_0001.jpg", d1970, 0, time.Time{}, false, false, "no PXL"},
		{"PXL_20211305_101112.jpg", d1970, 0, time.Time{}, false, true, "impossible date"},
	}
	for _, tt := range tests {
		got, fixed, invalid := Fix1970(tt.name, tt.dt, tt.tz)
		if fixed != tt.fixed || invalid != tt.invalid || !got.Equal(tt.want) {
			t.Errorf("%s: Fix1970 = %v, %v, %v; want %v, %v, %v", tt.testName, got, fixed, invalid, tt.want, tt.fixed, tt.invalid)
		}
	}
}

func TestEventToTitle(t *testing.T) {
	t.Parallel()
	tests := map[string]string{
		"01-03 - Party": "01/03 Party",
		"06 - Trip":     "06 Trip",
		"08-07":         "08/07",
		"08-07 Walk":    "08/07 Walk",
		"08 Walk":       "08 Walk",
		"Walk":          "Walk",
		"123 Walk":      "123 Walk",
		"07-145 x":      "07 145 x",
		"06-Trip":       "06 Trip",
	}
	for in, want := range tests {
		if got := EventToTitle(in); got != want {
			t.Errorf("EventToTitle(%q) = %q; want %q", in, got, want)
		}
	}
}

func TestFolderToAlbum(t *testing.T) {
	t.Parallel()
	tests := []struct {
		folder string
		ok     bool
		want   AlbumKey
	}{
		{"", false, AlbumKey{}},
		{"2024", false, AlbumKey{}},
		{"2024/05-01 - Holidays", true, AlbumKey{Folders: []string{"2024"}, Title: "05/01 Holidays"}},
		{"2024/05-01 - Holidays/Beach", true, AlbumKey{Folders: []string{"2024"}, Title: "05/01 Holidays / Beach"}},
		{"Albums/Trips/Sea", true, AlbumKey{Folders: []string{}, Title: "Albums / Trips / Sea"}},
		{"Unsorted", true, AlbumKey{Folders: []string{}, Title: "Unsorted"}},
	}
	for _, tt := range tests {
		got, ok := FolderToAlbum(tt.folder)
		if ok != tt.ok || got.String() != tt.want.String() {
			t.Errorf("FolderToAlbum(%q) = %+v, %v; want %+v, %v", tt.folder, got, ok, tt.want, tt.ok)
		}
	}
}

func TestNeighborKeyAndSuffix(t *testing.T) {
	t.Parallel()
	if SuffixN("IMG_0001 (1)") != " (1)" || SuffixN("IMG_0001") != "" || SuffixN("IMG (12)x") != "" {
		t.Error("SuffixN")
	}
	same := [][2]string{
		{"2024/a/IMG_0001.HEIC", "2024/a/img_0001 (1).MOV"},
		{"2024/a/IMG_0001.HEIC", "2024/a/IMG_0001.xmp"},
	}
	for _, p := range same {
		if NeighborKey(p[0]) != NeighborKey(p[1]) {
			t.Errorf("NeighborKey(%q) != NeighborKey(%q)", p[0], p[1])
		}
	}
	diff := [][2]string{
		{"2024/a/IMG_0001.HEIC", "2024/b/IMG_0001.HEIC"},
		{"2024/a/IMG_0001.HEIC", "2024/a/IMG_0002.HEIC"},
	}
	for _, p := range diff {
		if NeighborKey(p[0]) == NeighborKey(p[1]) {
			t.Errorf("NeighborKey(%q) == NeighborKey(%q)", p[0], p[1])
		}
	}
}

func TestSplitRuns(t *testing.T) {
	t.Parallel()
	got := splitRuns("IMG_0001 (1)Ёлка")
	want := []textRun{{runLetters, "IMG"}, {runOther, "_"}, {runDigits, "0001"}, {runOther, " ("}, {runDigits, "1"}, {runOther, ")"}, {runLetters, "Ёлка"}}
	if len(got) != len(want) {
		t.Fatalf("splitRuns = %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("run %d = %v; want %v", i, got[i], want[i])
		}
	}
	got = splitRuns("éx ́")
	if len(got) != 2 || got[0] != (textRun{runLetters, "éx"}) || got[1] != (textRun{runOther, " ́"}) {
		t.Errorf("marks: %v", got)
	}
}

func TestFormatDictionary(t *testing.T) {
	t.Parallel()
	for _, w := range []string{"photos", "ok", "real", "anon", "level", "same", "time", "size", "pair", "component", "keep", "cloud"} {
		if !AllowedWord(w) {
			t.Errorf("AllowedWord(%q) = false", w)
		}
	}
	for _, w := range []string{"heic", "cr", "gp", "ts", "img", "edited"} {
		if !AllowedWord(w) {
			t.Errorf("AllowedWord(%q) = false", w)
		}
	}
	if AllowedWord("holiday") {
		t.Error("AllowedWord(holiday) = true")
	}
	if len(formatSet) == 0 {
		t.Error("empty format dictionary")
	}
}
