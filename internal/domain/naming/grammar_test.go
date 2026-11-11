package naming_test

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
)

const (
	eventSep      = " / "
	eventUntitled = "Untitled"
)

var eventEncodedDay = regexp.MustCompile(`^\d\d-\d\d - `)

func TestEventDir(t *testing.T) {
	t.Parallel()

	a := func(n int) string { return strings.Repeat("a", n) }
	tests := []struct {
		title string
		want  []string
	}{
		{"05/01 Holidays", []string{"05-01 - Holidays"}},
		{"12/31 New Year", []string{"12-31 - New Year"}},
		{"02/29 Leap day", []string{"02-29 - Leap day"}},
		{"01/01 2024", []string{"01-01 - 2024"}},
		{"05/01 Day - 2", []string{"05-01 - Day - 2"}},

		{"05/01", []string{"05-01 - Untitled"}},
		{"05/01 Untitled", []string{"05-01 - Untitled"}},

		{"Holidays", []string{"Holidays"}},
		{"Отпуск", []string{"Отпуск"}},
		{"06 - Sea", []string{"06 - Sea"}},
		{"09-24 Flight home", []string{"09-24 Flight home"}},
		{"05-01 - Holidays", []string{"05-01 - Holidays"}},
		{"Trip - 2024", []string{"Trip - 2024"}},
		{"1-2 - 3", []string{"1-2 - 3"}},

		{"13/01 X", []string{"13∕01 X"}},
		{"00/10 X", []string{"00∕10 X"}},
		{"05/00 X", []string{"05∕00 X"}},
		{"05/32 X", []string{"05∕32 X"}},
		{"02/30 X", []string{"02∕30 X"}},
		{"04/31 X", []string{"04∕31 X"}},
		{"5/01 X", []string{"5∕01 X"}},
		{"05/1 X", []string{"05∕1 X"}},
		{"05/01X", []string{"05∕01X"}},
		{"05/01 ", []string{"05∕01 "}},

		{"05/01 a/b", []string{"05-01 - a∕b"}},
		{"05/01 a:b?", []string{"05-01 - a꞉b？"}},
		{"05/01 .", []string{"05-01 - ．"}},
		{"05/01 .hidden", []string{"05-01 - .hidden"}},
		{"05/01 CON", []string{"05-01 - CON"}},
		{"05/01 a\x00b", []string{"05-01 - ab"}},
		{"05/01 й", []string{"05-01 - й"}},
		{"05/01 " + a(247), []string{"05-01 - " + a(247)}},
		{"05/01 " + a(248), []string{"05-01 - " + a(247)}},

		{"05/01 Holidays / raw", []string{"05-01 - Holidays", "raw"}},
		{"05/01 Holidays / Picks / Dance 2", []string{"05-01 - Holidays", "Picks", "Dance 2"}},
		{"05/01 Trip / 05/02 Day two", []string{"05-01 - Trip", "05-02 - Day two"}},
		{"05/01 / raw", []string{"05-01 - Untitled", "raw"}},
		{"Trips / Kazan", []string{"Trips", "Kazan"}},
		{"a/b / c", []string{"a∕b", "c"}},

		{"A / ", []string{"A", "_"}},
		{" / ", []string{"_", "_"}},
		{"", []string{"_"}},
	}
	for _, tt := range tests {
		got := naming.EventDir(tt.title, eventSep, eventUntitled)
		if !slices.Equal(got, tt.want) {
			t.Errorf("EventDir(%q) = %q, want %q", tt.title, got, tt.want)
		}
		for _, level := range got {
			if !validName(level) {
				t.Errorf("EventDir(%q) = %q, level %q is not a valid name", tt.title, got, level)
			}
		}
	}
}

func TestEventTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dirs []string
		want string
	}{
		{[]string{"05-01 - Holidays"}, "05/01 Holidays"},
		{[]string{"12-31 - New Year"}, "12/31 New Year"},
		{[]string{"02-29 - Leap day"}, "02/29 Leap day"},
		{[]string{"05-01 - Day - 2"}, "05/01 Day - 2"},

		{[]string{"05-01 - Untitled"}, "05/01"},
		{[]string{"05-01 - untitled"}, "05/01 untitled"},

		{[]string{"Holidays"}, "Holidays"},
		{[]string{"06 - Sea"}, "06 - Sea"},
		{[]string{"09-24 Flight home"}, "09-24 Flight home"},
		{[]string{"Trip - 2024"}, "Trip - 2024"},
		{[]string{"05-01"}, "05-01"},
		{[]string{"05-01 - "}, "05-01 - "},
		{[]string{"05-01 -X"}, "05-01 -X"},
		{[]string{"05-01-X"}, "05-01-X"},
		{[]string{"05-01 X"}, "05-01 X"},
		{[]string{"13-01 - X"}, "13-01 - X"},
		{[]string{"02-30 - X"}, "02-30 - X"},
		{[]string{"5-01 - X"}, "5-01 - X"},

		{[]string{"05-01 - a∕b"}, "05/01 a∕b"},
		{[]string{"05-01 - .hidden"}, "05/01 .hidden"},

		{[]string{"05-01 - Holidays", "raw"}, "05/01 Holidays / raw"},
		{[]string{"05-01 - Trip", "05-02 - Day two"}, "05/01 Trip / 05/02 Day two"},
		{[]string{"05-01 - Untitled", "raw"}, "05/01 / raw"},
		{[]string{"Trips", "Kazan"}, "Trips / Kazan"},

		{nil, ""},
		{[]string{}, ""},
	}
	for _, tt := range tests {
		if got := naming.EventTitle(tt.dirs, eventSep, eventUntitled); got != tt.want {
			t.Errorf("EventTitle(%q) = %q, want %q", tt.dirs, got, tt.want)
		}
	}
}

func TestEventSeparator(t *testing.T) {
	t.Parallel()

	if got, want := naming.EventDir("05/01 A / b", "", eventUntitled), []string{"05-01 - A ∕ b"}; !slices.Equal(got, want) {
		t.Errorf("EventDir with an empty separator = %q, want %q", got, want)
	}
	if got, want := naming.EventDir("05/01 A > b / c", " > ", eventUntitled), []string{"05-01 - A", "b ∕ c"}; !slices.Equal(got, want) {
		t.Errorf("EventDir with separator %q = %q, want %q", " > ", got, want)
	}
	if got, want := naming.EventTitle([]string{"05-01 - A", "b"}, " > ", eventUntitled), "05/01 A > b"; got != want {
		t.Errorf("EventTitle with separator %q = %q, want %q", " > ", got, want)
	}
	if got, want := naming.EventDir("05/01", eventSep, "Без названия"), []string{"05-01 - Без названия"}; !slices.Equal(got, want) {
		t.Errorf("EventDir with another untitled = %q, want %q", got, want)
	}
	if got, want := naming.EventTitle([]string{"05-01 - Без названия"}, eventSep, "Без названия"), "05/01"; got != want {
		t.Errorf("EventTitle with another untitled = %q, want %q", got, want)
	}
}

func TestEventProperties(t *testing.T) {
	t.Parallel()

	t.Run("levels are valid, one per part", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := eventTitleGen().Draw(t, "x")
			got := naming.EventDir(x, eventSep, eventUntitled)
			if want := strings.Count(x, eventSep) + 1; len(got) != want {
				t.Fatalf("EventDir(%q) = %q, %d levels, want %d", x, got, len(got), want)
			}
			for _, level := range got {
				if !validName(level) {
					t.Fatalf("EventDir(%q) = %q, level %q is not a valid name", x, got, level)
				}
			}
		})
	})

	t.Run("title comes back", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			x := eventCanonicalTitleGen().Draw(t, "x")
			dirs := naming.EventDir(x, eventSep, eventUntitled)
			if got := naming.EventTitle(dirs, eventSep, eventUntitled); got != x {
				t.Fatalf("EventDir(%q) = %q, but EventTitle of it gives %q", x, dirs, got)
			}
		})
	})

	t.Run("directory comes back", func(t *testing.T) {
		t.Parallel()
		rapid.Check(t, func(t *rapid.T) {
			ds := eventDirsGen().Draw(t, "ds")
			title := naming.EventTitle(ds, eventSep, eventUntitled)
			if got := naming.EventDir(title, eventSep, eventUntitled); !slices.Equal(got, ds) {
				t.Fatalf("EventTitle(%q) = %q, but EventDir of it gives %q", ds, title, got)
			}
		})
	})
}

func FuzzEventDir(f *testing.F) {
	seeds := []string{
		"", "05/01", "05/01 Holidays / raw", "02/29 x", "13/01 x", "05/01 ", " / ",
		"05-01 - x / 05-01", "\xff / \xfe", "05/01 \x00",
		strings.Repeat("05/01 a / ", 40),
		"05/01 " + strings.Repeat("я", 200),
	}
	for _, seed := range seeds {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, s string) {
		dirs := naming.EventDir(s, eventSep, eventUntitled)
		if want := strings.Count(s, eventSep) + 1; len(dirs) != want {
			t.Errorf("EventDir(%q) = %q, %d levels, want %d", s, dirs, len(dirs), want)
		}
		for _, level := range dirs {
			if !validName(level) {
				t.Errorf("EventDir(%q) = %q, level %q is not a valid name", s, dirs, level)
			}
		}
		title := naming.EventTitle(dirs, eventSep, eventUntitled)
		if again := naming.EventDir(title, eventSep, eventUntitled); !slices.Equal(again, dirs) {
			t.Errorf("EventDir(%q) = %q, but EventDir of its title %q gives %q", s, dirs, title, again)
		}
		_ = naming.EventTitle([]string{s}, eventSep, eventUntitled)
	})
}

// The lengths of the months of a leap year are written out so that the
// generator does not depend on the table of the code under test.
func eventDayGen(sep string) *rapid.Generator[string] {
	days := [...]int{31, 29, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31}
	return rapid.Custom(func(t *rapid.T) string {
		m := rapid.IntRange(1, 12).Draw(t, "month")
		d := rapid.IntRange(1, days[m-1]).Draw(t, "day")
		return fmt.Sprintf("%02d%s%02d", m, sep, d)
	})
}

// A text that looks like a folder of a day or equals untitled is not
// canonical: the title would not come back from its directory.
func eventTextGen() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		s := naming.SanitizePart(rapid.StringOfN(runeGen(), 1, 40, -1).Draw(t, "text"))
		if s == eventUntitled || eventEncodedDay.MatchString(s) {
			return "_" + s
		}
		return s
	})
}

func eventTitleGen() *rapid.Generator[string] {
	day := eventDayGen("/")
	named := rapid.Custom(func(t *rapid.T) string {
		return day.Draw(t, "day") + " " + nameGen().Draw(t, "name")
	})
	part := rapid.OneOf(nameGen(), day, named)
	return rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(part, 1, 4).Draw(t, "parts"), eventSep)
	})
}

func eventCanonicalTitleGen() *rapid.Generator[string] {
	day := eventDayGen("/")
	named := rapid.Custom(func(t *rapid.T) string {
		return day.Draw(t, "day") + " " + eventTextGen().Draw(t, "text")
	})
	part := rapid.OneOf(day, named, eventTextGen())
	return rapid.Custom(func(t *rapid.T) string {
		return strings.Join(rapid.SliceOfN(part, 1, 4).Draw(t, "parts"), eventSep)
	})
}

func eventDirsGen() *rapid.Generator[[]string] {
	day := eventDayGen("-")
	untitled := rapid.Custom(func(t *rapid.T) string {
		return day.Draw(t, "day") + " - " + eventUntitled
	})
	named := rapid.Custom(func(t *rapid.T) string {
		return day.Draw(t, "day") + " - " + eventTextGen().Draw(t, "text")
	})
	sanitized := rapid.Custom(func(t *rapid.T) string {
		return naming.SanitizePart(nameGen().Draw(t, "name"))
	})
	plain := rapid.SampledFrom([]string{
		"13-01 - X", "02-30 - X", "05-01 -X", "05-01 X", "06 - Sea", "05-01 - untitled",
	})
	return rapid.SliceOfN(rapid.OneOf(day, untitled, named, sanitized, plain), 1, 4)
}
