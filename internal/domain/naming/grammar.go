package naming

import (
	"strings"
	"time"
)

// EventDir returns the directory levels of an album title. The title is
// split at each sep, unless sep is empty, and each part gives one level. A
// part "MM/DD Name", where MM/DD is a day of a leap year, becomes
// "MM-DD - Name"; a part "MM/DD" becomes "MM-DD - " followed by untitled, so
// that it is not taken for the folder of that day. Every level satisfies
// IsValidPart: it is sanitized and cut as a whole.
func EventDir(title, sep, untitled string) []string {
	parts := []string{title}
	if sep != "" {
		parts = strings.Split(title, sep)
	}
	dirs := make([]string, len(parts))
	for i, p := range parts {
		dirs[i] = levelDir(p, untitled)
	}
	return dirs
}

// EventTitle returns the album title of directory levels, joined with sep. A
// level "MM-DD - Name" gives "MM/DD Name", or "MM/DD" when Name is untitled;
// any other level, "MM-DD" too, is used as it is: a name read from the cloud
// is not decoded.
func EventTitle(dirs []string, sep, untitled string) string {
	titles := make([]string, len(dirs))
	for i, d := range dirs {
		titles[i] = levelTitle(d, untitled)
	}
	return strings.Join(titles, sep)
}

// A bare day gets a name: "MM-DD" is the folder of the day, not an album.
func levelDir(part, untitled string) string {
	day, name, ok := cutDay(part, "01/02", " ")
	if !ok {
		return SanitizePart(part)
	}
	if name == "" {
		name = untitled
	}
	return SanitizePart(day.Format("01-02") + " - " + name)
}

// Only the day is decoded: mapping a look-alike back would make the
// character the cloud refuses.
func levelTitle(dir, untitled string) string {
	day, name, ok := cutDay(dir, "01-02", " - ")
	switch {
	case !ok || name == "":
		return dir
	case name == untitled:
		return day.Format("01/02")
	}
	return day.Format("01/02") + " " + name
}

// The year of an event is in its folder, which a level does not see, so any
// day of a leap year is a day.
func cutDay(s, layout, sep string) (day time.Time, name string, ok bool) {
	if len(s) < len(layout) {
		return time.Time{}, "", false
	}
	day, err := time.Parse("2006 "+layout, "2000 "+s[:len(layout)])
	if err != nil {
		return time.Time{}, "", false
	}
	rest := s[len(layout):]
	if rest == "" {
		return day, "", true
	}
	name, ok = strings.CutPrefix(rest, sep)
	return day, name, ok && name != ""
}
