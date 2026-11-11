package dates

import "time"

// A date without its fraction of a second is up to a second off, and a copy
// may round it the other way. UTC offsets are whole quarter-hours (+5:45,
// +12:45) from -12:00 to +14:00, so a wall clock of an unknown zone is at most
// 14 hours from its moment.
const (
	tolerance   = 2 * time.Second
	quarterHour = 15 * time.Minute
	maxShift    = 14 * time.Hour
)

// Same reports whether a and b are the same moment within 2 seconds either
// way. It compares instants: the Locations of a and b do not matter.
func Same(a, b time.Time) bool {
	return a.Sub(b).Abs() <= tolerance
}

// QuarterHourShift reports whether a and b are the same moment up to a time
// zone: a is b plus k quarter-hours within 2 seconds either way, and |k| is at
// most 56 (14 hours). When a is a wall clock of an unknown zone and b is its
// moment, k quarter-hours is the UTC offset of the place. k is 0 when ok is
// false; ok with k == 0 is exactly Same(a, b).
func QuarterHourShift(a, b time.Time) (k int, ok bool) {
	d := a.Sub(b)
	shift := d.Round(quarterHour)
	if shift.Abs() > maxShift || (d-shift).Abs() > tolerance {
		return 0, false
	}
	return int(shift / quarterHour), true
}
