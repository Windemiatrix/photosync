package eventtpl

import (
	"cmp"
	"fmt"
	"strings"
	"time"
)

// Fields are the date and the description of an event that a path laid out
// by a template gives: Match returns them, Render takes them. A number that
// the path does not give is 0. Only a path of every level gives the
// description, which may be "".
type Fields struct {
	Year, Month, Day int
	Description      string
}

// Date returns the day of f at 00:00 UTC, and false when f lacks the year,
// the month or the day. The time is of no use with false, and a day that the
// month does not have rolls over, as in time.Date.
func (f Fields) Date() (time.Time, bool) {
	return time.Date(f.Year, time.Month(f.Month), f.Day, 0, 0, 0, 0, time.UTC), f.Year != 0 && f.Month != 0 && f.Day != 0
}

// Render returns the levels that t writes for f, in order, up to the first
// level with a field that f lacks: a number that is 0, a year out of 1969 to
// 2068 for {YY}, the day of the week without the whole date, or a name in a
// language that Validate refuses; a level of text alone lacks nothing. An
// empty description leaves out the text right before it. Values are written
// as they are, not made fit for a directory. The result is nil for a date
// that does not exist.
func (t Template) Render(f Fields) []string {
	if !f.valid() {
		return nil
	}
	var out []string
	for _, parts := range t.Levels {
		var level, text string
		for _, p := range parts {
			s, ok := t.write(p, f)
			switch {
			case !ok:
				return out
			case p.Field == "":
				text = s
			case s == "" && fields[p.Field] == "Description":
				level = strings.TrimSuffix(level, text)
			}
			level += s
		}
		out = append(out, level)
	}
	return out
}

func (t Template) write(p Part, f Fields) (string, bool) {
	switch p.Field {
	case "":
		return p.Literal, true
	case "YYYY", "year":
		return fmt.Sprintf("%04d", f.Year), f.Year != 0
	case "YY":
		return fmt.Sprintf("%02d", f.Year%100), f.Year >= 1969 && f.Year <= 2068
	case "MM":
		return fmt.Sprintf("%02d", f.Month), f.Month != 0
	case "M":
		return fmt.Sprint(f.Month), f.Month != 0
	case "DD":
		return fmt.Sprintf("%02d", f.Day), f.Day != 0
	case "D":
		return fmt.Sprint(f.Day), f.Day != 0
	case "Description", "title":
		return f.Description, true
	case "dow":
		date, full := f.Date()
		if !full || t.lang.days == nil {
			return "", false
		}
		return t.lang.days[date.Weekday()], true
	}
	if f.Month == 0 || t.lang.months == nil {
		return "", false
	}
	return form(p.Field, t.lang.months[f.Month-1]), true
}

// A number that f lacks stands for any: 2000 has a February 29, and
// January 31 days.
func (f Fields) valid() bool {
	y, m, d := cmp.Or(f.Year, 2000), cmp.Or(f.Month, 1), cmp.Or(f.Day, 1)
	t := time.Date(y, time.Month(m), d, 0, 0, 0, 0, time.UTC)
	return t.Year() == y && int(t.Month()) == m && t.Day() == d
}
