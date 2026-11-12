package pathtemplate_test

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
)

const sea = "2024/05-01 - Sea"

var (
	created = time.Date(2024, 5, 1, 10, 30, 0, 0, time.UTC)
	now     = time.Date(2001, 1, 2, 3, 4, 5, 0, time.UTC)
)

// fields gives the values of its map and reports any other name as not
// given.
type fields map[string]template.Value

func (f fields) Field(name string) (template.Value, bool) {
	v, ok := f[name]
	return v, ok
}

// photo returns an asset whose original is its second resource, and the
// values of its fields; each call returns new ones.
func photo() (domain.Asset, fields) {
	a := domain.Asset{
		Key:          domain.AssetKey{UUID: "uuid-1"},
		OriginalName: "IMG_0001.HEIC",
		Resources: []domain.Resource{
			{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 2},
			{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 1},
		},
	}
	return a, fields{
		"original_name": {Strings: []string{"IMG_0001.HEIC"}},
		"created":       {Time: &created},
		"title":         {Strings: []string{"Sea/Sun: day"}},
		"album":         {Strings: []string{"Holidays", "Trips"}},
		"keyword":       {Strings: []string{"Beach", "Sea/Sun", "Sea∕Sun"}},
		"folder_album":  {Albums: []template.AlbumRef{{Folders: []string{"Travel/2024"}, Title: "05/01 Sea"}}},
	}
}

func parse(t *testing.T, s string) *template.Template {
	t.Helper()
	tmpl, err := template.Parse(s)
	if err != nil {
		t.Fatalf("template.Parse(%q): %v", s, err)
	}
	return tmpl
}

func TestEventPath(t *testing.T) {
	t.Parallel()
	offset := 3 * 3600
	asset := domain.Asset{Taken: time.Date(2024, 5, 1, 22, 30, 0, 0, time.UTC), TZOffset: &offset}
	p := domain.DefaultPolicies()
	p.UntitledEvent, p.AlbumsRoot = "Untitled", "Albums"
	event := domain.Album{ID: 7, FolderTitles: []string{"2024"}, Title: "05/01 Sea"}
	// The pair of no album is a trap: the day folder never has a pair.
	pairs := []domain.AlbumPair{
		{AlbumID: 0, DirPath: "Unsorted"},
		{AlbumID: 3, DirPath: "2024/05-01 - Hills"},
		{AlbumID: 7, DirPath: "2024/05-01 - Sea trip"},
	}
	for _, c := range []struct {
		name  string
		album domain.Album
		pairs []domain.AlbumPair
		want  string
	}{
		{"the zero album: the day folder of the local date", domain.Album{}, pairs, "2024/05-02"},
		{"the directory of the pair, not the one of the title", event, pairs, "2024/05-01 - Sea trip"},
		{"an event without a pair", event, pairs[:2], "2024/05-01 - Sea"},
		{"a thematic album without a pair", domain.Album{ID: 9, Title: "Hobby"}, pairs, "Albums/Hobby"},
	} {
		if got := pathtemplate.EventPath(asset, c.album, c.pairs, p); got != c.want {
			t.Errorf("%s: EventPath = %q; want %q", c.name, got, c.want)
		}
	}
}

// The expected paths follow the godoc of Render and the behavior of the
// templates of osxphotos, not the code.
func TestRender(t *testing.T) {
	t.Parallel()
	nfd := norm.NFD.String("2024/05-01 - Майские")
	for _, c := range []struct {
		name, tmpl, event string
		mode              domain.Mode
		edit              func(*domain.Asset, fields)
		want              []string
		err               error
		text              string // in the message of err
	}{
		{name: "the default template", tmpl: "{event_path}/{original_name}", event: sea, mode: domain.ModeTwoWay,
			want: []string{"2024/05-01 - Sea/IMG_0001.HEIC"}},
		{name: "download-only", tmpl: "{event_path}/{original_name}", event: sea, mode: domain.ModeDownloadOnly,
			want: []string{"2024/05-01 - Sea/IMG_0001.HEIC"}},
		{name: "text only", tmpl: "photo", event: sea, mode: domain.ModeUploadOnly,
			want: []string{"photo.HEIC"}},
		{name: "the extension of the original file", tmpl: "{event_path}/{original_name}", event: sea, mode: domain.ModeTwoWay,
			edit: func(a *domain.Asset, _ fields) { a.Resources[1].Filename = "IMG_0001.heic" },
			want: []string{"2024/05-01 - Sea/IMG_0001.heic"}},
		{name: "no original file", tmpl: "{event_path}/{original_name}", event: sea, mode: domain.ModeTwoWay,
			edit: func(a *domain.Asset, _ fields) { a.Resources = a.Resources[:1] },
			err:  pathtemplate.ErrInvalidPath, text: "uuid-1"},
		{name: "fields of osxphotos", tmpl: "{created.year}/{created.mm}/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			want: []string{"2024/05/IMG_0001.HEIC"}},
		{name: "a path for each album", tmpl: "{album}/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			want: []string{"Holidays/IMG_0001.HEIC", "Trips/IMG_0001.HEIC"}},
		{name: "paths sorted, a repeat once", tmpl: "{keyword}/{album}/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			want: []string{
				"Beach/Holidays/IMG_0001.HEIC", "Beach/Trips/IMG_0001.HEIC",
				"Sea∕Sun/Holidays/IMG_0001.HEIC", "Sea∕Sun/Trips/IMG_0001.HEIC",
			}},
		{name: "a value makes no level", tmpl: "{event_path}/{title}", event: sea, mode: domain.ModeTwoWay,
			want: []string{"2024/05-01 - Sea/Sea∕Sun꞉ day.HEIC"}},
		{name: "the folders and the title of an album", tmpl: "{folder_album}/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			want: []string{"Travel∕2024/05∕01 Sea/IMG_0001.HEIC"}},
		{name: "a field without a value", tmpl: "{event_path}/{title}", event: sea, mode: domain.ModeTwoWay,
			edit: func(_ *domain.Asset, f fields) { f["title"] = template.Value{} },
			want: []string{"2024/05-01 - Sea/_.HEIC"}},
		{name: "a field the photo does not give", tmpl: "{event_path}/{title}", event: sea, mode: domain.ModeTwoWay,
			edit: func(_ *domain.Asset, f fields) { delete(f, "title") },
			err:  template.ErrUnsupportedField, text: "title"},
		{name: "the event", tmpl: "{event_path}/{event} {original_name}", event: sea, mode: domain.ModeTwoWay,
			want: []string{"2024/05-01 - Sea/05-01 - Sea IMG_0001.HEIC"}},
		{name: "an event path as the cloud holds it", tmpl: "{event_path}/{event} {original_name}", event: nfd, mode: domain.ModeTwoWay,
			want: []string{nfd + "/05-01 - Майские IMG_0001.HEIC"}},
		{name: "an event path that photosync would not make", tmpl: "{event_path}/{original_name}", event: "2024/05-01 - Sea?", mode: domain.ModeTwoWay,
			want: []string{"2024/05-01 - Sea?/IMG_0001.HEIC"}},
		{name: "today is now", tmpl: "{event_path}/{today.year} {original_name}", event: sea, mode: domain.ModeTwoWay,
			want: []string{"2024/05-01 - Sea/2001 IMG_0001.HEIC"}},
		{name: "out of the root", tmpl: "../{original_name}", event: sea, mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrInvalidPath, text: `"../IMG_0001.HEIC"`},
		{name: "an absolute path", tmpl: "/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrInvalidPath, text: `"/IMG_0001.HEIC"`},
		{name: "a dot level after the event path", tmpl: "{event_path}/./{original_name}", event: sea, mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrInvalidPath},
		{name: "an empty event path", tmpl: "/{original_name}", event: "", mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrInvalidPath},
		{name: "no path", tmpl: "", event: sea, mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrInvalidPath, text: "no path"},
		{name: "several paths in two-way", tmpl: "{event_path}/{album}", event: sea, mode: domain.ModeTwoWay,
			err: pathtemplate.ErrMode},
		{name: "another directory in two-way", tmpl: "{album}/{original_name}", event: sea, mode: domain.ModeTwoWay,
			err: pathtemplate.ErrMode},
		{name: "a directory below the event in download-only", tmpl: "{event_path}/{created.year}/{original_name}", event: sea, mode: domain.ModeDownloadOnly,
			err: pathtemplate.ErrMode},
		{name: "an unknown field", tmpl: "{event_path}/{foo}", event: sea, mode: domain.ModeTwoWay,
			err: pathtemplate.ErrUnknownField, text: "foo"},
		{name: "the event path with a filter", tmpl: "{event_path|lower}/{original_name}", event: sea, mode: domain.ModeUploadOnly,
			err: pathtemplate.ErrUnknownField, text: "event_path"},
		{name: "the event inside a field", tmpl: "{event_path}/{title,{event}}", event: sea, mode: domain.ModeTwoWay,
			err: pathtemplate.ErrUnknownField, text: "event"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			a, f := photo()
			if c.edit != nil {
				c.edit(&a, f)
			}
			got, err := pathtemplate.Render(parse(t, c.tmpl), c.event, a, f, c.mode, now)
			if c.err != nil {
				if !errors.Is(err, c.err) || !strings.Contains(fmt.Sprint(err), c.text) {
					t.Errorf("Render(%q, %q) = %q, %v; want an error %v with %q", c.tmpl, c.event, got, err, c.err, c.text)
				}
				return
			}
			if err != nil || !slices.Equal(got, c.want) {
				t.Errorf("Render(%q, %q) = %q, %v; want %q", c.tmpl, c.event, got, err, c.want)
			}
		})
	}
}

// One template serves every asset of a run, so the paths of an asset must
// not depend on the assets rendered before it.
func TestRenderRepeats(t *testing.T) {
	t.Parallel()
	tmpl := parse(t, "{event_path}/{counter} {original_name}")
	for _, event := range []string{sea, "2024/05-02 - Hills", sea} {
		a, f := photo()
		got, err := pathtemplate.Render(tmpl, event, a, f, domain.ModeTwoWay, now)
		if want := []string{event + "/0 IMG_0001.HEIC"}; err != nil || !slices.Equal(got, want) {
			t.Errorf("Render(%q) = %q, %v; want %q", event, got, err, want)
		}
	}
}

func TestRenderLeavesFields(t *testing.T) {
	t.Parallel()
	a, f := photo()
	if _, err := pathtemplate.Render(parse(t, "{title}/{keyword}/{folder_album}/{original_name}"), sea, a, f,
		domain.ModeUploadOnly, now); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if _, want := photo(); !reflect.DeepEqual(f, want) {
		t.Errorf("Render changed the values of the photo to %v; want %v", f, want)
	}
}

// Any title stays one valid level until the name with its extension is
// longer than the 255 bytes that the cloud takes.
func TestRenderValues(t *testing.T) {
	t.Parallel()
	tmpl := parse(t, "{event_path}/{title}")
	rapid.Check(t, func(t *rapid.T) {
		title := rapid.StringN(0, 200, -1).Draw(t, "title")
		a, f := photo()
		f["title"] = template.Value{Strings: []string{title}}
		got, err := pathtemplate.Render(tmpl, sea, a, f, domain.ModeTwoWay, now)
		name := naming.SanitizePart(title) + ".HEIC"
		switch {
		case len(name) > 255:
			if !errors.Is(err, pathtemplate.ErrInvalidPath) {
				t.Fatalf("Render = %q, %v; want ErrInvalidPath for a name of %d bytes", got, err, len(name))
			}
		case err != nil || !slices.Equal(got, []string{sea + "/" + name}):
			t.Fatalf("Render = %q, %v; want [%q]", got, err, sea+"/"+name)
		}
	})
}

func TestCompatible(t *testing.T) {
	t.Parallel()
	all := []domain.Mode{domain.ModeTwoWay, domain.ModeDownloadOnly, domain.ModeUploadOnly}
	upload := all[2:]
	for _, c := range []struct {
		tmpl string
		fits []domain.Mode
	}{
		{"{event_path}/{original_name}", all},
		{"{event_path}/photo {original_name}", all},
		// Render rejects this file below the event directory for each asset.
		{"{event_path}/{created.year}/{original_name}", all},
		{"{album}/{original_name}", upload},
		{"{event}/{original_name}", upload},
		{"{event_path}{original_name}", upload},
		{"x{event_path}/{original_name}", upload},
		{"{event_path|lower}/{original_name}", upload},
		{"photo", upload},
		{"", upload},
	} {
		for _, mode := range all {
			err := pathtemplate.Compatible(parse(t, c.tmpl), mode)
			if fits := slices.Contains(c.fits, mode); fits && err != nil || !fits && !errors.Is(err, pathtemplate.ErrMode) {
				t.Errorf("Compatible(%q, %s) = %v; want fits %v", c.tmpl, mode, err, fits)
			}
		}
	}
}
