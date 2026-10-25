package main

import (
	"reflect"
	"testing"
	"testing/fstest"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

func TestNaNToNull(t *testing.T) {
	t.Parallel()
	tests := []struct{ in, want string }{
		{`{"a": NaN, "b": Infinity, "c": -Infinity, "d": -1.5}`, `{"a": null, "b": null    , "c": null     , "d": -1.5}`},
		{`[NaN,NaN,-Infinity]`, `[null,null,null     ]`},
		// Strings are untouched, escapes included.
		{`{"t": "NaN", "u": "a\"NaN", "v": "a\\", "w": NaN, "x": "-Infinity"}`, `{"t": "NaN", "u": "a\"NaN", "v": "a\\", "w": null, "x": "-Infinity"}`},
		// Anything else passes as it is.
		{`[Nan, NaX, Inf, -Inf, -1, nan]`, `[Nan, NaX, Inf, -Inf, -1, nan]`},
		{`[-Infinit`, `[-Infinit`}, //nolint:misspell // a truncated token on purpose
		{`NaN`, `null`},
	}
	for _, tt := range tests {
		if got := string(nanToNull([]byte(tt.in))); got != tt.want {
			t.Errorf("nanToNull(%q) = %q; want %q", tt.in, got, tt.want)
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
	data, err := readInput(fsys, namePlan)
	if err != nil || string(data) != `{"download": [], "upload": [], "replace": [], "conflicts": [], "stats": null}` {
		t.Errorf("readInput = %q, %v", data, err)
	}
}

// TestSyntaxErrorOffset: the offset counts the bytes after the NaN
// replacement up to the end of the offending byte; no byte of the input is
// in the error.
func TestSyntaxErrorOffset(t *testing.T) {
	t.Parallel()
	_, err := readCatalog(fstest.MapFS{nameCatalog: {Data: []byte(`[NaN Zinaida]`)}})
	if err == nil || err.Error() != "read catalog: syntax error at offset 7" {
		t.Errorf("syntax error: %v", err)
	}
	_, err = readCatalog(fstest.MapFS{nameCatalog: {Data: []byte(`[{"uuid": "A"}] "Zinaida"`)}})
	if err == nil || err.Error() != "read catalog: syntax error at offset 17" {
		t.Errorf("trailing data: %v", err)
	}
}
