package eventtpl

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// Match returns the fields of levels, a path from the first level of t, and
// true when the first levels that Render writes for them are levels, without
// regard to case: numbers as Render writes them, of a date that exists, the
// day of the week of that date, text and names of months and days in any
// case, and no text right before an empty description. A path shorter than
// t, the empty one too, gives the fields of its levels; a longer one gives
// false. The fields are zero with false.
func (t Template) Match(levels []string) (Fields, bool) {
	if len(levels) > len(t.Levels) {
		return Fields{}, false
	}
	var f Fields
	for i, s := range levels {
		for _, p := range t.Levels[i] {
			s = t.read(p, s, &f)
		}
	}
	back := t.Render(f)
	if len(back) < len(levels) || !slices.EqualFunc(back[:len(levels)], levels, strings.EqualFold) {
		return Fields{}, false
	}
	return f, true
}

// A part takes what looks like its value, and Match then checks the whole
// path by Render, so that reading keeps no rules of its own.
func (t Template) read(p Part, s string, f *Fields) string {
	k := min(4, len(s)-len(strings.TrimLeft(s, "0123456789")))
	n, _ := strconv.Atoi(s[:k])
	switch p.Field {
	case "":
		rest, _ := cut(s, p.Literal)
		return rest
	case "YYYY", "year":
		f.Year = n
	case "YY":
		if y, err := time.Parse("06", s[:k]); err == nil {
			f.Year = y.Year()
		}
	case "MM", "M":
		f.Month = n
	case "DD", "D":
		f.Day = n
	case "Description", "title":
		f.Description = s
		return ""
	case "dow":
		_, rest := find(t.lang.days, p.Field, s)
		return rest
	default:
		i, rest := find(t.lang.months, p.Field, s)
		f.Month = i + 1
		return rest
	}
	return s[k:]
}

func find(names []string, field, s string) (int, string) {
	for i, name := range names {
		if rest, ok := cut(s, form(field, name)); ok {
			return i, rest
		}
	}
	return -1, s
}

func cut(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return s, false
	}
	return s[len(prefix):], true
}
