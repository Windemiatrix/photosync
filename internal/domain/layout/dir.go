package layout

import (
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/dates"
	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

// AlbumFor must not take the day folder that DayDir names for an album.
const dayLayout = "2006/01-02"

// DirFor returns the directory, relative to a remote root, that a new pair
// gives to a, an album or a folder of a snapshot. A top-level folder titled
// with four ASCII digits and all inside it keep their path from the year
// down; any other album or folder goes under p.AlbumsRoot. Each folder
// title, and the title of a folder, is one level as naming.SanitizePart
// gives it; the title of an album gives the levels of naming.EventDir with
// p.NestedSeparator and p.UntitledEvent. Every level satisfies
// naming.IsValidPart; the path may still be too long for naming.IsValidPath.
// A pair keeps its directory once made.
func DirFor(a domain.Album, p domain.Policies) string {
	var levels []string
	if !inYearFolder(a) {
		levels = append(levels, naming.SanitizePart(p.AlbumsRoot))
	}
	for _, t := range a.FolderTitles {
		levels = append(levels, naming.SanitizePart(t))
	}
	if a.Kind == domain.KindFolder {
		levels = append(levels, naming.SanitizePart(a.Title))
	} else {
		levels = append(levels, naming.EventDir(a.Title, p.NestedSeparator, p.UntitledEvent)...)
	}
	return strings.Join(levels, "/")
}

// A year folder counts as in itself, so that its directory is the year.
func inYearFolder(a domain.Album) bool {
	if len(a.FolderTitles) > 0 {
		return isYear(a.FolderTitles[0])
	}
	return a.Kind == domain.KindFolder && isYear(a.Title)
}

// AlbumFor returns the album or folder that a new pair gives to dir, a
// directory relative to a remote root. albumDirs holds the directories that
// stand for albums: those with media files of their own and those paired
// with an album. ok is false for the root, p.AlbumsRoot and a day folder
// "YYYY/MM-DD" of a real date: their files join no album. A year folder is
// a folder. Below it, below p.AlbumsRoot or from the top level, the topmost
// directory in albumDirs on the way to dir is an album, those above it are
// folders and those below it are parts of its title, joined with
// p.NestedSeparator; with none on the way, dir is a folder. A directory
// below a day folder is a part of the untitled event of that day. Names are
// kept, except that each part of a title passes naming.EventTitle with
// p.UntitledEvent. Only FolderTitles, Title and, for a folder, Kind are set,
// as in a snapshot.
func AlbumFor(dir string, albumDirs map[string]bool, p domain.Policies) (a domain.Album, ok bool) {
	levels := strings.Split(dir, "/")
	root, first := 0, 0
	switch {
	case dir == "" || dir == ".":
		return domain.Album{}, false
	case levels[0] == naming.SanitizePart(p.AlbumsRoot):
		root, first = 1, 1
	case len(levels) > 1 && isDay(levels[0], levels[1]):
		if len(levels) == 2 {
			return domain.Album{}, false
		}
		// A subdirectory of a day folder belongs to the untitled event of
		// that day, whose directory names it with the untitled name.
		levels[1] += " - " + p.UntitledEvent
		return titled(levels, 0, 1, p), true
	case isYear(levels[0]):
		first = 1
	}
	for i := first; i < len(levels); i++ {
		if albumDirs[strings.Join(levels[:i+1], "/")] {
			return titled(levels, root, i, p), true
		}
	}
	if len(levels) == root {
		return domain.Album{}, false
	}
	return domain.Album{Kind: domain.KindFolder, FolderTitles: levels[root : len(levels)-1], Title: levels[len(levels)-1]}, true
}

func titled(levels []string, root, at int, p domain.Policies) domain.Album {
	return domain.Album{FolderTitles: levels[root:at], Title: naming.EventTitle(levels[at:], p.NestedSeparator, p.UntitledEvent)}
}

// The year is the folder's: 02-29 is a day only in a leap year.
func isDay(year, level string) bool {
	_, err := time.Parse(dayLayout, year+"/"+level)
	return err == nil
}

// DayDir returns the day folder of asset: "YYYY/MM-DD" of the date that
// dates.LocalDay gives. It holds the file of an asset in no album.
func DayDir(asset domain.Asset) string {
	year, month, day := dates.LocalDay(asset.Taken, asset.TZOffset)
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC).Format(dayLayout)
}
