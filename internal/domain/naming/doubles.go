package naming

import (
	"slices"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Doubles returns the groups of paths that the Windows client of the cloud
// takes for one file: equal in upper case, whatever the normal form. A group has two paths
// or more in byte order, the same path given twice included; the groups are
// in the byte order of their first paths; nil when there is none. paths is
// not changed.
func Doubles(paths []string) [][]string {
	byKey := map[string][]string{}
	for _, p := range slices.Sorted(slices.Values(paths)) {
		// Windows compares by simple upper case the bytes the cloud keeps,
		// which may be NFD, so the upper case is taken of the decomposed form:
		// a letter without a composed capital, such as "ẗ", still meets its
		// capital.
		k := norm.NFC.String(strings.ToUpper(norm.NFD.String(p)))
		byKey[k] = append(byKey[k], p)
	}
	var groups [][]string
	for _, g := range byKey {
		if len(g) > 1 {
			groups = append(groups, g)
		}
	}
	slices.SortFunc(groups, func(a, b []string) int { return strings.Compare(a[0], b[0]) })
	return groups
}
