package fixtures_test

import (
	"io/fs"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/fixtures"
)

// fullCase holds the fields that matching does not read too, a burst key
// both as JSON false and as a string among them, as the anonymized cases do.
const fullCase = `{
 "format": 1,
 "provenance": {"kind": "real-anon", "source": "manual-sync", "expected": "reference-run", "category": "live"},
 "icloud": [
  {"uuid": "u1", "original_filename": "IMG_0001.HEIC", "filename": "u1.heic",
   "date": "2024-05-01T10:00:00.075000+03:00", "tzoffset": 10800, "original_filesize": 100,
   "original_width": 4032, "original_height": 3024, "width": 3024, "height": 4032,
   "hasadjustments": true, "live_photo": true, "ismovie": false, "has_raw": true, "israw": false,
   "burst": false, "burst_key": false, "burst_photos": [],
   "album_info": [{"folder_names": ["2024"], "title": "05/01 Trip"}]},
  {"uuid": "u2", "original_filename": "IMG_0002.MOV", "filename": "u2.mov",
   "date": null, "tzoffset": 0, "original_filesize": 300,
   "original_width": 1920, "original_height": 1080, "width": 1080, "height": 1920,
   "live_photo": false, "ismovie": true, "has_raw": false, "burst_key": "00000000AABBCCDD"},
  {"uuid": "u3", "original_filename": "IMG_0003.JPG", "filename": "u3.jpeg",
   "date": "2024-05-01T07:00:01.400000+00:00", "tzoffset": 0, "original_filesize": 50,
   "original_width": 0, "original_height": 0, "width": 640, "height": 480,
   "live_photo": false, "ismovie": false, "has_raw": false}
 ],
 "cloud": [
  {"path": "2024/05-01 - Café/IMG_0001.HEIC", "kind": "photos_ok", "size": 90,
   "date_taken": "2024-05-01T10:00:00.075", "width": 4032, "height": 3024,
   "content_id": "00000000-0000-8000-8000-000000000001", "live_pair": "cid", "sha1": "00000000aa"},
  {"path": "2024/05-01 - Café/IMG_0001.MOV", "kind": "photos_ok", "size": 200,
   "date_taken": "", "width": 0, "height": 0, "content_id": "", "live_pair": "", "sha1": ""},
  {"path": "2024/05-01 - Café/IMG_0001.xmp", "kind": "sidecar", "size": 1,
   "date_taken": "2024-05-01T10:00:00", "width": 0, "height": 0, "content_id": "", "live_pair": "", "sha1": ""}
 ],
 "expected": [
  {"uuid": "u1", "path": "2024/05-01 - Café/IMG_0001.HEIC", "level": "same_shot", "plan": "replace"},
  {"uuid": "u1", "path": "2024/05-01 - Café/IMG_0001.MOV", "level": "pair_component", "plan": ""}
 ]
}`

// Written from the format of the anonymized cases and the contract of a
// snapshot and a scan, not from the code.
func TestLoad(t *testing.T) {
	t.Parallel()
	fsys := fstest.MapFS{
		"tokens.json":     {Data: []byte(`{"format": 1, "words": ["trip"]}`)},
		"notes/README.md": {Data: []byte("not a case")},
		"b/case.json":     {Data: []byte(fullCase)},
		"a/case.json":     {Data: []byte(`{"format": 1}`)},
	}
	plus3, zero := 3*3600, 0
	const dir = "2024/05-01 - Café/"
	want := []fixtures.Case{
		{Name: "a"},
		{
			Name: "b",
			Assets: []domain.Asset{
				{
					Key: domain.AssetKey{UUID: "u1"}, Library: domain.LibraryPersonal, OriginalName: "IMG_0001.HEIC",
					Taken: time.Date(2024, 5, 1, 7, 0, 0, 0, time.UTC), TZOffset: &plus3, SubSec: "075000",
					Width: 4032, Height: 3024,
					Resources: []domain.Resource{
						{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100},
						{Role: domain.ResourceLiveVideo},
						{Role: domain.ResourceRAWPair},
					},
				},
				{
					Key: domain.AssetKey{UUID: "u2"}, Library: domain.LibraryPersonal, OriginalName: "IMG_0002.MOV",
					Width: 1920, Height: 1080, IsVideo: true,
					Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0002.MOV", Size: 300}},
				},
				{
					Key: domain.AssetKey{UUID: "u3"}, Library: domain.LibraryPersonal, OriginalName: "IMG_0003.JPG",
					Taken: time.Date(2024, 5, 1, 7, 0, 1, 0, time.UTC), TZOffset: &zero, SubSec: "400000",
					Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0003.JPG", Size: 50}},
				},
			},
			Files: []domain.RemoteEntry{
				{
					Root: domain.LibraryPersonal, Path: dir + "IMG_0001.HEIC", Size: 90,
					Meta: domain.MediaMeta{Taken: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC), SubSec: "075", Width: 4032, Height: 3024},
				},
				{Root: domain.LibraryPersonal, Path: dir + "IMG_0001.MOV", Size: 200},
				{
					Root: domain.LibraryPersonal, Path: dir + "IMG_0001.xmp", Size: 1,
					Meta: domain.MediaMeta{Taken: time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)},
				},
			},
			Expected: []fixtures.Link{
				{UUID: "u1", Path: dir + "IMG_0001.HEIC", Level: domain.LevelSameShot, Plan: "replace"},
				{UUID: "u1", Path: dir + "IMG_0001.MOV", Level: domain.LevelPairComponent},
			},
		},
	}
	if got, err := fixtures.Load(fsys); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("Load = %+v, %v; want %+v", got, err, want)
	}
}

// The case broken fails in its own way after the good case a, which Load must
// not return either.
func TestLoadErrors(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		file fstest.MapFile
	}{
		{"not JSON", fstest.MapFile{Data: []byte(`{"format": 1,`)}},
		{"another format", fstest.MapFile{Data: []byte(`{"format": 2}`)}},
		{"no format", fstest.MapFile{Data: []byte(`{"icloud": []}`)}},
		{"a date of an asset without a time zone", fstest.MapFile{Data: []byte(`{"format": 1, "icloud": [{"uuid": "u1", "date": "2024-05-01T10:00:00"}]}`)}},
		{"a capture time of a file with a time zone", fstest.MapFile{Data: []byte(`{"format": 1, "cloud": [{"path": "a/IMG_0001.HEIC", "date_taken": "2024-05-01T10:00:00+03:00"}]}`)}},
		{"a capture time of a file that is no time", fstest.MapFile{Data: []byte(`{"format": 1, "cloud": [{"path": "a/IMG_0001.HEIC", "date_taken": "yesterday"}]}`)}},
		{"a directory named case.json", fstest.MapFile{Mode: fs.ModeDir}},
	} {
		fsys := fstest.MapFS{"a/case.json": {Data: []byte(`{"format": 1}`)}, "broken/case.json": &c.file}
		if cases, err := fixtures.Load(fsys); err == nil || !strings.Contains(err.Error(), "broken") || cases != nil {
			t.Errorf("%s: Load = %d cases, %v; want no cases and an error that names the case", c.name, len(cases), err)
		}
	}
}
