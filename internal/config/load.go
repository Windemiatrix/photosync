package config

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Earlier designs had these keys; each has a code of its own so that the
// message can say what took its place.
var removed = map[string]string{
	"sync.untitled_event":   "untitled_event_removed",
	"sync.albums_root":      "albums_root_removed",
	"sync.album_template":   "album_template_removed",
	"sync.nested_separator": "nested_separator_removed",
}

var kindCodes = map[reflect.Kind]string{
	reflect.Int:    "not_integer",
	reflect.Bool:   "not_boolean",
	reflect.String: "not_string",
	reflect.Slice:  "not_list",
	reflect.Struct: "not_section",
}

// Parse reads a configuration file from data and puts the default in place of
// each key that it leaves out or leaves empty; it checks no value. The error
// is one *Error with Code "syntax", whose Line is 0 when the parser gives no
// line, or "multiple_documents"; otherwise an errors.Join of an *Error for each
// faulty key, in the order of the file, with Code
// "unknown_key", "duplicate_key", that of a removed key, or, for a value of
// the wrong kind, "not_integer", "not_boolean", "not_string", "not_list" or
// "not_section". On an error the Config is zero.
func Parse(data []byte) (Config, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc, next yaml.Node
	if err := dec.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		// The message of the parser can quote the file, such as the name of an
		// undefined anchor, so only its line number is kept.
		var line int
		_, _ = fmt.Sscanf(err.Error(), "yaml: line %d:", &line)
		return Config{}, &Error{Code: "syntax", Line: line}
	}
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		return Config{}, &Error{Code: "multiple_documents", Line: next.Line}
	}
	cfg := defaults()
	var errs []error
	if len(doc.Content) > 0 {
		decode(doc.Content[0], reflect.ValueOf(&cfg).Elem(), "", &errs)
	}
	if len(errs) > 0 {
		return Config{}, errors.Join(errs...)
	}
	return cfg, nil
}

func decode(n *yaml.Node, v reflect.Value, key string, errs *[]error) {
	// A wrong value is fixed where it is used, not at its anchor.
	line := n.Line
	// The library would decode an aliased mapping itself and drop its unknown keys.
	if n.Kind == yaml.AliasNode {
		n = n.Alias
	}
	switch {
	// An empty value keeps the default, as a missing key does; the library
	// would empty a list but keep a number. A section tagged !!null still has
	// its keys checked.
	case n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null":
	case n.Kind == yaml.MappingNode && v.Kind() == reflect.Struct:
		seen := map[string]bool{}
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			path := strings.TrimPrefix(key+"."+k.Value, ".")
			var f reflect.Value
			// The value of an alias is the name of its anchor, which can spell a key.
			if k.Kind == yaml.ScalarNode {
				f = field(v, k.Value)
			}
			switch {
			case seen[k.Value]:
				*errs = append(*errs, &Error{Key: path, Code: "duplicate_key", Line: k.Line})
			case !f.IsValid():
				*errs = append(*errs, &Error{Key: path, Code: cmp.Or(removed[path], "unknown_key"), Line: k.Line})
			default:
				decode(n.Content[i+1], f, path, errs)
			}
			seen[k.Value] = true
		}
	// The library would decode 1.5 into an int as 1.
	case v.Kind() == reflect.Int && n.ShortTag() != "!!int" || n.Decode(v.Addr().Interface()) != nil:
		value := n.Value
		// A file that YAML reads as one scalar, such as a key in PEM, would
		// otherwise land whole in the logs.
		if key == "" {
			value = ""
		}
		*errs = append(*errs, &Error{Key: key, Code: kindCodes[v.Kind()], Value: value, Line: line})
	}
}

func field(v reflect.Value, name string) reflect.Value {
	for i := range v.NumField() {
		if v.Type().Field(i).Tag.Get("yaml") == name {
			return v.Field(i)
		}
	}
	return reflect.Value{}
}
