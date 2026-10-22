package anon

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNewMapperSaltLength(t *testing.T) {
	t.Parallel()
	for _, n := range []int{0, 16, 31, 33, 64} {
		if _, err := NewMapper(make([]byte, n)); err == nil {
			t.Errorf("NewMapper(len %d) = nil error", n)
		}
	}
	if _, err := NewMapper(testSalt); err != nil {
		t.Errorf("NewMapper(32 bytes) = %v", err)
	}
}

func TestYearShift(t *testing.T) {
	t.Parallel()
	seen := map[int]bool{}
	for i := 0; i < 64; i++ {
		salt := []byte(fmt.Sprintf("%032d", i))
		m, err := NewMapper(salt)
		if err != nil {
			t.Fatal(err)
		}
		s := m.YearShift()
		if s%4 != 0 || s < 28 || s > 72 {
			t.Fatalf("YearShift = %d", s)
		}
		seen[s] = true
	}
	if len(seen) < 6 {
		t.Errorf("YearShift takes %d values over 64 salts", len(seen))
	}
	m := newTestMapper(t)
	if m.YearShift() != newTestMapper(t).YearShift() {
		t.Error("YearShift is not deterministic")
	}
}

func shifted(m *Mapper, year int) string { return strconv.Itoa(year + m.YearShift()) }

func TestTextKeepsStructure(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	tests := []struct {
		in    string
		check func(out string) bool
	}{
		{"IMG_0001", func(o string) bool { return strings.HasPrefix(o, "IMG_") && len(o) == 8 }},
		{"img_0001", func(o string) bool { return strings.HasPrefix(o, "img_") }},
		{"IMG_0001 (1)", func(o string) bool { return strings.HasSuffix(o, " (1)") }},
		{"IMG_0001_edited", func(o string) bool { return strings.HasSuffix(o, "_edited") }},
		{"07-14 - X", func(o string) bool { return o == "07-14 - X" }},
		{"DSC_0001", func(o string) bool { return strings.HasPrefix(o, "DSC_") }},
		{"20230714", func(o string) bool { return o == shifted(m, 2023)+"0714" }},
		{"PXL_20230714_183027123", func(o string) bool { return o == "PXL_"+shifted(m, 2023)+"0714_183027123" }},
		{"IMG_20190101_120000", func(o string) bool { return o == "IMG_"+shifted(m, 2019)+"0101_120000" }},
		{"PXL_19700101_000001000", func(o string) bool { return o == "PXL_19700101_000001000" }},
		{"🙂 & !", func(o string) bool { return o == "🙂 & !" }},
		{"a-b c", func(o string) bool { return o == "a-b c" }},
	}
	for _, tt := range tests {
		out, err := m.Text(tt.in)
		if err != nil {
			t.Errorf("Text(%q): %v", tt.in, err)
			continue
		}
		if !tt.check(out) {
			t.Errorf("Text(%q) = %q", tt.in, out)
		}
	}
}

func TestTextDigits(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	for _, in := range []string{"0001", "123", "4821", "000", "1234567", "20231340", "19991231", "12345678901234567890"} {
		out, err := m.Text(in)
		if err != nil {
			t.Fatalf("Text(%q): %v", in, err)
		}
		if len(out) != len(in) || strings.Trim(out, "0123456789") != "" {
			t.Errorf("Text(%q) = %q: want digits of the same length", in, out)
		}
		if len(in) == 8 && !isKeptDate(in) {
			if d, err := time.Parse("20060102", out); err == nil && (d.Year() >= 1999 && d.Year() <= 2099 || d.Year() == 1969 || d.Year() == 1970) {
				t.Errorf("Text(%q) = %q: looks like a shifted or epoch date", in, out)
			}
		}
	}
	// Same input, same output; different inputs, different outputs.
	a1, _ := m.Text("x 4821")
	a2, _ := m.Text("y 4821")
	if a1[2:] != a2[2:] {
		t.Errorf("4821 maps to %q and %q", a1[2:], a2[2:])
	}
	outs := map[string]string{}
	for i := 0; i < 900; i++ {
		in := fmt.Sprintf("%03d", i)
		out, err := m.Text(in)
		if err != nil {
			t.Fatalf("Text(%q): %v", in, err)
		}
		if prev, ok := outs[out]; ok {
			t.Fatalf("Text(%q) = Text(%q) = %q", in, prev, out)
		}
		outs[out] = in
	}
}

func TestTextWords(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	words := []string{"Holiday", "holiday", "HOLIDAY", "hOliday", "Мырдон", "мырдон", "МЫРДОН", "Ёлка", "ёлка", "iPhone", "IPhone", "iphone", "Ab", "Straße", "Ελλάδα", "日本"}
	out := map[string]string{}
	for _, w := range words {
		o, err := m.Text(w)
		if err != nil {
			t.Fatalf("Text(%q): %v", w, err)
		}
		out[w] = o
		if runeLen(o) != runeLen(w) {
			t.Errorf("Text(%q) = %q: length differs", w, o)
		}
		if caseMask(o) != caseMask(w) {
			t.Errorf("Text(%q) = %q: case differs", w, o)
		}
		if strings.EqualFold(o, w) {
			t.Errorf("Text(%q) = %q: not replaced", w, o)
		}
	}
	for _, a := range words {
		for _, b := range words {
			if strings.EqualFold(a, b) != strings.EqualFold(out[a], out[b]) {
				t.Errorf("lower relation of %q, %q not kept: %q, %q", a, b, out[a], out[b])
			}
			if (a == b) != (out[a] == out[b]) {
				t.Errorf("equality of %q, %q not kept", a, b)
			}
		}
	}
	ascii := regexp.MustCompile(`^[g-zG-Z]+$`)
	if !ascii.MatchString(out["Holiday"]) {
		t.Errorf("ASCII word maps to %q: want letters g-z", out["Holiday"])
	}
	cyr := regexp.MustCompile(`^[абвгдежзиклмнопрстуфхцчшщъыьэюяАБВГДЕЖЗИКЛМНОПРСТУФХЦЧШЩЪЫЬЭЮЯ]+$`)
	if !cyr.MatchString(out["Мырдон"]) {
		t.Errorf("non-ASCII word maps to %q", out["Мырдон"])
	}
	ws := m.Words()
	if !sort.StringsAreSorted(ws) {
		t.Error("Words not sorted")
	}
	in := map[string]bool{}
	for _, w := range ws {
		in[w] = true
	}
	for _, w := range []string{"Holiday", "Мырдон", "Ab"} {
		if !in[strings.ToLower(out[w])] {
			t.Errorf("Words lacks the base form of %q", w)
		}
	}
	for _, w := range ws {
		if AllowedWord(w) {
			t.Errorf("generated word %q is a structural or format word", w)
		}
	}
}

// TestTextNFD needs Unicode normalization: the NFC and the NFD form of one
// word share the lower key, so their images are equal after NFC and lower
// case, differ as bytes, and the NFD image is decomposed.
func TestTextNFD(t *testing.T) {
	t.Parallel()
	if !NormalizationAvailable {
		t.Skip("no Unicode normalization in this build")
	}
	m := newTestMapper(t)
	composed := "Ёлка"
	decomposed := nfd(composed)
	if decomposed == composed {
		t.Fatal("nfd does not decompose")
	}
	a, err1 := m.Text(composed)
	b, err2 := m.Text(decomposed)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	if a == b || lowerNFC(a) != lowerNFC(b) {
		t.Errorf("Text(NFC) = %q, Text(NFD) = %q: want equal lower NFC forms, different bytes", a, b)
	}
	if a != nfc(a) || b == nfc(b) {
		t.Errorf("forms not kept: %q, %q", a, b)
	}
	if len(clusters(b)) != len(clusters(decomposed)) || caseMask(nfc(b)) != caseMask(composed) {
		t.Errorf("Text(NFD) = %q: length or case differs", b)
	}
	first := []rune(lowerNFC(a))[0]
	if !strings.ContainsRune(string(alphaDecomposable), first) {
		t.Errorf("decomposable cluster maps to %q: want a decomposable letter", first)
	}
}

// TestNumberRejectsDates forces the first attempt of an 8-digit output to a
// date: shifted and epoch dates are retried, other dates are kept.
func TestNumberRejectsDates(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		date  string
		retry bool
	}{{"20500101", true}, {"19990101", true}, {"20991231", true}, {"19700101", true}, {"19691231", true}, {"21000101", false}, {"19980101", false}} {
		mac := func(purpose string, input []byte, counter uint64) [32]byte {
			if purpose == purposeDigits && counter>>32 == 0 {
				var out [32]byte
				for i := 0; i < 8; i++ {
					out[i] = tt.date[i] - '0'
				}
				return out
			}
			return counterMAC(purpose, input, counter)
		}
		m := newMapperMAC(mac)
		out, err := m.Text("12345678")
		if err != nil {
			t.Fatal(err)
		}
		if len(out) != 8 || (out == tt.date) == tt.retry {
			t.Errorf("first attempt %s: Text = %q; retry %v", tt.date, out, tt.retry)
		}
	}
}

func TestTextDigitsAfterDate(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	for _, tt := range []struct {
		in   string
		kept bool
	}{
		{"IMG_20190514_101010", true},
		{"PXL_20190514_101010123", true},
		{"IMG_20190514_79161234567", false},
		{"PXL_20190514_1010101", false},
		{"IMG_20190514_1234", false},
		{"IMG_20190514_791612", false},
		{"IMG_20190514_256199123", false},
		{"IMG_20190514_235959", true},
	} {
		out, err := m.Text(tt.in)
		if err != nil {
			t.Fatal(err)
		}
		prefix := tt.in[:4] + shifted(m, 2019) + "0514_"
		tail := tt.in[len(prefix):]
		if !strings.HasPrefix(out, prefix) || len(out) != len(tt.in) || (out[len(prefix):] == tail) != tt.kept {
			t.Errorf("Text(%q) = %q; time digits kept: %v", tt.in, out, tt.kept)
		}
	}
}

func TestTextUUIDAndHex(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	u := "3F2A8C41-9D07-4B1E-8A0C-77F25B1E6A0C"
	id, err := m.ID(u)
	if err != nil {
		t.Fatal(err)
	}
	txt, err := m.Text(u)
	if err != nil {
		t.Fatal(err)
	}
	if txt != id {
		t.Errorf("Text(uuid) = %q; ID = %q", txt, id)
	}
	name, err := m.Name(u + ".heic")
	if err != nil {
		t.Fatal(err)
	}
	if name != id+".heic" {
		t.Errorf("Name(uuid.heic) = %q", name)
	}
	lower, _ := m.Text(strings.ToLower(u))
	if lower != strings.ToLower(id) {
		t.Errorf("Text(lower uuid) = %q; want %q", lower, strings.ToLower(id))
	}
	h := "d41d8cd98f00b204e9800998ecf8427e"
	ho, err := m.Name(h + ".jpg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ho, "00000000") || len(ho) != len(h)+4 || strings.Contains(ho, h[8:]) {
		t.Errorf("Name(hex.jpg) = %q", ho)
	}
	// A short hex-looking run is ordinary text.
	if o, _ := m.Text("ab12"); strings.HasPrefix(o, "00000000") {
		t.Errorf("Text(ab12) = %q", o)
	}
}

func TestName(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	tests := []struct {
		in    string
		check func(string) bool
	}{
		{"IMG_0001.HEIC", func(o string) bool { return strings.HasPrefix(o, "IMG_") && strings.HasSuffix(o, ".HEIC") }},
		{"IMG_0001.heic", func(o string) bool { return strings.HasSuffix(o, ".heic") }},
		{"clip.MTS", func(o string) bool { return strings.HasSuffix(o, ".MTS") }},
		{"notes.docx", func(o string) bool { return !strings.HasSuffix(o, ".docx") && strings.Count(o, ".") == 1 }},
		{".hidden", func(o string) bool { return strings.HasPrefix(o, ".") && o != ".hidden" }},
		{"a.", func(o string) bool { return o == "a." }},
	}
	for _, tt := range tests {
		o, err := m.Name(tt.in)
		if err != nil {
			t.Errorf("Name(%q): %v", tt.in, err)
			continue
		}
		if !tt.check(o) {
			t.Errorf("Name(%q) = %q", tt.in, o)
		}
	}
}

func TestPathAndFolder(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	p, err := m.Path("2023/07-14 - Мырдон/IMG_0001.HEIC")
	if err != nil {
		t.Fatal(err)
	}
	seg := strings.Split(p, "/")
	if len(seg) != 3 || seg[0] != shifted(m, 2023) || !strings.HasPrefix(seg[1], "07-14 - ") || !strings.HasPrefix(seg[2], "IMG_") {
		t.Errorf("Path = %q", p)
	}
	if p, _ := m.Path("1970/01-01/x.jpg"); !strings.HasPrefix(p, "1970/01-01/") {
		t.Errorf("Path(1970) = %q", p)
	}
	if p, _ := m.Path("2027/x.jpg"); !strings.HasPrefix(p, shifted(m, 2027)+"/") {
		t.Errorf("Path(2027) = %q", p)
	}
	for _, bad := range []string{"2028/x.jpg", "1968/x.jpg", "2023/123 Trip/x.jpg", "2023/07-145/x.jpg"} {
		if _, err := m.Path(bad); !errors.Is(err, ErrUnsupported) {
			t.Errorf("Path(%q) = %v; want ErrUnsupported", bad, err)
		}
	}
	// Four digits outside the first segment are ordinary text.
	if p, err := m.Path("Archive/2019/x.jpg"); err != nil || !regexp.MustCompile(`^[^/]+/[0-9]{4}/x\.jpg$`).MatchString(p) {
		t.Errorf("Path(Archive/2019) = %q, %v", p, err)
	}
	f, err := m.Folder("2023")
	if err != nil || f != shifted(m, 2023) {
		t.Errorf("Folder(2023) = %q, %v", f, err)
	}
	if _, err := m.Folder("2031"); !errors.Is(err, ErrUnsupported) {
		t.Errorf("Folder(2031) = %v", err)
	}
	ti, err := m.Title("07/14 Мырдон / Пляж")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ti, "07/14 ") || strings.Count(ti, " / ") != 1 || strings.Contains(ti, "Мырдон") {
		t.Errorf("Title = %q", ti)
	}
	// The album of a folder and the folder of an album stay one key.
	fo, _ := m.Path("2023/07-14 - Мырдон/x.jpg")
	k, _ := FolderToAlbum(FolderOf(fo))
	if k.Folders[0] != f || !strings.HasPrefix(k.Title, "07/14 ") {
		t.Errorf("album of the image folder = %+v", k)
	}
	tt, _ := m.Title("07/14 Мырдон")
	if k.Title != tt {
		t.Errorf("album title %q, title image %q", k.Title, tt)
	}
}

func TestDate(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	days := m.YearShift() / 4 * 1461
	tests := []struct {
		in      string
		shifted bool
	}{
		{"2023-07-14T18:03:27.455000+03:00", true},
		{"2024-02-29T23:59:59", true},
		{"2027-12-31T23:59:59.5", true},
		{"1971-01-01T00:00:00Z", true},
		{"1970-01-01T00:00:00+00:00", false},
		{"1969-12-31T21:00:00", false},
	}
	for _, tt := range tests {
		out, err := m.Date(tt.in)
		if err != nil {
			t.Errorf("Date(%q): %v", tt.in, err)
			continue
		}
		if len(out) != len(tt.in) || out[4:] != tt.in[4:] {
			t.Errorf("Date(%q) = %q: form changed", tt.in, out)
		}
		a, _ := ParseDT(tt.in)
		b, _ := ParseDT(out)
		want := time.Duration(0)
		if tt.shifted {
			want = time.Duration(days) * 24 * time.Hour
		}
		if b.Sub(a) != want {
			t.Errorf("Date(%q) = %q: shift %v, want %v", tt.in, out, b.Sub(a), want)
		}
	}
	if out, err := m.Date(""); out != "" || err != nil {
		t.Errorf("Date(\"\") = %q, %v", out, err)
	}
	for _, bad := range []string{"2028-01-01T00:00:00", "1968-12-31T00:00:00", "garbage", "2023:07:14 18:03:27"} {
		if _, err := m.Date(bad); !errors.Is(err, ErrUnsupported) {
			t.Errorf("Date(%q) = %v; want ErrUnsupported", bad, err)
		}
	}
}

func TestID(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	marker := regexp.MustCompile(`^00000000-[0-9A-F]{4}-8[0-9A-F]{3}-[89AB][0-9A-F]{3}-[0-9A-F]{12}$`)
	u := "3F2A8C41-9D07-4B1E-8A0C-77F25B1E6A0C"
	o, err := m.ID(u)
	if err != nil || !marker.MatchString(o) {
		t.Fatalf("ID(uuid) = %q, %v", o, err)
	}
	if lo, _ := m.ID(strings.ToLower(u)); lo != strings.ToLower(o) {
		t.Errorf("ID(lower) = %q", lo)
	}
	if o2, _ := m.ID("3F2A8C41-9D07-4B1E-8A0C-77F25B1E6A0D"); o2 == o {
		t.Error("two UUIDs gave one ID")
	}
	other, err := m.ID("burst-key-1")
	if err != nil || !regexp.MustCompile(`^00000000[0-9a-f]{24}$`).MatchString(other) {
		t.Errorf("ID(other) = %q, %v", other, err)
	}
	if e, err := m.ID(""); e != "" || err != nil {
		t.Errorf("ID(\"\") = %q, %v", e, err)
	}
}

func TestHash(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	h, err := m.Hash("da39a3ee5e6b4b0d3255bfef95601890afd80709")
	if err != nil || !regexp.MustCompile(`^00000000[0-9a-f]{32}$`).MatchString(h) {
		t.Errorf("Hash = %q, %v", h, err)
	}
	if h2, _ := m.Hash("DA39A3EE5E6B4B0D3255BFEF95601890AFD80709"); h2 != h {
		t.Error("Hash depends on case")
	}
	if e, _ := m.Hash(""); e != "" {
		t.Errorf("Hash(\"\") = %q", e)
	}
}

func TestSize(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	if m.Size(0) != 0 {
		t.Error("Size(0) != 0")
	}
	in := []int64{1, 2, 3, 4095, 4096, 1 << 20, 1<<32 - 1, 1 << 32, 1<<32 + 1, 5e9, 5e9 + 1, 1 << 46}
	prev := int64(0)
	for _, n := range in {
		s := m.Size(n)
		if s <= prev || s <= n {
			t.Errorf("Size(%d) = %d: not strictly increasing", n, s)
		}
		prev = s
	}
	if m.Size(5e9) != newTestMapper(t).Size(5e9) {
		t.Error("Size not deterministic")
	}
	// Neither the exact bytes nor the differences are kept.
	if a, b := int64(1e6), int64(1e9); m.Size(b)-m.Size(a) == b-a {
		t.Errorf("Size keeps the difference of %d and %d", a, b)
	}
	other, _ := NewMapper(bytes.Repeat([]byte{7}, 32))
	if m.sizeA == other.sizeA && m.sizeC == other.sizeC {
		t.Error("two salts give the same size map")
	}
	for i := 0; i < 64; i++ {
		s, _ := NewMapper([]byte(fmt.Sprintf("%032d", i)))
		if s.sizeA < 1 || s.sizeC < 1 || s.sizeC > 4096 {
			t.Fatalf("size map a=%d c=%d", s.sizeA, s.sizeC)
		}
	}
}

func TestMapperUnsupportedText(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	for _, bad := range []string{"a@b", `a\b`, "55°", "a\tb", "\u0301x", "x\u0663", "/abs", "~home", "C:/x", "55.75393 x", "55.75 x", "İx", "Σx"} {
		if _, err := m.Text(bad); !errors.Is(err, ErrUnsupported) {
			t.Errorf("Text(%q) = %v; want ErrUnsupported", bad, err)
		}
	}
	if _, err := m.Text("е́"); errors.Is(err, ErrUnsupported) == NormalizationAvailable {
		t.Errorf("Text(mark) = %v with normalization %v", err, NormalizationAvailable)
	}
}

func TestMapperForcedCollision(t *testing.T) {
	t.Parallel()
	m := newMapperMAC(counterMAC)
	a, err := m.Text("alpha")
	if err != nil {
		t.Fatal(err)
	}
	b, err := m.Text("bravo")
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Errorf("collision not resolved: %q", a)
	}
	d1, _ := m.Text("123")
	d2, _ := m.Text("456")
	if d1 == d2 {
		t.Errorf("digit collision not resolved: %q", d1)
	}
	i1, _ := m.ID("one")
	i2, _ := m.ID("two")
	if i1 == i2 {
		t.Errorf("id collision not resolved: %q", i1)
	}
}

func TestMapperExhausted(t *testing.T) {
	t.Parallel()
	m := newMapperMAC(constantMAC)
	if _, err := m.Text("alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Text("bravo"); !errors.Is(err, ErrExhausted) {
		t.Errorf("second word: %v; want ErrExhausted", err)
	}
	if _, err := m.Text("123"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Text("456"); !errors.Is(err, ErrExhausted) {
		t.Errorf("second number: %v; want ErrExhausted", err)
	}
}

func TestMapperAssetAndFile(t *testing.T) {
	t.Parallel()
	m := newTestMapper(t)
	a := photo(1, "IMG_4821.HEIC", "2023-07-14T18:03:27.455000+03:00", 2734118)
	a.LivePhoto, a.Burst, a.BurstKeyPhoto, a.BurstPhotos = true, true, true, []string{uuidN(3), uuidN(2)}
	a.Albums = []AlbumRef{{FolderNames: []string{"2023"}, Title: "07/14 Мырдон"}}
	ma, err := m.Asset(a)
	if err != nil {
		t.Fatal(err)
	}
	if ma.UUID == a.UUID || ma.Filename != ma.UUID+".heic" || !ma.LivePhoto || ma.TZOffset != a.TZOffset ||
		ma.OriginalWidth != a.OriginalWidth || ma.OriginalFilesize <= a.OriginalFilesize || !ma.BurstKeyPhoto ||
		ma.Albums[0].FolderNames[0] != shifted(m, 2023) {
		t.Errorf("Asset = %+v", ma)
	}
	// burst_photos: mapped in place as the UUIDs of those photos map, also
	// outside the set.
	id2, _ := m.ID(uuidN(2))
	id3, _ := m.ID(uuidN(3))
	if !reflect.DeepEqual(ma.BurstPhotos, []string{id3, id2}) || id2 == uuidN(2) || !markerUUIDRE.MatchString(id3) {
		t.Errorf("burst_photos = %v", ma.BurstPhotos)
	}
	if nb, err := m.Asset(photo(5, "IMG_0005.HEIC", "2023-07-14T18:03:27+03:00", 1)); err != nil || nb.BurstPhotos == nil || len(nb.BurstPhotos) != 0 {
		t.Errorf("burst_photos of a single photo = %#v, %v", nb.BurstPhotos, err)
	}
	f := cloudFile("2023/07-14 - Мырдон/IMG_4821.HEIC", "2023-07-14T18:03:27.455", 2734118)
	f.ContentID, f.LivePair, f.SHA1 = "91C38A0E-B2D4-4F1A-9E3C-5B706F1A9E3C", LivePairCID, "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	mf, err := m.File(f)
	if err != nil {
		t.Fatal(err)
	}
	if mf.Size != ma.OriginalFilesize || mf.Kind != f.Kind || mf.LivePair != f.LivePair || mf.Width != f.Width ||
		mf.ContentID == f.ContentID || mf.SHA1 == f.SHA1 || BaseName(mf.Path) != ma.OriginalFilename {
		t.Errorf("File = %+v (asset %+v)", mf, ma)
	}
	s, err := m.Set(&Set{Assets: []Asset{a}, Files: []File{f}, Src: []int{0, 1}})
	if err != nil {
		t.Fatal(err)
	}
	if s.Assets[0].UUID != ma.UUID || s.Files[0].Path != mf.Path || s.Src[1] != 1 {
		t.Errorf("Set = %+v", s)
	}
}
