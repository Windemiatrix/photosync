package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"strconv"
)

// nanFilter reads JSON written by Python json.dump with allow_nan (the
// default): outside strings the bare tokens NaN, Infinity and -Infinity
// become null, padded with spaces to the token length where it fits.
// Anything else passes through unchanged, so a broken input stays broken.
// NaN is one byte shorter than null: the filter records where the stream
// grew, and origOffset maps an offset of the filtered stream back to the
// input.
type nanFilter struct {
	r       *bufio.Reader
	pending []byte
	inStr   bool
	esc     bool
	n       int64   // bytes of the filtered stream produced so far
	grown   []int64 // filtered offsets after which the stream is one byte longer
}

// newNaNFilter reads r through a buffer of size bytes (at least 16).
func newNaNFilter(r io.Reader, size int) *nanFilter {
	return &nanFilter{r: bufio.NewReaderSize(r, size)}
}

var nanTokens = []struct {
	first      byte
	rest, repl string
}{
	{'N', "aN", "null"},
	{'I', "nfinity", "null    "},
	{'-', "Infinity", "null     "},
}

func (f *nanFilter) Read(p []byte) (int, error) {
	n := 0
	for n < len(p) {
		if len(f.pending) > 0 {
			c := copy(p[n:], f.pending)
			f.pending = f.pending[c:]
			n += c
			continue
		}
		b, err := f.r.ReadByte()
		if err != nil {
			if n > 0 {
				return n, nil
			}
			return 0, err
		}
		switch {
		case f.inStr:
			switch {
			case f.esc:
				f.esc = false
			case b == '\\':
				f.esc = true
			case b == '"':
				f.inStr = false
			}
		case b == '"':
			f.inStr = true
		default:
			if f.replace(b) {
				continue
			}
		}
		p[n] = b
		n++
		f.n++
	}
	return n, nil
}

// replace checks whether b starts a token to replace; if so it consumes the
// rest of the token and queues its replacement.
func (f *nanFilter) replace(b byte) bool {
	for _, t := range nanTokens {
		if b != t.first {
			continue
		}
		next, _ := f.r.Peek(len(t.rest)) // a short peek (end of input, error) is no match
		if string(next) != t.rest {
			return false
		}
		_, _ = f.r.Discard(len(t.rest)) // the bytes are buffered
		if extra := len(t.repl) - 1 - len(t.rest); extra > 0 {
			f.grown = append(f.grown, f.n+int64(len(t.repl))-1)
		}
		f.n += int64(len(t.repl))
		f.pending = []byte(t.repl)
		return true
	}
	return false
}

// origOffset maps an offset of the filtered stream to the input.
func (f *nanFilter) origOffset(off int64) int64 {
	k := 0
	for k < len(f.grown) && f.grown[k] < off {
		k++
	}
	return off - int64(k)
}

// byteClass names the class of a byte without showing it.
func byteClass(b byte) string {
	switch {
	case b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z':
		return "letter"
	case b >= '0' && b <= '9':
		return "digit"
	case b == ' ' || b == '\t' || b == '\n' || b == '\r':
		return "whitespace"
	case b < 0x20 || b == 0x7f:
		return "control"
	case b >= 0x80:
		return "non-ASCII"
	}
	return "punctuation"
}

// syntaxDetail scans the input up to the byte at index pos and describes
// that byte and where it stands in the JSON structure: its class, the
// position (top level, array element, object key or value, inside a string
// or not) and the nesting depth. It holds no byte of the input.
func syntaxDetail(r io.Reader, pos int64) string {
	type level struct {
		object bool
		value  bool // in an object: after the colon of the current member
	}
	var stack []level
	inStr, esc := false, false
	br := bufio.NewReaderSize(r, 1<<20)
	for i := int64(0); ; i++ {
		b, err := br.ReadByte()
		if err != nil {
			return "end of input"
		}
		if i == pos {
			where := "top level"
			if len(stack) > 0 {
				switch top := stack[len(stack)-1]; {
				case !top.object:
					where = "array element"
				case top.value:
					where = "object value"
				default:
					where = "object key"
				}
			}
			if inStr {
				where = "string in " + where
			}
			return byteClass(b) + " byte in " + where + " at depth " + strconv.Itoa(len(stack))
		}
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
		case b == '[' || b == '{':
			stack = append(stack, level{object: b == '{'})
		case (b == ']' || b == '}') && len(stack) > 0:
			stack = stack[:len(stack)-1]
		case b == ':' && len(stack) > 0:
			stack[len(stack)-1].value = true
		case b == ',' && len(stack) > 0:
			stack[len(stack)-1].value = false
		}
	}
}

// syntaxFail builds the error of a syntax error in an input file read
// through f: the offset in the file and the detail of the offending byte,
// read again from the file.
func syntaxFail(fsys fs.FS, name, stage string, f *nanFilter, se *json.SyntaxError) *toolError {
	off := f.origOffset(se.Offset)
	kind := kindSyntax(off)
	if off <= 0 {
		return fail(stage, kind)
	}
	in, err := fsys.Open(name)
	if err != nil {
		return fail(stage, kind)
	}
	defer closeQuietly(in)
	return fail(stage, kind+": "+syntaxDetail(in, off-1))
}

// translateIn translates an error of reading the input file name through f.
func translateIn(fsys fs.FS, name, stage string, f *nanFilter, err error, field string) *toolError {
	var se *json.SyntaxError
	if errors.As(err, &se) {
		return syntaxFail(fsys, name, stage, f, se)
	}
	return translate(stage, err, field)
}

// openJSON opens an input file through the filter.
func openJSON(fsys fs.FS, name string) (*nanFilter, io.Closer, error) {
	in, err := fsys.Open(name)
	if err != nil {
		return nil, nil, err
	}
	return newNaNFilter(in, 1<<20), in, nil
}

// readJSON reads a whole input file through the filter.
func readJSON(fsys fs.FS, name, stage string) ([]byte, *nanFilter, error) {
	f, c, err := openJSON(fsys, name)
	if err != nil {
		return nil, nil, translate(stage, err, "")
	}
	defer closeQuietly(c)
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, nil, translate(stage, err, "")
	}
	return buf.Bytes(), f, nil
}
