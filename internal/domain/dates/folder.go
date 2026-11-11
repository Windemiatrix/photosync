package dates

import (
	"regexp"
	"time"
)

var folderRE = regexp.MustCompile(`^(\d{4})/(\d{2})(-\d{2})?(?:[ /]|$)`)

// ParseFolder returns the date of a folder at 12:00 of the wall clock:
// "YYYY/MM-DD…" gives that day, "YYYY/MM…" the first of the month. dir is a
// "/"-separated folder relative to a target root, not a file path; the date
// must end the second element or be followed by a space, and deeper elements
// share its date. ok is false for any other folder and for a date that does
// not exist.
func ParseFolder(dir string) (t time.Time, ok bool) {
	m := folderRE.FindStringSubmatch(dir)
	if m == nil {
		return time.Time{}, false
	}
	day := m[3]
	if day == "" {
		day = "-01"
	}
	t, err := time.Parse("2006/01-02", m[1]+"/"+m[2]+day)
	if err != nil {
		return time.Time{}, false
	}
	// Noon keeps the day in UTC for every zone from -11:00 to +12:00, so the
	// import can give the wall clock a zone later.
	return t.Add(12 * time.Hour), true
}
