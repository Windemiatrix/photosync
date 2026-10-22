package anon

import (
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Unicode normalization hooks, backed by golang.org/x/text/unicode/norm.

// NormalizationAvailable reports whether NFC and NFD are real normalizations.
const NormalizationAvailable = true

// nfc returns the NFC form of s.
func nfc(s string) string { return norm.NFC.String(s) }

// nfd returns the NFD form of s.
func nfd(s string) string { return norm.NFD.String(s) }

// decomposable reports whether the cluster c (NFC) has a different NFD form.
func decomposable(c string) bool { return norm.NFD.String(c) != c }

// lowerNFC returns the lower-case NFC form of s, rune by rune.
func lowerNFC(s string) string { return strings.ToLower(nfc(s)) }

// sortedKeys returns the keys of a set in sorted order.
func sortedKeys(l lookup) []string {
	out := make([]string, 0, len(l))
	for k := range l {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
