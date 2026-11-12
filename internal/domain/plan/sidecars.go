package plan

import (
	"path"
	"slices"
	"strings"

	"github.com/Windemiatrix/go-libs/osxphotos/export"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// Sidecars returns, by link ID, the sidecars in scan, the scan of remote, of
// the links that have any: the files that go with the file of a link by the
// same action when it moves, goes to quarantine or leaves the cloud, and
// stay when it is copied. A sidecar is a file, not a directory, whose
// extension match.IsSidecar takes, in the root and the directory of the path
// of a link byte for byte and with the export.StemKey of its name. Only the
// active links of remote with the original or the source role have sidecars,
// and one beside two of them goes with neither. Each list is in the order of
// Path, and the inputs are not changed.
func Sidecars(links []domain.Link, remote string, scan []domain.RemoteEntry) map[domain.LinkID][]domain.RemoteEntry {
	mains := map[[3]string][]domain.LinkID{}
	for _, l := range links {
		if l.Remote == remote && objectFile(l) {
			k := sidecarKey(l.Root, l.Path)
			mains[k] = append(mains[k], l.ID)
		}
	}
	out := map[domain.LinkID][]domain.RemoteEntry{}
	for _, e := range slices.SortedFunc(slices.Values(scan), func(a, b domain.RemoteEntry) int { return strings.Compare(a.Path, b.Path) }) {
		if ids := mains[sidecarKey(e.Root, e.Path)]; len(ids) == 1 && !e.IsDir && match.IsSidecar(e.Path) {
			out[ids[0]] = append(out[ids[0]], e)
		}
	}
	return out
}

// match.Group puts the files of one shot together by the same key, so a
// sidecar goes with the file that matching found it beside.
func sidecarKey(root domain.LibraryKind, p string) [3]string {
	return [3]string{string(root), path.Dir(p), export.StemKey(path.Base(p))}
}

// SidecarPath returns the path that the sidecar at path sidecar takes beside
// the main file at path main: main with the extension of sidecar in place of
// its own, in the case that the cloud gave it, since Photos holds no name for
// a sidecar.
func SidecarPath(main, sidecar string) string {
	return strings.TrimSuffix(main, path.Ext(main)) + path.Ext(sidecar)
}
