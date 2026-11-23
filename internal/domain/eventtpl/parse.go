package eventtpl

import (
	"errors"
	"fmt"
	"strings"
)

// Side is a side of a sync that a template lays out, named as the key of its
// template in the configuration.
type Side string

// The sides.
const (
	Photos Side = "photos"
	Cloud  Side = "cloud"
)

// Template is a template that Parse takes: the parts of each level, in order,
// with no two parts of text next to each other.
type Template struct {
	Levels [][]Part
}

// Part is a piece of a level: the text Literal, never empty, when Field is
// empty, else the field named Field as it is spelled between the braces.
type Part struct {
	Literal, Field string
}

// ErrUnknownField is a name between braces that is not a field, or a brace
// without its pair: braces stand only around fields.
var ErrUnknownField = errors.New("eventtpl: unknown field")

// Error is a template that photosync does not take: the rule Err, one of the
// errors of this package, broken by Part of the level Level of the template
// of Side. Level counts from 1, and 0 stands for the whole template; Part is
// text or a field in braces as written, or the field that is missing.
type Error struct {
	Side  Side
	Level int
	Part  string
	Err   error
}

func (e *Error) Error() string {
	return fmt.Sprintf("%v: %s level %d: %q", e.Err, e.Side, e.Level, e.Part)
}

// Unwrap returns Err, for errors.Is.
func (e *Error) Unwrap() error { return e.Err }

// A template has each field once whatever the spelling, so the spellings of
// a field map to its first one.
var fields = map[string]string{
	"YYYY": "YYYY", "year": "YYYY", "YY": "YYYY",
	"MM": "MM", "M": "MM", "month": "MM", "mon": "MM", "Month": "MM",
	"DD": "DD", "D": "DD",
	"Description": "Description", "title": "Description",
	"dow": "dow",
}

// Parse returns the template of levels, one string per level as the
// configuration writes them, for side. A name between braces is a field,
// spelled with its case, and the rest is text. The error is an *Error, with
// the zero Template: for the first unknown field by level and part, else for
// the first level or part that breaks a rule of the errors of this package,
// else for the first of the year, month, day and description that it lacks.
func Parse(levels []string, side Side) (Template, error) {
	t := Template{Levels: make([][]Part, len(levels))}
	for i, s := range levels {
		fail := func(part string) (Template, error) {
			return Template{}, &Error{Side: side, Level: i + 1, Part: part, Err: ErrUnknownField}
		}
		for s != "" {
			text, rest, open := strings.Cut(s, "{")
			name, after, closed := strings.Cut(rest, "}")
			switch {
			case strings.Contains(text, "}"):
				return fail(text)
			case open && !closed:
				return fail("{" + rest)
			case open && fields[name] == "":
				return fail("{" + name + "}")
			}
			if text != "" {
				t.Levels[i] = append(t.Levels[i], Part{Literal: text})
			}
			if open {
				t.Levels[i] = append(t.Levels[i], Part{Field: name})
			}
			s = after
		}
	}
	if err := check(t, side); err != nil {
		return Template{}, err
	}
	return t, nil
}
