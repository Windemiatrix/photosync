package eventtpl

import (
	"errors"
	"slices"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
)

var (
	// ErrEmptyLevel is a level without text and fields.
	ErrEmptyLevel = errors.New("eventtpl: empty level")
	// ErrMissingField is a template without a year, a month, a day or a
	// description, in any spelling.
	ErrMissingField = errors.New("eventtpl: missing field")
	// ErrRepeatedField is a second year, month, day, description or day of the
	// week, in any spelling.
	ErrRepeatedField = errors.New("eventtpl: repeated field")
	// ErrAdjacentFields is a field right after another one in a level, where
	// nothing tells where the first one ends.
	ErrAdjacentFields = errors.New("eventtpl: field right after a field")
	// ErrDigitAfterNumber is text that starts with a digit right after a field
	// written in digits, which could take that digit.
	ErrDigitAfterNumber = errors.New("eventtpl: digit right after a number")
	// ErrDescriptionPlace is a description that is not the last part of the
	// last level or has no year, month or day before it in that level.
	ErrDescriptionPlace = errors.New("eventtpl: description out of place")
	// ErrSlash is a "/" in the text of a cloud template: each level of the
	// cloud is one directory.
	ErrSlash = errors.New("eventtpl: slash in a cloud level")
)

// Each side gives back the date and the description that the other writes.
var required = []string{"YYYY", "MM", "DD", "Description"}

// These spellings write names in the language of Templates.Names.
var words = []string{"month", "mon", "Month", "dow"}

// Validate returns nil when ts suits a sync, else an errors.Join, in this
// order, of the error of Parse for ts.Photos, that for ts.Cloud, and a
// *domain.PolicyError with Key "templates.names" when Names is not "", "ru"
// or "en", or is "" while a template that Parse takes spells a month or a day
// of the week by name.
func Validate(ts domain.Templates) error {
	photos, perr := Parse(ts.Photos, Photos, ts.Names)
	cloud, cerr := Parse(ts.Cloud, Cloud, ts.Names)
	var nerr error
	if _, known := languages[ts.Names]; !known && (ts.Names != "" || worded(photos) || worded(cloud)) {
		nerr = &domain.PolicyError{Key: "templates.names", Value: ts.Names}
	}
	return errors.Join(perr, cerr, nerr)
}

func check(t Template, side Side) error {
	seen := map[string]bool{}
	for i, parts := range t.Levels {
		fail := func(err error, part string) error {
			return &Error{Side: side, Level: i + 1, Part: part, Err: err}
		}
		if len(parts) == 0 {
			return fail(ErrEmptyLevel, "")
		}
		for j, p := range parts {
			var prev string
			if j > 0 {
				prev = parts[j-1].Field
			}
			of, braced := fields[p.Field], "{"+p.Field+"}"
			switch {
			case p.Field == "" && side == Cloud && strings.Contains(p.Literal, "/"):
				return fail(ErrSlash, p.Literal)
			case p.Field == "" && number(prev) && strings.IndexAny(p.Literal, "0123456789") == 0:
				return fail(ErrDigitAfterNumber, p.Literal)
			case p.Field == "":
			case prev != "":
				return fail(ErrAdjacentFields, braced)
			case seen[of]:
				return fail(ErrRepeatedField, braced)
			// Free text reads back only as the tail of the last level, and an
			// empty description leaves the date to name the album.
			case of == "Description" && (i < len(t.Levels)-1 || j < len(parts)-1 || !slices.ContainsFunc(parts[:j], dated)):
				return fail(ErrDescriptionPlace, braced)
			}
			seen[of] = true
		}
	}
	for _, f := range required {
		if !seen[f] {
			return &Error{Side: side, Part: "{" + f + "}", Err: ErrMissingField}
		}
	}
	return nil
}

func number(field string) bool {
	return dated(Part{Field: field}) && !slices.Contains(words, field)
}

func dated(p Part) bool {
	of := fields[p.Field]
	return of == "YYYY" || of == "MM" || of == "DD"
}

func worded(t Template) bool {
	return slices.ContainsFunc(slices.Concat(t.Levels...), func(p Part) bool { return slices.Contains(words, p.Field) })
}
