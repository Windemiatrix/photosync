package eventtpl_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain/eventtpl"
)

// Short names keep the parts of a row on one line.
func lit(s string) eventtpl.Part { return eventtpl.Part{Literal: s} }

func fld(s string) eventtpl.Part { return eventtpl.Part{Field: s} }

func fail(side eventtpl.Side, level int, part string, err error) *eventtpl.Error {
	return &eventtpl.Error{Side: side, Level: level, Part: part, Err: err}
}

// The oracle spells the parts out, so that a scanner that drops, merges or
// splits text fails here. Every spelling of every field is in a row without
// an error, and each such row keeps every rule of the templates, so that the
// table stays true once Parse checks them.
func TestParse(t *testing.T) {
	t.Parallel()
	const photos, cloud = eventtpl.Photos, eventtpl.Cloud
	unknown := eventtpl.ErrUnknownField
	for _, c := range []struct {
		name   string
		side   eventtpl.Side
		levels []string
		want   [][]eventtpl.Part
		err    *eventtpl.Error
	}{
		{name: "the default of Photos", side: photos, levels: []string{"{YYYY}", "{MM}/{DD} {Description}"},
			want: [][]eventtpl.Part{{fld("YYYY")}, {fld("MM"), lit("/"), fld("DD"), lit(" "), fld("Description")}}},
		{name: "the default of the cloud", side: cloud, levels: []string{"{YYYY}", "{MM}-{DD} - {Description}"},
			want: [][]eventtpl.Part{{fld("YYYY")}, {fld("MM"), lit("-"), fld("DD"), lit(" - "), fld("Description")}}},
		{name: "a level of text alone and short spellings", side: cloud, levels: []string{"Photos", "{year}", "{M}.{D} {title}"},
			want: [][]eventtpl.Part{{lit("Photos")}, {fld("year")}, {fld("M"), lit("."), fld("D"), lit(" "), fld("title")}}},
		{name: "a month and a day of the week by name", side: photos, levels: []string{"{YY} {Month}", "{DD} {dow}, {Description}"},
			want: [][]eventtpl.Part{{fld("YY"), lit(" "), fld("Month")}, {fld("DD"), lit(" "), fld("dow"), lit(", "), fld("Description")}}},
		{name: "text that is not ASCII", side: photos, levels: []string{"{YYYY} год", "{mon}", "{D} – {Description}"},
			want: [][]eventtpl.Part{{fld("YYYY"), lit(" год")}, {fld("mon")}, {fld("D"), lit(" – "), fld("Description")}}},
		{name: "a month in lower case", side: cloud, levels: []string{"{YYYY}", "{DD} {month} {Description}"},
			want: [][]eventtpl.Part{{fld("YYYY")}, {fld("DD"), lit(" "), fld("month"), lit(" "), fld("Description")}}},

		{name: "an unknown field", side: photos, levels: []string{"{YYYY}", "{MMM}-{DD} {Description}"},
			err: fail(photos, 2, "{MMM}", unknown)},
		{name: "names are case-sensitive", side: cloud, levels: []string{"{yyyy}", "{MM}-{DD} {Description}"},
			err: fail(cloud, 1, "{yyyy}", unknown)},
		{name: "empty braces", side: photos, levels: []string{"{YYYY}", "{MM}-{DD} {}"},
			err: fail(photos, 2, "{}", unknown)},
		{name: "spaces inside the braces", side: photos, levels: []string{"{ YYYY }", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "{ YYYY }", unknown)},
		{name: "a brace that is not closed", side: photos, levels: []string{"{YYYY", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "{YYYY", unknown)},
		{name: "a brace that is not opened", side: photos, levels: []string{"YYYY}", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "YYYY}", unknown)},
		{name: "a closing brace twice", side: cloud, levels: []string{"{YYYY}}", "{MM}-{DD} {Description}"},
			err: fail(cloud, 1, "}", unknown)},
		{name: "a brace that is not closed and unknown", side: photos, levels: []string{"{abc", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "{abc", unknown)},
		{name: "a closing brace twice in the text", side: photos, levels: []string{"{YYYY}", "{MM}}-{DD} {Description}"},
			err: fail(photos, 2, "}-", unknown)},
		{name: "a closing brace inside text", side: photos, levels: []string{"a}b", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "a}b", unknown)},
		{name: "a closing brace before an unknown field", side: photos, levels: []string{"x}{MMM}"},
			err: fail(photos, 1, "x}", unknown)},
		{name: "a spelling near a field: Title", side: photos, levels: []string{"{Title}"}, err: fail(photos, 1, "{Title}", unknown)},
		{name: "a spelling near a field: day", side: photos, levels: []string{"{day}"}, err: fail(photos, 1, "{day}", unknown)},
		{name: "a spelling near a field: DOW", side: photos, levels: []string{"{DOW}"}, err: fail(photos, 1, "{DOW}", unknown)},
		{name: "a spelling near a field: mm", side: photos, levels: []string{"{mm}"}, err: fail(photos, 1, "{mm}", unknown)},
		{name: "braces inside braces", side: photos, levels: []string{"{{YYYY}}", "{MM}-{DD} {Description}"},
			err: fail(photos, 1, "{{YYYY}", unknown)},
		{name: "the first unknown field by level, then by part", side: photos, levels: []string{"{YYYY}", "{a} {b}", "{c}"},
			err: fail(photos, 2, "{a}", unknown)},
		{name: "an unknown field before any rule", side: photos, levels: []string{"", "{MMM}"},
			err: fail(photos, 2, "{MMM}", unknown)},
	} {
		got, err := eventtpl.Parse(c.levels, c.side)
		checkErr(t, c.name, got, err, c.err)
		if c.err == nil && !slices.EqualFunc(got.Levels, c.want, slices.Equal[[]eventtpl.Part]) {
			t.Errorf("%s: Parse = %q; want %q", c.name, got.Levels, c.want)
		}
	}
}

// errors.As, not a type assertion: errorlint rejects one on an error in tests
// too.
func checkErr(t *testing.T, name string, got eventtpl.Template, err error, want *eventtpl.Error) {
	t.Helper()
	var e *eventtpl.Error
	switch {
	case want == nil && err != nil:
		t.Errorf("%s: Parse error = %v; want nil", name, err)
	case want == nil:
	case err == nil:
		t.Errorf("%s: Parse error = nil; want %+v", name, *want)
	case !errors.As(err, &e):
		t.Errorf("%s: Parse error = %v (%T); want an *eventtpl.Error", name, err, err)
	case *e != *want:
		t.Errorf("%s: Parse error = %+v; want %+v", name, *e, *want)
	case len(got.Levels) > 0:
		t.Errorf("%s: Parse = %q with the error; want the zero Template", name, got.Levels)
	}
}

// The configuration names the side by its key and tells the rule by
// errors.Is; the text goes to the log.
func TestError(t *testing.T) {
	t.Parallel()
	if eventtpl.Photos != "photos" || eventtpl.Cloud != "cloud" {
		t.Errorf("sides = %q, %q; want the keys of the configuration, photos and cloud", eventtpl.Photos, eventtpl.Cloud)
	}
	err := error(fail(eventtpl.Cloud, 2, "{MMM}", eventtpl.ErrUnknownField))
	if got, want := err.Error(), `eventtpl: unknown field: cloud level 2: "{MMM}"`; got != want {
		t.Errorf("Error() = %q; want %q", got, want)
	}
	if !errors.Is(err, eventtpl.ErrUnknownField) {
		t.Errorf("errors.Is(%v, ErrUnknownField) = false; want true", err)
	}
}

// Whatever a configuration holds, Parse must not panic, must give back every
// byte of a template that it takes, and must say where a template fails.
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		"{YYYY}\n{MM}/{DD} {Description}", "{YYYY}\n{MM}-{DD} - {Description}", "Photos\n{year}\n{M}.{D} {title}",
		"", "\n", "{YYYY", "YYYY}", "{}", "{{YYYY}}", "a}b", "{MM}}-", "{abc", "{YYYY}\n\n{MM}-{DD} {Description}",
	} {
		f.Add(s, false)
		f.Add(s, true)
	}
	f.Fuzz(func(t *testing.T, s string, cloud bool) {
		side := eventtpl.Photos
		if cloud {
			side = eventtpl.Cloud
		}
		levels := strings.Split(s, "\n")
		got, err := eventtpl.Parse(levels, side)
		var e *eventtpl.Error
		switch {
		case err != nil && (!errors.As(err, &e) || e.Side != side || e.Level < 1 || e.Level > len(levels) || e.Err == nil || !errors.Is(err, e.Err)):
			t.Fatalf("Parse(%q, %s) error = %+v; want an *eventtpl.Error of side %s at a level from 1 to %d", levels, side, err, side, len(levels))
		case err != nil && len(got.Levels) > 0:
			t.Fatalf("Parse(%q, %s) = %q with an error; want the zero Template", levels, side, got.Levels)
		case err == nil && !slices.Equal(spell(got), levels):
			t.Fatalf("Parse(%q, %s) spells back as %q", levels, side, spell(got))
		}
	})
}

// A part with both or neither of text and a field, text next to text, or a
// brace in the text or the name breaks the contract of Template; nil then
// differs from any levels.
func spell(t eventtpl.Template) []string {
	out := make([]string, len(t.Levels))
	for i, parts := range t.Levels {
		for j, p := range parts {
			if (p.Literal == "") == (p.Field == "") || strings.ContainsAny(p.Literal+p.Field, "{}") ||
				p.Literal != "" && j > 0 && parts[j-1].Literal != "" {
				return nil
			}
			out[i] += p.Literal
			if p.Field != "" {
				out[i] += "{" + p.Field + "}"
			}
		}
	}
	return out
}
