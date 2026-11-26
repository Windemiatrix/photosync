package eventtpl_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
)

// The paths that a sync reads come from users, so each row pins one thing
// that makes a path an event or a folder, or one thing that does not; a path
// that fails gives the zero Fields.
func TestMatch(t *testing.T) {
	t.Parallel()
	const photos, cloud = eventtpl.Photos, eventtpl.Cloud
	std := tpl(t, photos, "", "{YYYY}", "{MM}/{DD} {Description}")
	dirs := tpl(t, cloud, "", "{YYYY}", "{MM}-{DD} - {Description}")
	short := tpl(t, cloud, "", "Photos", "{year}", "{M}.{D} {title}")
	middle := tpl(t, cloud, "", "{YYYY}", "Photos", "{MM}-{DD} {Description}")
	labeled := tpl(t, cloud, "", "Year {YYYY}", "{MM}-{DD} {Description}")
	yy := tpl(t, cloud, "", "{YY}", "{MM}-{DD} {Description}")
	reversed := tpl(t, cloud, "", "{DD}", "{MM}", "{YYYY} {Description}")
	ru := tpl(t, photos, "ru", "{YYYY}", "{DD} {Month} {Description}")
	lower := tpl(t, photos, "ru", "{YYYY}", "{DD} {month} {Description}")
	unnamed := tpl(t, photos, "", "{YYYY}", "{DD} {Month} {Description}")
	mon := tpl(t, photos, "en", "{YYYY}", "{DD} {mon} {Description}")
	weekday := tpl(t, photos, "en", "{YYYY}", "{MM}-{DD} {dow}, {Description}")
	weekFolder := tpl(t, photos, "en", "{YYYY} {dow}", "{MM}-{DD} {Description}")
	var none eventtpl.Fields
	for _, c := range []struct {
		name string
		tmpl eventtpl.Template
		path []string
		want eventtpl.Fields
		ok   bool
	}{
		{"the default of Photos", std, []string{"2024", "05/01 Holidays"}, event(2024, 5, 1, "Holidays"), true},
		{"the default of the cloud", dirs, []string{"2024", "05-01 - Holidays"}, event(2024, 5, 1, "Holidays"), true},
		{"an empty description without the text before it", dirs, []string{"2024", "05-01"}, event(2024, 5, 1, ""), true},
		{"the text before an empty description", dirs, []string{"2024", "05-01 - "}, none, false},
		{"a folder", dirs, []string{"2024"}, eventtpl.Fields{Year: 2024}, true},
		{"the root", dirs, nil, none, true},
		{"a path longer than the template", dirs, []string{"2024", "05-01 - Holidays", "Day 1"}, none, false},
		{"a directory that is not an event", dirs, []string{"2024", "Holidays"}, none, false},
		{"one digit for {MM}", dirs, []string{"2024", "5-01 - x"}, none, false},
		{"a year of three digits", dirs, []string{"024"}, none, false},
		{"a year of five digits", dirs, []string{"20245"}, none, false},
		{"the year 0000", dirs, []string{"0000"}, none, false},
		{"February 29 of a leap year", dirs, []string{"2024", "02-29 - x"}, event(2024, 2, 29, "x"), true},
		{"February 29 of another year", dirs, []string{"2023", "02-29 - x"}, none, false},
		{"a thirteenth month", dirs, []string{"2024", "13-01 - x"}, none, false},
		{"day 00", dirs, []string{"2024", "05-00 - x"}, none, false},
		{"short spellings and a level of text", short, []string{"Photos", "2024", "5.1 x"}, event(2024, 5, 1, "x"), true},
		{"a leading zero for {M}", short, []string{"Photos", "2024", "05.1 x"}, none, false},
		{"two digits for {M} and {D}", short, []string{"Photos", "2024", "12.31 x"}, event(2024, 12, 31, "x"), true},
		{"a folder below a level of text", short, []string{"Photos", "2024"}, eventtpl.Fields{Year: 2024}, true},
		{"a level of text alone", short, []string{"Photos"}, none, true},
		{"text in another case", short, []string{"PHOTOS", "2024"}, eventtpl.Fields{Year: 2024}, true},
		{"other text", short, []string{"Fotos", "2024"}, none, false},
		{"a folder above a level of text", middle, []string{"2024"}, eventtpl.Fields{Year: 2024}, true},
		{"text before a field in another case", labeled, []string{"YEAR 2024", "05-01 x"}, event(2024, 5, 1, "x"), true},
		{"{YY} from 1969", yy, []string{"69", "05-01 x"}, event(1969, 5, 1, "x"), true},
		{"{YY} to 2068", yy, []string{"68", "05-01 x"}, event(2068, 5, 1, "x"), true},
		{"four digits for {YY}", yy, []string{"1969", "05-01 x"}, none, false},
		{"a month by name in another case", ru, []string{"2024", "01 МАЙ x"}, event(2024, 5, 1, "x"), true},
		{"a month in the genitive", ru, []string{"2024", "01 Мая x"}, none, false},
		{"a short month in lower case", mon, []string{"2024", "30 sep x"}, event(2024, 9, 30, "x"), true},
		{"the last month of the table", mon, []string{"2024", "01 Dec x"}, event(2024, 12, 1, "x"), true},
		{"a month in lower case", lower, []string{"2024", "01 май x"}, event(2024, 5, 1, "x"), true},
		{"the last day of the week of the table", weekday, []string{"2024", "06-01 Saturday, x"}, event(2024, 6, 1, "x"), true},
		{"the day of the week of the date", weekday, []string{"2024", "05-01 Wednesday, x"}, event(2024, 5, 1, "x"), true},
		{"another day of the week", weekday, []string{"2024", "05-01 Thursday, x"}, none, false},
		{"the day of the week in lower case, no description", weekday, []string{"2024", "05-01 wednesday"}, event(2024, 5, 1, ""), true},
		{"the day of the week in a folder", weekFolder, []string{"2024 Wednesday"}, none, false},
		{"the day of the week above the date", weekFolder, []string{"2024 Wednesday", "05-01 x"}, event(2024, 5, 1, "x"), true},
		{"names without a language", unnamed, []string{"2024", "01 May x"}, none, false},
		{"a day and a month without the year", reversed, []string{"29", "02"}, eventtpl.Fields{Month: 2, Day: 29}, true},
		{"a day that no February has", reversed, []string{"30", "02"}, none, false},
		{"the description as it is", dirs, []string{"2024", "05-01 - - A/B "}, event(2024, 5, 1, "- A/B "), true},
		{"spaces around the description", dirs, []string{"2024", "05-01 -  A/B "}, event(2024, 5, 1, " A/B "), true},
	} {
		got, ok := c.tmpl.Match(c.path)
		if got != c.want || ok != c.ok {
			t.Errorf("%s: Match(%q) = %+v, %v; want %+v, %v", c.name, c.path, got, ok, c.want, c.ok)
		}
	}
}

// The rules of Parse must leave Match one way to read every level that
// Render writes, whatever the template and the description.
//
// covers: INV-19
func TestRenderMatch(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		tmpl, f := drawEvent(t)
		path := tmpl.Render(f)
		if got, ok := tmpl.Match(path); len(path) != len(tmpl.Levels) || !ok || got != f {
			t.Fatalf("Render(%+v) by %q = %q, read back as %+v, %v; want every level and the same fields", f, tmpl.Levels, path, got, ok)
		}
	})
}

// Templates are built, not filtered: a random one almost never keeps the
// rules of Parse. Text with a digit or the letters of a name stands wherever
// the rules allow it, to tempt the reading.
func drawEvent(t *rapid.T) (eventtpl.Template, eventtpl.Fields) {
	draw := func(label string, from ...string) string { return rapid.SampledFrom(from).Draw(t, label) }
	yes := func(label string) bool { return rapid.SampledFrom([]bool{false, true}).Draw(t, label) }
	side := rapid.SampledFrom([]eventtpl.Side{eventtpl.Photos, eventtpl.Cloud}).Draw(t, "side")
	names := draw("names", "ru", "en")
	year := draw("year", "YYYY", "year", "YY")
	order := []string{year, draw("month", "MM", "M", "month", "mon", "Month"), draw("day", "DD", "D")}
	for i := len(order) - 1; i > 0; i-- {
		j := rapid.IntRange(0, i).Draw(t, "swap")
		order[i], order[j] = order[j], order[i]
	}
	if yes("dow") {
		order = slices.Insert(order, rapid.IntRange(0, len(order)).Draw(t, "dow at"), "dow")
	}
	last := len(order) - 1
	if order[last] == "dow" {
		last--
	}
	pieces := []string{" ", "-", ".", ", ", " - ", "_", "x", "1", "г ", "Ма"}
	if side == eventtpl.Photos {
		pieces = append(pieces, "/")
	}
	text := func(after string) string {
		s := strings.Join(rapid.SliceOfN(rapid.SampledFrom(pieces), 1, 2).Draw(t, "text"), "")
		if slices.Contains([]string{"YYYY", "year", "YY", "MM", "M", "DD", "D"}, after) && s[0] >= '0' && s[0] <= '9' {
			s = "-" + s
		}
		return s
	}
	level := func(fields []string) string {
		var s string
		if yes("text first") {
			s = text("")
		}
		for i, f := range fields {
			if i > 0 {
				s += text(fields[i-1])
			}
			s += "{" + f + "}"
		}
		return s
	}
	var levels []string
	start := rapid.IntRange(0, last).Draw(t, "last level from")
	for i := 0; i < start; {
		n := rapid.IntRange(1, start-i).Draw(t, "fields of a level")
		s := level(order[i : i+n])
		if yes("text last") {
			s += text(order[i+n-1])
		}
		levels = append(levels, s)
		i += n
	}
	if yes("level of text") {
		levels = slices.Insert(levels, rapid.IntRange(0, len(levels)).Draw(t, "text level at"), "Photos")
	}
	tail := order[start:]
	levels = append(levels, level(tail)+text(tail[len(tail)-1])+"{"+draw("description", "Description", "title")+"}")
	tmpl, err := eventtpl.Parse(levels, side, names)
	if err != nil {
		t.Fatalf("Parse(%q, %s, %s) error = %v; want a template", levels, side, names, err)
	}
	lo, hi := 1, 9999
	if year == "YY" {
		lo, hi = 1969, 2068
	}
	y := rapid.IntRange(lo, hi).Draw(t, "Year")
	m := rapid.IntRange(1, 12).Draw(t, "Month")
	d := rapid.IntRange(1, time.Date(y, time.Month(m)+1, 0, 0, 0, 0, 0, time.UTC).Day()).Draw(t, "Day")
	return tmpl, event(y, m, d, rapid.StringOfN(rapid.Rune(), 0, 20, -1).Draw(t, "Description"))
}

// Directory and album names come from users: whatever they hold, Match must
// not panic, must give the zero Fields with false, and what it reads Render
// must write back, to be read the same.
func FuzzMatch(f *testing.F) {
	templates := []eventtpl.Template{
		tpl(f, eventtpl.Photos, "", "{YYYY}", "{MM}/{DD} {Description}"),
		tpl(f, eventtpl.Cloud, "", "{YYYY}", "{MM}-{DD} - {Description}"),
		tpl(f, eventtpl.Cloud, "", "Photos", "{year}", "{M}.{D} {title}"),
		tpl(f, eventtpl.Photos, "ru", "{YY} {Month}", "{DD} {dow}, {Description}"),
		tpl(f, eventtpl.Cloud, "en", "{DD}", "{mon}", "{YYYY} {Description}"),
	}
	for _, s := range []string{
		"2024\n05/01 Holidays", "2024\n05-01 - Holidays", "2024\n05-01", "Photos\n2024\n5.1 x", "24 Май\n01 Среда, x",
		"01\nSep\n2024 x", "2024", "", "\n", "2023\n02-29 - x",
		"2024\n05-01 - \xff", "\x00", "Photos\x00\n2024", "24 Май\n01 Среда, x", "Photoſ\n2024\n5.1 x", "01\nKep\n2024 x",
	} {
		for k := uint8(0); int(k) < len(templates); k++ {
			f.Add(s, k)
		}
	}
	f.Fuzz(func(t *testing.T, s string, k uint8) {
		tmpl := templates[int(k)%len(templates)]
		levels := strings.Split(s, "\n")
		got, ok := tmpl.Match(levels)
		back := tmpl.Render(got)
		again, twice := tmpl.Match(back)
		switch {
		case !ok && got != (eventtpl.Fields{}):
			t.Fatalf("Match(%q) = %+v, false; want the zero Fields", levels, got)
		case !ok:
		case len(back) < len(levels) || !slices.EqualFunc(back[:len(levels)], levels, strings.EqualFold):
			t.Fatalf("Match(%q) = %+v, but Render writes %q", levels, got, back)
		case !twice || again != got:
			t.Fatalf("Match(%q) = %+v, and Render writes %q, which reads as %+v, %v", levels, got, back, again, twice)
		}
	})
}
