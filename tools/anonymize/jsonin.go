package main

import (
	"bytes"
	"encoding/json"
	"io/fs"
)

// nanTokens are the bare tokens Python json.dump writes for NaN and the
// infinities, with their replacements; the infinities keep their length.
var nanTokens = []struct{ token, repl string }{
	{"NaN", "null"},
	{"Infinity", "null    "},
	{"-Infinity", "null     "},
}

// nanToNull replaces the bare NaN, Infinity and -Infinity tokens outside
// strings with null. Anything else passes through unchanged, so a broken
// input stays broken.
func nanToNull(data []byte) []byte {
	out := make([]byte, 0, len(data))
	inStr, esc := false, false
next:
	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case inStr:
			switch {
			case esc:
				esc = false
			case b == '\\':
				esc = true
			case b == '"':
				inStr = false
			}
		case b == '"':
			inStr = true
		default:
			for _, t := range nanTokens {
				if bytes.HasPrefix(data[i:], []byte(t.token)) {
					out = append(out, t.repl...)
					i += len(t.token) - 1
					continue next
				}
			}
		}
		out = append(out, b)
	}
	return out
}

// readInput reads an input file written by Python json.dump.
func readInput(fsys fs.FS, name string) ([]byte, error) {
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, err
	}
	return nanToNull(data), nil
}

// decodeFile decodes an input file; data after the JSON value is a syntax
// error.
func decodeFile(fsys fs.FS, name string, v any) error {
	data, err := readInput(fsys, name)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}
