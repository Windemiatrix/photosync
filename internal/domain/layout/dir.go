package layout

import (
	"slices"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

// DirFor returns the directory, relative to a remote root, that a new pair
// gives to a, an album or a folder of a snapshot, or "" when it gives none.
// p.Templates.Photos reads a date and a description from the folder titles
// and the title, and p.Templates.Cloud writes the levels of the directory
// for them: all its levels for an album that the photos template reads
// whole, the levels above an event for a folder that it reads as such.
// Either template stands for a path of the other only at the depth at which
// it writes that path, and only when it reads its own writing back as the
// same date and description. Each level passes naming.SanitizePart whole,
// and there is no directory when AlbumFor does not read the result back;
// the path may still be too long for naming.IsValidPath. A pair keeps its
// directory once made. p.Templates are as eventtpl.Validate takes them.
func DirFor(a domain.Album, p domain.Policies) string {
	photos, cloud := parse(p.Templates)
	titles := slices.Concat(a.FolderTitles, []string{a.Title})
	levels, ok := cross(photos, cloud, titles)
	if !ok || (a.Kind == domain.KindFolder) == (len(titles) == len(photos.Levels)) {
		return ""
	}
	for i, level := range levels {
		levels[i] = naming.SanitizePart(level)
	}
	// A later run reads the directory by AlbumFor, and sanitizing can take
	// what it reads, as a description of control characters.
	if _, ok := cross(cloud, photos, levels); !ok {
		return ""
	}
	return strings.Join(levels, "/")
}

// AlbumFor returns the album or the folder that a new pair gives to dir, a
// directory relative to a remote root, as DirFor maps them the other way: an
// album for a directory that p.Templates.Cloud reads whole, a folder for one
// that it reads as the levels above an event, titled with the levels that
// p.Templates.Photos writes, the description as dir holds it. ok is false
// for any other directory, the root and one below an event among them. Only
// FolderTitles, Title and, for a folder, Kind are set, as in a snapshot.
func AlbumFor(dir string, p domain.Policies) (domain.Album, bool) {
	photos, cloud := parse(p.Templates)
	levels := strings.Split(dir, "/")
	titles, ok := cross(cloud, photos, levels)
	if !ok {
		return domain.Album{}, false
	}
	a := domain.Album{FolderTitles: titles[:len(titles)-1], Title: titles[len(titles)-1]}
	if len(levels) < len(cloud.Levels) {
		a.Kind = domain.KindFolder
	}
	return a, true
}

// A path stands for one of the other side only where both sides read the
// same date and description from it and write them at its depth: otherwise
// two directories would give one folder, and a rename would follow either.
func cross(from, to eventtpl.Template, levels []string) ([]string, bool) {
	f, ok := from.Match(levels)
	out := to.Render(f)
	back, _ := to.Match(out)
	return out, ok && len(out) > 0 && back == f && len(from.Render(f)) == len(levels)
}

// Build refuses the templates that eventtpl.Validate refuses before any
// layout; a template that Parse refuses is the zero one, which reads nothing.
func parse(ts domain.Templates) (photos, cloud eventtpl.Template) {
	photos, _ = eventtpl.Parse(ts.Photos, eventtpl.Photos, ts.Names)
	cloud, _ = eventtpl.Parse(ts.Cloud, eventtpl.Cloud, ts.Names)
	return photos, cloud
}
