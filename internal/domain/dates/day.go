package dates

import "time"

// LocalDay returns the date of t at the place it was taken; offset is the UTC
// offset of that place in seconds east of UTC. A nil offset means t is a wall
// clock of an unknown zone: the date is the one t shows in time.UTC.
func LocalDay(t time.Time, offset *int) (year int, month time.Month, day int) {
	if offset == nil {
		return t.UTC().Date()
	}
	return t.In(time.FixedZone("", *offset)).Date()
}
