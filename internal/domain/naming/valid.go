package naming

import (
	"strings"
	"unicode/utf8"
)

// IsValidPart reports whether SanitizePart leaves s unchanged, that is,
// whether s can be one level of a cloud path as it is.
func IsValidPart(s string) bool { return SanitizePart(s) == s }

// IsValidPath reports whether p, a cloud path relative to a remote root such
// as "2024/05-01 - Holidays/IMG_0001.HEIC", is valid parts joined by "/" and
// is at most 1024 characters long with the quarantine prefix
// ".photosync/quarantine/YYYY-MM-DD/" put in front of it.
func IsValidPath(p string) bool {
	if utf8.RuneCountInString(p) > maxPathRunes {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if !IsValidPart(part) {
			return false
		}
	}
	return true
}
