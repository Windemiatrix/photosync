package main

import (
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"testing/iotest"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// filtered reads s through a filter with the smallest buffer, one input
// byte per read and one output byte per read, so every token crosses read
// and buffer boundaries.
func filtered(t *testing.T, s string) string {
	t.Helper()
	f := newNaNFilter(iotest.OneByteReader(strings.NewReader(s)), 16)
	out, err := io.ReadAll(iotest.OneByteReader(f))
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestNaNFilter(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{`{"a": NaN, "b": Infinity, "c": -Infinity, "d": -1.5}`, `{"a": null, "b": null    , "c": null     , "d": -1.5}`},
		{`[NaN,NaN,-Infinity]`, `[null,null,null     ]`},
		// Strings are untouched, escapes included.
		{`{"t": "NaN", "u": "a\"NaN", "v": "a\\", "w": NaN, "x": "-Infinity"}`, `{"t": "NaN", "u": "a\"NaN", "v": "a\\", "w": null, "x": "-Infinity"}`},
		// Anything else passes as it is.
		{`[Nan, NaX, Inf, -Inf, -1, nan]`, `[Nan, NaX, Inf, -Inf, -1, nan]`},
		{`[-Infinit`, `[-Infinit`},
		{`NaN`, `null`},
	}
	for _, tt := range tests {
		if got := filtered(t, tt.in); got != tt.want {
			t.Errorf("filter(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
	// Every position of the tokens against the 16-byte buffer.
	for pad := 0; pad < 40; pad++ {
		p := strings.Repeat(" ", pad)
		in := p + `["x\"NaN", NaN, Infinity, -Infinity]`
		want := p + `["x\"NaN", null, null    , null     ]`
		if got := filtered(t, in); got != want {
			t.Errorf("pad %d: %q", pad, got)
		}
	}
}

// TestCatalogNaN: the three tokens decode as null in a decoded field (the
// zero value) and in a field that is never decoded; strings keep them.
func TestCatalogNaN(t *testing.T) {
	t.Parallel()
	catalog := `[{"uuid": "A", "tzoffset": NaN, "original_filesize": Infinity, "width": -Infinity,
 "score": {"overall": NaN, "curation": -Infinity}, "latitude": NaN, "filename": "NaN",
 "album_info": [{"title": "a\"NaN", "folder_names": []}]}]`
	got, err := readCatalog(fstest.MapFS{nameCatalog: {Data: []byte(catalog)}})
	if err != nil {
		t.Fatalf("readCatalog: %v", err)
	}
	want := []anon.Asset{{UUID: "A", Filename: "NaN", Albums: []anon.AlbumRef{{FolderNames: []string{}, Title: `a"NaN`}}}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("readCatalog = %+v", got)
	}
}

func TestOtherInputsNaN(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		nameInventory: {Data: []byte(`{"files": [{"path": "2023/a.jpg", "kind": "photos_ok", "size": 1, "width": NaN, "height": Infinity, "mtime": -Infinity}]}`)},
		nameLinks:     {Data: []byte(`{"A": [["2023/a.jpg", "exact"]], "score": []}`)},
		namePlan:      {Data: []byte(`{"download": [], "upload": [], "replace": [], "conflicts": [], "stats": NaN}`)},
	}
	inv, err := readInventory(fsys, nameInventory, false)
	if err != nil || len(inv.Files) != 1 || inv.Files[0].Width != 0 || inv.Files[0].Height != 0 {
		t.Errorf("readInventory = %+v, %v", inv, err)
	}
	if _, err := readLinks(fsys); err != nil {
		t.Errorf("readLinks: %v", err)
	}
	data, _, err := readJSON(fsys, namePlan, stagePlan)
	if err != nil || !strings.Contains(string(data), `"stats": null}`) {
		t.Errorf("readJSON = %q, %v", data, err)
	}
}

// TestSyntaxErrorDetail: a syntax error reports the offset in the input
// (not in the filtered stream), the class of the byte and its place, and no
// byte of the input.
func TestSyntaxErrorDetail(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, doc, mark, detail string
	}{
		{"letter after NaN", `[{"uuid": "A", "s": NaN, "t": NaN} Zinaida]`, "Z", "letter byte in array element at depth 1"},
		{"control in a string", "[{\"uuid\": \"A\x01\"}]", "\x01", "control byte in string in object value at depth 2"},
		{"digit as a key", `[{"uuid": "A", 5: 1}]`, "5", "digit byte in object key at depth 2"},
		{"non-ASCII value", `[{"uuid": Ё}]`, "Ё", "non-ASCII byte in object value at depth 2"},
	}
	for _, tt := range tests {
		_, err := readCatalog(fstest.MapFS{nameCatalog: {Data: []byte(tt.doc)}})
		// The offset counts the bytes read up to the end of the offending
		// character, as encoding/json reports it.
		want := "read catalog: syntax error at offset " + strconv.Itoa(strings.Index(tt.doc, tt.mark)+len(tt.mark)) + ": " + tt.detail
		if err == nil || err.Error() != want {
			t.Errorf("%s: %v; want %s", tt.name, err, want)
		}
	}
	if got := syntaxDetail(strings.NewReader("[1"), 5); got != "end of input" {
		t.Errorf("syntaxDetail past the end = %q", got)
	}
	if got := byteClass('{'); got != "punctuation" {
		t.Errorf("byteClass = %q", got)
	}
	if got := byteClass('\t'); got != "whitespace" {
		t.Errorf("byteClass = %q", got)
	}
}
