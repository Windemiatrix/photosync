package eventtpl_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
)

func tpl(tb testing.TB, side eventtpl.Side, names string, levels ...string) eventtpl.Template {
	tb.Helper()
	t, err := eventtpl.Parse(levels, side, names)
	if err != nil {
		tb.Fatalf("Parse(%q, %s, %q) error = %v; want a template", levels, side, names, err)
	}
	return t
}

// go vet wants the field names of a struct of another package, and a row
// must stay on one line.
func event(year, month, day int, description string) eventtpl.Fields {
	return eventtpl.Fields{Year: year, Month: month, Day: day, Description: description}
}

// The names of the directories and albums that a sync makes come from here,
// so each row pins one form of a field or one place where the levels stop,
// and the output must be in NFC, so that a table typed in another normal
// form fails.
func TestRender(t *testing.T) {
	t.Parallel()
	const photos, cloud = eventtpl.Photos, eventtpl.Cloud
	std := tpl(t, photos, "", "{YYYY}", "{MM}/{DD} {Description}")
	dirs := tpl(t, cloud, "", "{YYYY}", "{MM}-{DD} - {Description}")
	short := tpl(t, cloud, "", "Photos", "{year}", "{M}.{D} {title}")
	yy := tpl(t, cloud, "", "{YY}", "{MM}-{DD} {Description}")
	reversed := tpl(t, cloud, "", "{DD}", "{MM}", "{YYYY} {Description}")
	labeled := tpl(t, cloud, "", "Year {YYYY}", "{MM}-{DD} {Description}")
	weekFolder := tpl(t, photos, "en", "{YYYY} {dow}", "{MM}-{DD} {Description}")
	byName := func(names, spelling string) eventtpl.Template {
		return tpl(t, photos, names, "{YYYY}", "{DD} {"+spelling+"} {Description}")
	}
	weekday := func(names string) eventtpl.Template {
		return tpl(t, photos, names, "{YYYY}", "{MM}-{DD} {dow}, {Description}")
	}
	for _, c := range []struct {
		name string
		tmpl eventtpl.Template
		f    eventtpl.Fields
		want []string
	}{
		{"the default of Photos", std, event(2024, 5, 1, "Holidays"), []string{"2024", "05/01 Holidays"}},
		{"the default of the cloud", dirs, event(2024, 5, 1, "Holidays"), []string{"2024", "05-01 - Holidays"}},
		{"an empty description leaves out the text before it", dirs, event(2024, 5, 1, ""), []string{"2024", "05-01"}},
		{"only the text right before the description", weekday("en"), event(2024, 5, 1, ""), []string{"2024", "05-01 Wednesday"}},
		{"a folder: the levels that the fields fill", dirs, eventtpl.Fields{Year: 2024}, []string{"2024"}},
		{"a year and a month: the day is missing", dirs, eventtpl.Fields{Year: 2024, Month: 2}, []string{"2024"}},
		{"no fields, no levels", dirs, eventtpl.Fields{}, nil},
		{"a level of text alone lacks nothing", short, eventtpl.Fields{}, []string{"Photos"}},
		{"short spellings", short, event(2024, 5, 1, "x"), []string{"Photos", "2024", "5.1 x"}},
		{"short spellings of two digits", short, event(2024, 12, 31, "x"), []string{"Photos", "2024", "12.31 x"}},
		{"a month that the short spelling lacks", short, event(2024, 0, 1, "x"), []string{"Photos", "2024"}},
		{"a day that the short spelling lacks", short, event(2024, 5, 0, "x"), []string{"Photos", "2024"}},
		{"an empty description in the short spelling", short, event(2024, 5, 1, ""), []string{"Photos", "2024", "5.1"}},
		{"four digits of an early year", dirs, event(987, 5, 1, "x"), []string{"0987", "05-01 - x"}},
		{"{YY} from 1969", yy, event(1969, 5, 1, "x"), []string{"69", "05-01 x"}},
		{"{YY} to 2068", yy, event(2068, 5, 1, "x"), []string{"68", "05-01 x"}},
		{"{YY} with a leading zero", yy, event(2005, 5, 1, "x"), []string{"05", "05-01 x"}},
		{"{YY} before 1969", yy, event(1968, 5, 1, "x"), nil},
		{"{YY} after 2068", yy, event(2069, 5, 1, "x"), nil},
		{"a month by name", byName("ru", "Month"), event(2024, 5, 1, "x"), []string{"2024", "01 Май x"}},
		{"a month in lower case", byName("ru", "month"), event(2024, 1, 1, "x"), []string{"2024", "01 январь x"}},
		{"a short month of three letters", byName("ru", "mon"), event(2024, 5, 1, "x"), []string{"2024", "01 Май x"}},
		{"a short month", byName("ru", "mon"), event(2024, 9, 1, "x"), []string{"2024", "01 Сен x"}},
		{"a month in lower case in English", byName("en", "month"), event(2024, 9, 1, "x"), []string{"2024", "01 september x"}},
		{"a short month in English", byName("en", "mon"), event(2024, 9, 1, "x"), []string{"2024", "01 Sep x"}},
		{"a month by name that the fields lack", byName("en", "Month"), eventtpl.Fields{Year: 2024, Day: 1}, []string{"2024"}},
		{"the day of the week needs the whole date", weekFolder, eventtpl.Fields{Year: 2024}, nil},
		{"a month by name without a language", byName("", "Month"), event(2024, 5, 1, "x"), []string{"2024"}},
		{"a day of the week without a language", weekday(""), event(2024, 5, 1, "x"), []string{"2024"}},
		{"a language that Parse does not know", byName("fr", "Month"), event(2024, 5, 1, "x"), []string{"2024"}},
		{"a language in another case", byName("RU", "Month"), event(2024, 5, 1, "x"), []string{"2024"}},
		{"February 29 of a leap year", dirs, event(2024, 2, 29, "x"), []string{"2024", "02-29 - x"}},
		{"a date that does not exist", dirs, event(2023, 2, 29, "x"), nil},
		{"a month that does not exist", dirs, event(2024, 13, 1, "x"), nil},
		{"a day and a month without the year", reversed, eventtpl.Fields{Month: 2, Day: 29}, []string{"29", "02"}},
		{"a day that no February has", reversed, eventtpl.Fields{Month: 2, Day: 30}, nil},
		{"a day without the month", reversed, eventtpl.Fields{Day: 31}, []string{"31"}},
		{"the description as it is", dirs, event(2024, 5, 1, " A/B "), []string{"2024", "05-01 -  A/B "}},
		{"dots and control characters as they are", dirs, event(2024, 5, 1, "../..\x00\t"), []string{"2024", "05-01 - ../..\x00\t"}},
		{"text before a field", labeled, event(2024, 5, 1, "x"), []string{"Year 2024", "05-01 x"}},
	} {
		got := c.tmpl.Render(c.f)
		if !slices.Equal(got, c.want) {
			t.Errorf("%s: Render(%+v) = %q; want %q", c.name, c.f, got, c.want)
		}
		if !norm.NFC.IsNormalString(strings.Join(got, "/")) {
			t.Errorf("%s: Render(%+v) = %q; want NFC", c.name, c.f, got)
		}
	}
}

// English names must be those of package time, and Russian ones the
// nominative case as spelled here; the first days of the months of 2024 fall
// on every day of the week.
func TestNames(t *testing.T) {
	t.Parallel()
	ru := []string{
		"Январь", "Февраль", "Март", "Апрель", "Май", "Июнь", "Июль", "Август", "Сентябрь", "Октябрь", "Ноябрь", "Декабрь",
		"Воскресенье", "Понедельник", "Вторник", "Среда", "Четверг", "Пятница", "Суббота",
	}
	for _, names := range []string{"ru", "en"} {
		tmpl := tpl(t, eventtpl.Photos, names, "{YYYY}", "{DD} {Month} {dow}, {Description}")
		for m := time.January; m <= time.December; m++ {
			day := time.Date(2024, m, 1, 0, 0, 0, 0, time.UTC).Weekday()
			want := []string{"2024", "01 " + m.String() + " " + day.String() + ", x"}
			if names == "ru" {
				want[1] = "01 " + ru[m-1] + " " + ru[12+day] + ", x"
			}
			if got := tmpl.Render(event(2024, int(m), 1, "x")); !slices.Equal(got, want) {
				t.Errorf("%s: Render of %s 1 = %q; want %q", names, m, got, want)
			}
		}
	}
}

// Without the whole date an event would get a day that it does not have:
// time.Date rolls a 0 over to the month before.
func TestDate(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		f    eventtpl.Fields
		want time.Time
		ok   bool
	}{
		{"a date", event(2024, 5, 1, ""), time.Date(2024, 5, 1, 0, 0, 0, 0, time.UTC), true},
		{"no day", eventtpl.Fields{Year: 2024, Month: 5}, time.Time{}, false},
		{"no month", eventtpl.Fields{Year: 2024, Day: 1}, time.Time{}, false},
		{"no year", eventtpl.Fields{Month: 5, Day: 1}, time.Time{}, false},
	} {
		got, ok := c.f.Date()
		if ok != c.ok || ok && (!got.Equal(c.want) || got.Location() != time.UTC) {
			t.Errorf("%s: %+v.Date() = %v, %v; want %v, %v", c.name, c.f, got, ok, c.want, c.ok)
		}
	}
}
