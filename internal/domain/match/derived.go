package match

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// CompareCopies tells which of two copies of one shot is better, the
// original of a or f. More pixels, width times height, are better when both
// frames are known; then a RAW, by the extension of OriginalName or of Path,
// is better than another file; then more bytes, the size of the original
// resource of a against Size. The result is positive when the original is
// better, negative when f is, and zero when neither is.
func CompareCopies(a domain.Asset, f domain.RemoteEntry) int {
	var pixels int
	if photos, cloud := a.Width*a.Height, f.Meta.Width*f.Meta.Height; photos != 0 && cloud != 0 {
		pixels = cmp.Compare(photos, cloud)
	}
	var size int64
	if i := slices.IndexFunc(a.Resources, func(r domain.Resource) bool { return r.Role == domain.ResourceOriginal }); i >= 0 {
		size = a.Resources[i].Size
	}
	// A HEIC has fewer bytes than a JPEG made from it, so bytes come last.
	return cmp.Or(pixels, cmp.Compare(rawRank(a.OriginalName), rawRank(f.Path)), cmp.Compare(size, f.Size))
}

// cmp.Compare takes no bool.
func rawRank(name string) int {
	if kindOf(name) == raw {
		return 1
	}
	return 0
}
