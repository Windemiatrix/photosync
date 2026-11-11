package naming

import (
	"path"

	"github.com/Windemiatrix/go-libs/osxphotos/export"
)

// Free returns name as a level that no file of a directory takes. name goes
// through SanitizePart first. taken holds export.StemKey of every file of the
// directory, photosync's or not; a nil taken is an empty directory. When the
// stem of name is taken, " (N)" goes before the extension with the first free
// N, as osxphotos export numbers a name; when the number makes the name longer
// than 255 bytes, the stem is cut and N does not go back, and a name whose
// stem cannot give up the bytes becomes "_ (N)". The result satisfies
// IsValidPart and its export.StemKey is not in taken. The caller frees the
// names of one directory in an order of its own and adds the key of each
// result to taken before the next.
func Free(name string, taken map[string]bool) string {
	name = SanitizePart(name)
	got, n := export.IncrementName(name, 0, taken)
	for len(got) > maxNameBytes {
		name = shortenStem(name, len(got)-maxNameBytes)
		got, n = export.IncrementName(name, n, taken)
	}
	return got
}

// The extension stays so that the file keeps its type. A valid name neither
// starts nor ends with a dot, so path.Ext splits it where osxphotos does. A
// stem that cannot give up the bytes is replaced: an empty one would leave a
// name that starts with a dot, which the scan of the cloud skips.
func shortenStem(name string, over int) string {
	ext := path.Ext(name)
	stem := name[:len(name)-len(ext)]
	if stem = cut(stem, max(len(stem)-over, 0)); stem == "" {
		return "_"
	}
	return stem + ext
}
