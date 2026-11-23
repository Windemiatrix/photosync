package layout

import "github.com/Windemiatrix/photosync/internal/domain"

// Classify returns the kind of an album of a snapshot: a folder, the only
// kind a snapshot sets, stays a folder, an album to which DirFor gives a
// directory with p is an event, and any other album is thematic, which a
// sync leaves out.
func Classify(a domain.Album, p domain.Policies) domain.AlbumKind {
	switch {
	case a.Kind == domain.KindFolder:
		return domain.KindFolder
	case DirFor(a, p) != "":
		return domain.KindEvent
	}
	return domain.KindThematic
}
