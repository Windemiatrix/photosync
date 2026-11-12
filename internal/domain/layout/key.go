package layout

import (
	"fmt"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Key returns the key of a, an album or a folder of a snapshot, by which
// the state knows it from one run to the next, and the source of the key:
// a.Key with a.KeySource when a.Key is not empty, else a path key with
// domain.KeyPath. A path key is the folder titles, outermost first, and
// the title, each quoted with ASCII escapes, as in ["2024"] "Trips". It
// changes when the album is renamed or moved, and albums and folders with
// one path share it. The state stores keys, so the form of a path key does
// not change.
func Key(a domain.Album) (key string, source domain.KeySource) {
	if a.Key != "" {
		return a.Key, a.KeySource
	}
	// Quoting keeps a "/" in a title from passing for a level; ASCII
	// escapes, unlike %q, keep the key the same when a new Go release
	// counts more characters as printable.
	return fmt.Sprintf("%+q %+q", a.FolderTitles, a.Title), domain.KeyPath
}
