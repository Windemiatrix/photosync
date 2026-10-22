package anon

import (
	"regexp"
	"strings"
	"time"
	"unicode"
)

// This file ports the small derivations of the reference manual-sync script
// that the relation check and the unit selection need. It does no matching.

// Splitext splits a name into a stem and an extension with the semantics of
// Python's os.path.splitext: the extension starts at the last dot of the last
// path element, and dots at the start of that element do not start one.
func Splitext(p string) (stem, ext string) {
	sep := strings.LastIndexByte(p, '/')
	dot := strings.LastIndexByte(p, '.')
	if dot > sep {
		for i := sep + 1; i < dot; i++ {
			if p[i] != '.' {
				return p[:dot], p[dot:]
			}
		}
	}
	return p, ""
}

// BaseName returns the last element of a "/"-separated cloud path.
func BaseName(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[i+1:]
	}
	return p
}

// FolderOf returns the folder of a "/"-separated cloud path ("" at the root).
func FolderOf(p string) string {
	if i := strings.LastIndexByte(p, '/'); i >= 0 {
		return p[:i]
	}
	return ""
}

// ExtLower returns the extension of a name in lower case, without the dot.
func ExtLower(name string) string {
	_, ext := Splitext(name)
	return strings.ToLower(strings.TrimPrefix(ext, "."))
}

// FolderInfo is the folder structure of a cloud path as the inventory
// derives it.
type FolderInfo struct {
	Folder     string
	Year       string
	Event      string
	EventMonth string
	EventDay   string
}

var (
	fourDigits = regexp.MustCompile(`^[0-9]{4}$`)
	// The derivations compared with the inventory use Python's \d: any
	// decimal digit. Such records are not supported (RuleNonASCIIDigit), so
	// the mapping and the rules keep ASCII digits.
	yearFolderRE = regexp.MustCompile(`^\p{Nd}{4}$`)
	eventDateRE  = regexp.MustCompile(`^(\p{Nd}{2})(?:-(\p{Nd}{2}))?`)
	eventTitleRE = regexp.MustCompile(`^([0-9]{2})(?:-([0-9]{2}))?(?:[\s\x{85}\pZ]*-[\s\x{85}\pZ]*|[\s\x{85}\pZ]+|$)(.*)$`)
	pxlRE        = regexp.MustCompile(`PXL_([0-9]{8})_([0-9]{6})`)
	suffixNRE    = regexp.MustCompile(` \(([0-9]+)\)$`)
)

// FolderInfoOf derives the folder structure of a cloud path: the year folder
// (exactly four digits), the event folder below it and the event month and
// day from the event name.
func FolderInfoOf(p string) FolderInfo {
	fi := FolderInfo{Folder: FolderOf(p)}
	if fi.Folder == "" {
		return fi
	}
	parts := strings.Split(fi.Folder, "/")
	if !yearFolderRE.MatchString(parts[0]) {
		return fi
	}
	fi.Year = parts[0]
	if len(parts) > 1 {
		fi.Event = parts[1]
	}
	if m := eventDateRE.FindStringSubmatch(fi.Event); m != nil {
		fi.EventMonth, fi.EventDay = m[1], m[2]
	}
	return fi
}

// KindOf returns the inventory kind of a file name.
func KindOf(name string) string {
	switch ExtLower(name) {
	case "jpg", "jpeg", "heic", "heif", "png", "gif", "tif", "tiff", "dng", "cr2", "cr3", "nef", "arw", "mov", "mp4", "m4v":
		return KindPhotosOK
	case "mpg", "mpeg", "vob", "3gp", "avi", "mts", "m2ts", "wmv", "mkv":
		return KindConvert
	case "xmp", "thm", "aae":
		return KindSidecar
	}
	return KindOther
}

// IsRawExt reports whether a lower-case extension is a RAW format of the
// reference script.
func IsRawExt(ext string) bool {
	switch ext {
	case "dng", "cr2", "cr3", "nef", "arw":
		return true
	}
	return false
}

const (
	layoutZoned = "2006-01-02T15:04:05.999999999Z07:00"
	layoutNaive = "2006-01-02T15:04:05.999999999"
)

// ParseDT parses an ISO 8601 date as the reference script does: an offset,
// when present, is dropped and the wall clock is kept (returned in UTC).
// ok is false for an empty or unparsable string.
func ParseDT(s string) (t time.Time, ok bool) {
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{layoutZoned, layoutNaive} {
		if v, err := time.Parse(layout, s); err == nil {
			return wall(v), true
		}
	}
	return time.Time{}, false
}

// wall returns the wall clock of t as a UTC time.
func wall(t time.Time) time.Time {
	y, mo, d := t.Date()
	h, mi, sec := t.Clock()
	return time.Date(y, mo, d, h, mi, sec, t.Nanosecond(), time.UTC)
}

// Fix1970 repeats the reference fix of an asset dated 1970: the name
// PXL_YYYYMMDD_HHMMSS holds UTC, moved to the asset time zone by tzoffset.
// fixed is false when the fix does not apply; invalid is true when the name
// holds an impossible date (the reference script would stop on it).
func Fix1970(name string, dt time.Time, tzoffset int) (t time.Time, fixed, invalid bool) {
	if dt.Year() != 1970 {
		return time.Time{}, false, false
	}
	m := pxlRE.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false, false
	}
	utc, err := time.Parse("20060102150405", m[1]+m[2])
	if err != nil {
		return time.Time{}, false, true
	}
	return utc.Add(time.Duration(tzoffset) * time.Second), true, false
}

// EventToTitle converts an event folder name to an album title:
// "01-03 - Party" -> "01/03 Party", "06 - Trip" -> "06 Trip", "08-07" -> "08/07".
func EventToTitle(event string) string {
	m := eventTitleRE.FindStringSubmatch(event)
	if m == nil {
		return event
	}
	head := m[1]
	if m[2] != "" {
		head += "/" + m[2]
	}
	return strings.TrimFunc(head+" "+strings.TrimFunc(m[3], unicode.IsSpace), unicode.IsSpace)
}

// AlbumKey is an album of the catalog: the folder path and the title.
type AlbumKey struct {
	Folders []string
	Title   string
}

// String returns a map key for the album.
func (k AlbumKey) String() string { return strings.Join(k.Folders, "\x00") + "\x01" + k.Title }

// FolderToAlbum converts a cloud folder to the album key the reference script
// expects for it; ok is false for the root and for a bare year folder. The
// special root of themed albums is not recognized: that segment is ordinary
// text for the tool.
func FolderToAlbum(folder string) (key AlbumKey, ok bool) {
	var parts []string
	for _, p := range strings.Split(folder, "/") {
		if p != "" {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return AlbumKey{}, false
	}
	if fourDigits.MatchString(parts[0]) {
		if len(parts) == 1 {
			return AlbumKey{}, false
		}
		title := append([]string{EventToTitle(parts[1])}, parts[2:]...)
		return AlbumKey{Folders: []string{parts[0]}, Title: strings.Join(title, " / ")}, true
	}
	return AlbumKey{Folders: []string{}, Title: strings.Join(parts, " / ")}, true
}

// SuffixN returns the trailing " (N)" of a stem, or "".
func SuffixN(stem string) string {
	if m := suffixNRE.FindStringIndex(stem); m != nil {
		return stem[m[0]:]
	}
	return ""
}

// NeighborKey is the neighbor key of a cloud file: its folder and its lower
// NFC stem without a trailing " (N)".
func NeighborKey(p string) string {
	stem, _ := Splitext(BaseName(p))
	s := lowerNFC(stem)
	s = strings.TrimSuffix(s, SuffixN(s))
	return FolderOf(p) + "\x00" + s
}
