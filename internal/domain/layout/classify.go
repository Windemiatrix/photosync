package layout

import (
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Classify returns the kind of an album of a snapshot: an album anywhere in a
// top-level folder titled with four ASCII digits is an event, any other album
// is thematic, and a folder, the only kind a snapshot sets, stays a folder.
func Classify(a domain.Album) domain.AlbumKind {
	switch {
	case a.Kind == domain.KindFolder:
		return domain.KindFolder
	case len(a.FolderTitles) > 0 && isYear(a.FolderTitles[0]):
		return domain.KindEvent
	}
	return domain.KindThematic
}

func isYear(title string) bool {
	return len(title) == 4 && strings.Trim(title, "0123456789") == ""
}
