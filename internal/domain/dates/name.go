package dates

import (
	"regexp"
	"strings"
	"time"
)

// Parsed is a shooting date read from a file name.
type Parsed struct {
	Time time.Time // in time.UTC
	// Offset is the UTC offset of Time in seconds: 0 when the name holds UTC
	// (Pixel), nil when Time is the wall clock of an unknown zone. It never
	// tells the zone the shot was taken in.
	Offset *int
}

// Every expression is anchored at the start: a false date is worse than a
// missed one. Pixel cameras write UTC into the name. The digits are checked by
// the expression (time.Parse accepts one-digit hours), the calendar by
// time.Parse (time.Date would silently turn 31 February into March).
var nameFormats = []struct {
	re     *regexp.Regexp
	layout string
	utc    bool
}{
	{regexp.MustCompile(`^PXL_(\d{8}_\d{6})(\d{3})(?:\D|$)`), "20060102_150405.000", true},
	{regexp.MustCompile(`^(?:IMG|VID)_(\d{8}_\d{6})(?:\D|$)`), "20060102_150405", false},
	{regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}\.\d{2}\.\d{2})(?:\D|$)`), "2006-01-02 15.04.05", false},
	{regexp.MustCompile(`^(\d{8}_\d{6})(?:\D|$)`), "20060102_150405", false},
}

// ParseName returns the shooting date that a file name starts with. It knows
// PXL_YYYYMMDD_HHMMSSmmm (UTC, with milliseconds), IMG_YYYYMMDD_HHMMSS,
// VID_YYYYMMDD_HHMMSS, "YYYY-MM-DD HH.MM.SS" and YYYYMMDD_HHMMSS, with the
// prefixes in upper case; the digits must be followed by a non-digit or the
// end of the name. ok is false for any other name and for a date or time that
// does not exist. Each call returns its own Offset.
func ParseName(name string) (p Parsed, ok bool) {
	for _, f := range nameFormats {
		m := f.re.FindStringSubmatch(name)
		if m == nil {
			continue
		}
		// time.Parse reads a fraction of a second only after a separator.
		t, err := time.Parse(f.layout, strings.Join(m[1:], "."))
		if err != nil {
			continue
		}
		p = Parsed{Time: t}
		if f.utc {
			p.Offset = new(int)
		}
		return p, true
	}
	return Parsed{}, false
}
