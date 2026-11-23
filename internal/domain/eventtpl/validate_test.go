package eventtpl_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
)

// Each rule has a row that breaks it and a row that only just keeps it, and
// each row with an error breaks one rule, but for the rows that lack two
// fields to pin the order of the missing ones; the default of the other side
// or a plain year level fills the rest.
func TestParseRules(t *testing.T) {
	t.Parallel()
	const photos, cloud = eventtpl.Photos, eventtpl.Cloud
	var (
		empty, slash, digit = eventtpl.ErrEmptyLevel, eventtpl.ErrSlash, eventtpl.ErrDigitAfterNumber
		adjacent, repeated  = eventtpl.ErrAdjacentFields, eventtpl.ErrRepeatedField
		place, missing      = eventtpl.ErrDescriptionPlace, eventtpl.ErrMissingField
	)
	for _, c := range []struct {
		name   string
		side   eventtpl.Side
		levels []string
		err    *eventtpl.Error
	}{
		{name: "an empty level", side: photos, levels: []string{"{YYYY}", "", "{MM}-{DD} {Description}"}, err: fail(photos, 2, "", empty)},
		{name: "no levels", side: photos, levels: []string{}, err: fail(photos, 0, "{YYYY}", missing)},
		{name: "a path in one cloud level", side: cloud, levels: []string{"{YYYY}/{MM}-{DD} {Description}"}, err: fail(cloud, 1, "/", slash)},
		{name: "the same level in Photos", side: photos, levels: []string{"{YYYY}/{MM}-{DD} {Description}"}},
		{name: "the default of Photos as the cloud template", side: cloud, levels: []string{"{YYYY}", "{MM}/{DD} {Description}"}, err: fail(cloud, 2, "/", slash)},
		{name: "a slash inside longer cloud text", side: cloud, levels: []string{"{YYYY}", "{MM}-{DD} a/b {Description}"}, err: fail(cloud, 2, " a/b ", slash)},
		{name: "a character that the cloud refuses is left to the configuration", side: cloud, levels: []string{"{YYYY}", "{MM}:{DD} {Description}"}},
		{name: "a digit after {YYYY}", side: photos, levels: []string{"{YYYY}1", "{MM}-{DD} {Description}"}, err: fail(photos, 1, "1", digit)},
		{name: "a digit after {year}", side: photos, levels: []string{"{year}1", "{MM}-{DD} {Description}"}, err: fail(photos, 1, "1", digit)},
		{name: "a digit after {YY}", side: photos, levels: []string{"{YY}1", "{MM}-{DD} {Description}"}, err: fail(photos, 1, "1", digit)},
		{name: "a digit after {MM}", side: photos, levels: []string{"{YYYY}", "{MM}1{DD} {Description}"}, err: fail(photos, 2, "1", digit)},
		{name: "a digit after {M}", side: photos, levels: []string{"{YYYY}", "{M}1{D} {Description}"}, err: fail(photos, 2, "1", digit)},
		{name: "a digit after {DD}", side: photos, levels: []string{"{YYYY}", "{MM}-{DD}1 {Description}"}, err: fail(photos, 2, "1 ", digit)},
		{name: "a digit after {D}", side: photos, levels: []string{"{YYYY}", "{MM}-{D}1 {Description}"}, err: fail(photos, 2, "1 ", digit)},
		{name: "a zero after {DD}", side: photos, levels: []string{"{YYYY}", "{MM}-{DD}0 {Description}"}, err: fail(photos, 2, "0 ", digit)},
		{name: "a digit after {month}", side: photos, levels: []string{"{YYYY}", "{month}1 {DD} {Description}"}},
		{name: "a digit after {mon}", side: photos, levels: []string{"{YYYY}", "{mon}1 {DD} {Description}"}},
		{name: "a digit after {Month}", side: photos, levels: []string{"{YYYY}", "{Month}1 {DD} {Description}"}},
		{name: "a digit after {dow}", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {dow}1 {Description}"}},
		{name: "a digit later in the text after a number", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} 1 {Description}"}},
		{name: "a digit before a number", side: photos, levels: []string{"{YYYY}", "1{MM}-{DD} {Description}"}},
		{name: "two numbers without text between them", side: photos, levels: []string{"{YYYY}", "{MM}{DD} {Description}"}, err: fail(photos, 2, "{DD}", adjacent)},
		{name: "the description right after the day", side: photos, levels: []string{"{YYYY}", "{MM}-{DD}{Description}"}, err: fail(photos, 2, "{Description}", adjacent)},
		{name: "the description right after the day of the week", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {dow}{Description}"}, err: fail(photos, 2, "{Description}", adjacent)},
		{name: "fields at the ends of two levels", side: photos, levels: []string{"{YYYY}", "{MM}", "{DD} {Description}"}},
		{name: "a second year", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {YYYY} {Description}"}, err: fail(photos, 2, "{YYYY}", repeated)},
		{name: "a year in another spelling", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {year} {Description}"}, err: fail(photos, 2, "{year}", repeated)},
		{name: "a month by number and by name", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {Month} {Description}"}, err: fail(photos, 2, "{Month}", repeated)},
		{name: "a day in two levels", side: photos, levels: []string{"{YYYY}", "{DD}", "{MM}-{D} {Description}"}, err: fail(photos, 3, "{D}", repeated)},
		{name: "two days of the week", side: photos, levels: []string{"{YYYY} {dow}", "{MM}-{DD} {dow} {Description}"}, err: fail(photos, 2, "{dow}", repeated)},
		{name: "two descriptions: the first is out of place", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {title} {Description}"}, err: fail(photos, 2, "{title}", place)},
		{name: "the description in a level above the last", side: photos, levels: []string{"{YYYY} {Description}", "{MM}-{DD}"}, err: fail(photos, 1, "{Description}", place)},
		{name: "text after the description", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {Description}!"}, err: fail(photos, 2, "{Description}", place)},
		{name: "the description alone in the last level", side: cloud, levels: []string{"{YYYY}", "{MM}-{DD}", "{Description}"}, err: fail(cloud, 3, "{Description}", place)},
		{name: "only text before the description", side: cloud, levels: []string{"{YYYY}", "{MM}-{DD}", "# {Description}"}, err: fail(cloud, 3, "{Description}", place)},
		{name: "only the day of the week before the description", side: photos, levels: []string{"{YYYY}", "{MM}-{DD}", "{dow} {Description}"}, err: fail(photos, 3, "{Description}", place)},
		{name: "the description after a month by name", side: photos, levels: []string{"{YYYY} {DD}", "{Month} {Description}"}},
		{name: "the description after the year", side: photos, levels: []string{"{MM}-{DD}", "{YYYY} {Description}"}},
		{name: "the description after the day", side: photos, levels: []string{"{YYYY}-{MM}", "{DD} {Description}"}},
		{name: "no year", side: photos, levels: []string{"{MM}-{DD} {Description}"}, err: fail(photos, 0, "{YYYY}", missing)},
		{name: "no month", side: cloud, levels: []string{"{YYYY}", "{DD} {Description}"}, err: fail(cloud, 0, "{MM}", missing)},
		{name: "no day", side: photos, levels: []string{"{YYYY}", "{MM} {Description}"}, err: fail(photos, 0, "{DD}", missing)},
		{name: "no description", side: photos, levels: []string{"{YYYY}", "{MM}-{DD}"}, err: fail(photos, 0, "{Description}", missing)},
		{name: "no month and no day: the month first", side: photos, levels: []string{"{YYYY} {Description}"}, err: fail(photos, 0, "{MM}", missing)},
		{name: "no month and no description: the month first", side: photos, levels: []string{"{YYYY}", "{DD}"}, err: fail(photos, 0, "{MM}", missing)},
		{name: "no day and no description: the day first", side: photos, levels: []string{"{YYYY}", "{MM}"}, err: fail(photos, 0, "{DD}", missing)},
		{name: "text alone: the year is missing first", side: cloud, levels: []string{"Photos"}, err: fail(cloud, 0, "{YYYY}", missing)},
		{name: "a rule before a missing field", side: photos, levels: []string{"", "{YYYY}"}, err: fail(photos, 1, "", empty)},
	} {
		got, err := eventtpl.Parse(c.levels, c.side)
		checkErr(t, c.name, got, err, c.err)
	}
}

// Each side is checked and named, then the language; a configuration hears
// of every template that it must fix at once.
func TestValidate(t *testing.T) {
	t.Parallel()
	const photos, cloud = eventtpl.Photos, eventtpl.Cloud
	std := domain.DefaultPolicies().Templates
	monthByName := []string{"{YYYY}", "{DD} {Month} {Description}"}
	names := func(v string) domain.PolicyError { return domain.PolicyError{Key: "templates.names", Value: v} }
	missing, slash := eventtpl.ErrMissingField, eventtpl.ErrSlash
	for _, c := range []struct {
		name string
		ts   domain.Templates
		want []any
	}{
		{"the defaults of the policies", std, nil},
		{"no templates", domain.Templates{},
			[]any{*fail(photos, 0, "{YYYY}", missing), *fail(cloud, 0, "{YYYY}", missing)}},
		{"each template and the language wrong",
			domain.Templates{Photos: []string{"{YYYY}", "{MM} {Description}"}, Cloud: []string{"{YYYY}/{MM}-{DD} {Description}"}, Names: "fr"},
			[]any{*fail(photos, 0, "{DD}", missing), *fail(cloud, 1, "/", slash), names("fr")}},
		{"a month by name without the language", domain.Templates{Photos: monthByName, Cloud: std.Cloud}, []any{names("")}},
		{"a month by name in the cloud only",
			domain.Templates{Photos: std.Photos, Cloud: []string{"{YYYY}", "{DD} {month} {Description}"}}, []any{names("")}},
		{"a short month name", domain.Templates{Photos: []string{"{YYYY}", "{DD} {mon} {Description}"}, Cloud: std.Cloud}, []any{names("")}},
		{"a day of the week", domain.Templates{Photos: []string{"{YYYY}", "{MM}-{DD} {dow} {Description}"}, Cloud: std.Cloud}, []any{names("")}},
		{"names in Russian", domain.Templates{Photos: monthByName, Cloud: std.Cloud, Names: "ru"}, nil},
		{"names in English", domain.Templates{Photos: monthByName, Cloud: std.Cloud, Names: "en"}, nil},
		{"a language without names in the templates", domain.Templates{Photos: std.Photos, Cloud: std.Cloud, Names: "ru"}, nil},
		{"an unknown language", domain.Templates{Photos: std.Photos, Cloud: std.Cloud, Names: "fr"}, []any{names("fr")}},
		{"a language in another case", domain.Templates{Photos: std.Photos, Cloud: std.Cloud, Names: "RU"}, []any{names("RU")}},
		{"a template that Parse refuses is not read for names",
			domain.Templates{Photos: []string{"{Month} {DD} {Description}"}, Cloud: std.Cloud},
			[]any{*fail(photos, 0, "{YYYY}", missing)}},
	} {
		checkJoined(t, c.name, eventtpl.Validate(c.ts), c.want)
	}
}

// errors.As, not a type assertion: errorlint rejects one on an error in tests
// too.
func checkJoined(t *testing.T, name string, err error, want []any) {
	t.Helper()
	if len(want) == 0 {
		if err != nil {
			t.Errorf("%s: Validate() = %v; want nil", name, err)
		}
		return
	}
	var joined interface{ Unwrap() []error }
	if !errors.As(err, &joined) {
		t.Errorf("%s: Validate() = %v (%T); want an errors.Join of %+v", name, err, err, want)
		return
	}
	var got []any
	for _, e := range joined.Unwrap() {
		var te *eventtpl.Error
		var pe *domain.PolicyError
		switch {
		case errors.As(e, &te):
			got = append(got, *te)
		case errors.As(e, &pe):
			got = append(got, *pe)
		default:
			t.Errorf("%s: %v (%T) in Validate(); want an *eventtpl.Error or a *domain.PolicyError", name, e, e)
			return
		}
	}
	if !slices.Equal(got, want) {
		t.Errorf("%s: Validate() = %+v; want %+v", name, got, want)
	}
}
