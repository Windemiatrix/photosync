// Package fixtures reads the anonymized cases of matching in testdata/fixtures
// into the types of the domain, so that tests replay them.
package fixtures

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Case is one anonymized case: the assets of a slice of a Photos catalog, the
// files of a slice of a cloud inventory and the links that the reference
// manual sync made between them. Name is the directory of the case.
type Case struct {
	Name     string
	Assets   []domain.Asset
	Files    []domain.RemoteEntry
	Expected []Link
}

// Link is a link that the reference manual sync made from the asset of UUID
// to the file of Path. Plan is "replace" when the original replaced the file,
// "keep_cloud" when the file stayed as the better copy, and empty when the
// sync did not compare the two.
type Link struct {
	UUID, Path string
	Level      domain.MatchLevel
	Plan       string
}

// The catalog and the inventory hold more than a snapshot and a scan give,
// and burst_key is a string or false; what matching does not read stays out.
type record struct {
	Format int
	ICloud []struct {
		UUID     string
		Name     string `json:"original_filename"`
		Date     string
		TZOffset int   `json:"tzoffset"`
		Size     int64 `json:"original_filesize"`
		Width    int   `json:"original_width"`
		Height   int   `json:"original_height"`
		Live     bool  `json:"live_photo"`
		Video    bool  `json:"ismovie"`
		RAW      bool  `json:"has_raw"`
	}
	Cloud []struct {
		Path          string
		Size          int64
		Taken         string `json:"date_taken"`
		Width, Height int
	}
	Expected []Link
}

// Load reads the cases of fsys, one from case.json in each directory, in the
// order of the names of the directories; other files stay out. An asset is in
// the personal library, its width and height are those of its original, and
// it has a Live video and a RAW, with neither a name nor a size, when
// live_photo and has_raw tell so. A file is in the personal root whatever its
// kind, and its capture time has no time zone. Strings stay as written: Taken
// keeps whole seconds and SubSec the digits after the point, and an asset has
// a TZOffset exactly when it has a date. On an error Load returns no cases and
// an error that names the case.
func Load(fsys fs.FS) ([]Case, error) {
	// Glob fails only on a malformed pattern.
	names, _ := fs.Glob(fsys, "*/case.json")
	var cases []Case
	for _, name := range names {
		c, err := read(fsys, name)
		if err != nil {
			return nil, fmt.Errorf("fixtures: %s: %w", path.Dir(name), err)
		}
		cases = append(cases, c)
	}
	return cases, nil
}

func read(fsys fs.FS, name string) (Case, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return Case{}, err
	}
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return Case{}, err
	}
	if r.Format != 1 {
		return Case{}, fmt.Errorf("format %d, want 1", r.Format)
	}
	c := Case{Name: path.Dir(name), Expected: r.Expected}
	for _, a := range r.ICloud {
		taken, subsec, err := moment(a.Date, time.RFC3339)
		if err != nil {
			return Case{}, err
		}
		asset := domain.Asset{
			Key: domain.AssetKey{UUID: a.UUID}, Library: domain.LibraryPersonal, OriginalName: a.Name,
			Taken: taken, SubSec: subsec, Width: a.Width, Height: a.Height, IsVideo: a.Video,
			Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: a.Name, Size: a.Size}},
		}
		if a.Date != "" {
			asset.TZOffset = &a.TZOffset
		}
		if a.Live {
			asset.Resources = append(asset.Resources, domain.Resource{Role: domain.ResourceLiveVideo})
		}
		if a.RAW {
			asset.Resources = append(asset.Resources, domain.Resource{Role: domain.ResourceRAWPair})
		}
		c.Assets = append(c.Assets, asset)
	}
	for _, f := range r.Cloud {
		taken, subsec, err := moment(f.Taken, "2006-01-02T15:04:05")
		if err != nil {
			return Case{}, err
		}
		c.Files = append(c.Files, domain.RemoteEntry{
			Root: domain.LibraryPersonal, Path: f.Path, Size: f.Size,
			Meta: domain.MediaMeta{Taken: taken, SubSec: subsec, Width: f.Width, Height: f.Height},
		})
	}
	return c, nil
}

// A snapshot and a scan keep the fraction of a second out of Taken, as the
// digits that the metadata writes.
func moment(s, layout string) (time.Time, string, error) {
	if s == "" {
		return time.Time{}, "", nil
	}
	t, err := time.Parse(layout, s)
	_, fraction, _ := strings.Cut(s, ".")
	return t.UTC().Truncate(time.Second), fraction[:len(fraction)-len(strings.TrimLeft(fraction, "0123456789"))], err
}
