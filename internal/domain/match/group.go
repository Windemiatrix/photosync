package match

import (
	"cmp"
	"path"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/export"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Candidate is a cloud file that may hold an asset of the photo library, with
// the files of its directory and stem that go with it. Main is a photo or a
// video. Role is the role of the link of Main: domain.RoleSource for a video
// that Photos does not take, domain.RoleOriginal otherwise. Live is the video
// of a Live Photo and RAW the RAW of a RAW+JPEG pair, nil when there is none.
// Sidecars, in path order, are files that only stay in the cloud.
type Candidate struct {
	Main      domain.RemoteEntry
	Role      domain.LinkRole
	Live, RAW *domain.RemoteEntry
	Sidecars  []domain.RemoteEntry
}

// Group sorts the files of a scan out for matching. Files go together when
// they have the same Root, the same directory byte for byte and the same
// export.StemKey of their names, so "IMG_0001 (1).HEIC" has a stem of its
// own. A HEIC, HEIF or JPEG photo with at most one MOV and at most one RAW of
// its stem is one candidate; any other mix of media gives a candidate per
// file, and then sidecars follow only a lone one. unknown holds the files
// with an extension that is neither media nor sidecar. Directories and
// sidecars that follow no candidate are left out. Both results are in the
// order of Root, then Path byte by byte, and nil when empty; entries is not
// changed.
func Group(entries []domain.RemoteEntry) (candidates []Candidate, unknown []domain.RemoteEntry) {
	groups := map[groupKey][]domain.RemoteEntry{}
	for _, e := range entries {
		switch {
		case e.IsDir:
		case kindOf(e.Path) == unlisted:
			unknown = append(unknown, e)
		default:
			k := groupKey{e.Root, path.Dir(e.Path), export.StemKey(path.Base(e.Path))}
			groups[k] = append(groups[k], e)
		}
	}
	for _, files := range groups {
		candidates = append(candidates, fromStem(files)...)
	}
	slices.SortFunc(candidates, func(a, b Candidate) int { return byPlace(a.Main, b.Main) })
	slices.SortFunc(unknown, byPlace)
	return candidates, unknown
}

type groupKey struct {
	root      domain.LibraryKind
	dir, stem string
}

func byPlace(a, b domain.RemoteEntry) int {
	return cmp.Or(cmp.Compare(a.Root, b.Root), strings.Compare(a.Path, b.Path))
}

// Photos and the cameras give the files of one shot one stem; when a stem
// holds more, the names cannot tell the shots apart.
func fromStem(files []domain.RemoteEntry) []Candidate {
	byKind := map[kind][]domain.RemoteEntry{}
	for _, f := range files {
		k := kindOf(f.Path)
		byKind[k] = append(byKind[k], f)
	}
	sidecars, stills, movs, raws := byKind[sidecar], byKind[still], byKind[mov], byKind[raw]
	slices.SortFunc(sidecars, byPlace)
	if len(stills) == 1 && len(movs) <= 1 && len(raws) <= 1 && len(files) == 1+len(movs)+len(raws)+len(sidecars) {
		return []Candidate{{Main: stills[0], Role: domain.RoleOriginal, Live: first(movs), RAW: first(raws), Sidecars: sidecars}}
	}
	var cs []Candidate
	for _, f := range files {
		switch kindOf(f.Path) {
		case sidecar:
		case source:
			cs = append(cs, Candidate{Main: f, Role: domain.RoleSource})
		default:
			cs = append(cs, Candidate{Main: f, Role: domain.RoleOriginal})
		}
	}
	if len(cs) == 1 {
		cs[0].Sidecars = sidecars
	}
	return cs
}

func first(files []domain.RemoteEntry) *domain.RemoteEntry {
	if len(files) == 0 {
		return nil
	}
	return &files[0]
}
