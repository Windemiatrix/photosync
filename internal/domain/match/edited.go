package match

import (
	"path"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/go-libs/osxphotos/export"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Edited sets the edits of a scan apart from the other entries. A file is an
// edit when a directory on its path, at any depth, has the name of an element
// of editedDirs, both taken in NFC and lower case, or when its
// export.StemKey, without one " (N)" of ASCII digits at the end, ends with
// "_edited", as osxphotos export names an edited photo and numbers a second
// one. Its extension does not matter; an element of editedDirs that holds a
// slash matches nothing. Directories are kept. Both results are in the order
// of Root, then Path byte by byte, and nil when empty; entries is not
// changed.
func Edited(entries []domain.RemoteEntry, editedDirs []string) (kept, edited []domain.RemoteEntry) {
	dirs := map[string]bool{}
	for _, d := range editedDirs {
		dirs[fold(d)] = true
	}
	for _, e := range entries {
		parts := strings.Split(e.Path, "/")
		inDir := slices.ContainsFunc(parts[:len(parts)-1], func(p string) bool { return dirs[fold(p)] })
		if !e.IsDir && (inDir || strings.HasSuffix(nameKey(path.Base(e.Path)), "_edited")) {
			edited = append(edited, e)
		} else {
			kept = append(kept, e)
		}
	}
	slices.SortFunc(kept, byPlace)
	slices.SortFunc(edited, byPlace)
	return kept, edited
}

// osxphotos export, and photosync after it, number the second file of one
// name in a directory with " (N)", so the number is no part of the name.
func nameKey(name string) string {
	return countSuffix.ReplaceAllLiteralString(export.StemKey(name), "")
}

var countSuffix = regexp.MustCompile(` \([0-9]+\)$`)

// A Mac writes names in NFD, and a user types a setting in either case.
func fold(s string) string {
	return strings.ToLower(norm.NFC.String(s))
}
