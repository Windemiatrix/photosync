package pathtemplate

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

var (
	// ErrUnknownField is a field that neither osxphotos nor photosync has, or
	// {event_path} or {event} with more than its name or inside a field.
	ErrUnknownField = errors.New("pathtemplate: unknown field")
	// ErrInvalidPath is a template that gives no path, a path that is not a
	// valid cloud path, an empty event path or an asset without an original.
	ErrInvalidPath = errors.New("pathtemplate: invalid cloud path")
	// ErrMode is a template that does not fit the mode of a remote.
	ErrMode = errors.New("pathtemplate: the template does not fit the mode")
)

// EventPath returns the value of {event_path} for a file of asset placed in
// album: the directory of the pair of album among pairs, the pairs of one
// remote root, else the directory that layout.DirFor gives album with p,
// and for an album with a zero ID the day folder of asset that layout.DayDir
// gives.
// The result is relative to the remote root and never empty.
func EventPath(asset domain.Asset, album domain.Album, pairs []domain.AlbumPair, p domain.Policies) string {
	if album.ID == 0 {
		return layout.DayDir(asset)
	}
	if i := slices.IndexFunc(pairs, func(x domain.AlbumPair) bool { return x.AlbumID == album.ID }); i >= 0 {
		return pairs[i].DirPath
	}
	return layout.DirFor(album, p)
}

// Render returns the cloud paths of the file of asset, relative to the
// remote root, sorted and without repeats: one for each value of a field
// with several, such as {album}. {event_path} is eventPath and {event} its
// last level through naming.SanitizePart, each a field of its name alone
// outside other fields. fields gives the fields of osxphotos; every string,
// folder and album title of a value passes naming.SanitizePart, so that no
// value makes a level. A field without a value gives "_", as in osxphotos;
// {today} is now; {counter} starts anew in each call. A path ends with the
// extension of the original file of asset; its levels after eventPath
// satisfy naming.IsValidPart, and eventPath, which the cloud may hold
// already, stays as it is: the caller keeps it inside the remote root. A
// remote not in upload-only mode takes one path, of a file directly in
// eventPath. The errors are ErrUnknownField, ErrInvalidPath, ErrMode and
// those of template.Renderer.Render, wrapped.
func Render(t *template.Template, eventPath string, asset domain.Asset, fields template.Photo, mode domain.Mode, now time.Time) ([]string, error) {
	i := slices.IndexFunc(asset.Resources, func(r domain.Resource) bool { return r.Role == domain.ResourceOriginal })
	if i < 0 {
		return nil, fmt.Errorf("%w: asset %s has no original file", ErrInvalidPath, asset.Key.UUID)
	}
	values, unknown, err := new(template.Renderer).Render(withEvents(t, eventPath), sanitized{fields}, template.RenderOptions{Today: now})
	switch {
	case err != nil:
		return nil, fmt.Errorf("pathtemplate: %w", err)
	case len(unknown) > 0:
		return nil, fmt.Errorf("%w: %s", ErrUnknownField, strings.Join(unknown, ", "))
	}
	paths := make([]string, len(values))
	for j, v := range values {
		paths[j] = v + path.Ext(asset.Resources[i].Filename)
		if eventPath == "" || !naming.IsValidPath(strings.TrimPrefix(paths[j], eventPath+"/")) {
			return nil, fmt.Errorf("%w: %q", ErrInvalidPath, paths[j])
		}
	}
	slices.Sort(paths)
	paths = slices.Compact(paths)
	switch {
	case len(paths) == 0:
		return nil, fmt.Errorf("%w: the template gives no path", ErrInvalidPath)
	case mode != domain.ModeUploadOnly && (len(paths) > 1 || path.Dir(paths[0]) != eventPath):
		return nil, fmt.Errorf("%w: a %s remote takes one file in %q, not %q", ErrMode, mode, eventPath, paths)
	}
	return paths, nil
}

// The library knows only the fields of osxphotos, so the two of photosync
// become text before it renders; t stays as it is for the next asset.
func withEvents(t *template.Template, eventPath string) *template.Template {
	values := map[string]string{"event_path": eventPath, "event": naming.SanitizePart(path.Base(eventPath))}
	out := &template.Template{Segments: slices.Clone(t.Segments)}
	for i, s := range out.Segments {
		if v, ok := values[plainName(s.Field)]; ok {
			out.Segments[i] = template.Segment{Pre: s.Pre + v + s.Post}
		}
	}
	return out
}

// A field with more than its name stays a field, for the library to report
// as unknown; DeepEqual also sees a part that the language may gain later.
func plainName(f *template.Field) string {
	if f == nil || !reflect.DeepEqual(*f, template.Field{Name: f.Name}) {
		return ""
	}
	return f.Name
}

// Compatible returns nil when t fits a remote in mode, else ErrMode. A
// two-way or download-only remote maps its directories back to albums, so
// its template starts with {event_path}/; Render checks the rest for each
// asset. An upload-only remote takes any template.
func Compatible(t *template.Template, mode domain.Mode) error {
	if mode == domain.ModeUploadOnly || len(t.Segments) > 0 && t.Segments[0].Pre == "" &&
		plainName(t.Segments[0].Field) == "event_path" && strings.HasPrefix(t.Segments[0].Post, "/") {
		return nil
	}
	return fmt.Errorf("%w: a %s remote needs a template that starts with {event_path}/", ErrMode, mode)
}

// Photos takes names that the cloud refuses, and a "/" in a value would
// make a level of the path.
type sanitized struct{ template.Photo }

func (s sanitized) Field(name string) (template.Value, bool) {
	v, ok := s.Photo.Field(name)
	v.Strings = levels(v.Strings)
	albums := make([]template.AlbumRef, len(v.Albums))
	for i, a := range v.Albums {
		albums[i] = template.AlbumRef{Folders: levels(a.Folders), Title: naming.SanitizePart(a.Title), Index: a.Index}
	}
	v.Albums = albums
	return v, ok
}

func levels(names []string) []string {
	out := make([]string, len(names))
	for i, n := range names {
		out[i] = naming.SanitizePart(n)
	}
	return out
}
