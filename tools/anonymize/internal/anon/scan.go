package anon

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Finding is one leak scanner finding: a file, a JSON pointer and a rule.
// It never holds the value that triggered it.
type Finding struct {
	File    string
	Pointer string
	Rule    string
}

// Rules of the leak scanner.
const (
	RuleUnexpectedFile = "unexpected-file" // a file that is not a regular .json file
	RuleNotJSON        = "not-json"        // a file that does not parse as JSON
	RuleNoDictionary   = "no-dictionary"   // no tokens.json in the directory
	RuleUUID           = "uuid"            // a UUID without the marker
	RuleHex            = "hex"             // 16+ hex characters without the marker
	RuleCoordinate     = "coordinate"      // a fractional number, a decimal like 55.7539, "°"
	RuleEmail          = "email"           // an e-mail address
	RulePath           = "path"            // a local path
	RuleWord           = "word"            // a word outside the dictionaries
	RuleSalt           = "salt"            // the salt in any encoding
	RuleUnexpectedKey  = "unexpected-key"  // an object key outside the fields of the output types
)

// OutputKeys are the JSON field names of the output types; any other object
// key is reported and never printed.
var OutputKeys = jsonKeys(Case{}, Tokens{}, Asset{}, AlbumRef{}, File{}, Pair{}, Provenance{})

func jsonKeys(types ...any) map[string]bool {
	keys := map[string]bool{}
	for _, v := range types {
		t := reflect.TypeOf(v)
		for i := 0; i < t.NumField(); i++ {
			if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name != "" && name != "-" {
				keys[name] = true
			}
		}
	}
	return keys
}

var (
	markerUUIDRE = regexp.MustCompile(`00000000-[0-9A-Fa-f]{4}-8[0-9A-Fa-f]{3}-[89ABab][0-9A-Fa-f]{3}-[0-9A-Fa-f]{12}`)
	isoDateRE    = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]{1,9})?(?:[+-][0-9]{2}:[0-9]{2}|Z)?`)
	coordRE      = regexp.MustCompile(`[-+]?[0-9]{1,3}\.[0-9]{2,}`)
	emailRE      = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	hexAnyRE     = regexp.MustCompile(`[0-9A-Fa-f]{16,}`)
	localPaths   = []string{`\`, "/Users/", "/Volumes/", "/home/", "/private/"}
)

// Scan checks one JSON file for leaks: identifiers without the marker,
// coordinates, e-mail addresses, local paths, words outside the dictionary
// of generated words and the format dictionary, and the salt (when given).
// Object keys are checked by every rule except "word"; a key outside the
// field names of the output types is reported as unexpected-key and shown as
// <key> in the pointer.
func Scan(name string, data []byte, words map[string]bool, salt []byte) []Finding {
	var out []Finding
	if len(salt) > 0 && hasSalt(data, salt) {
		out = append(out, Finding{name, "", RuleSalt})
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return append(out, Finding{name, "", RuleNotJSON})
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return append(out, Finding{name, "", RuleNotJSON})
	}
	s := scanner{file: name, words: words}
	s.value("", v)
	return append(out, s.out...)
}

type scanner struct {
	file  string
	words map[string]bool
	out   []Finding
	seen  map[string]bool
}

func (s *scanner) add(ptr, rule string) {
	if s.seen == nil {
		s.seen = map[string]bool{}
	}
	if k := ptr + "\x00" + rule; !s.seen[k] {
		s.seen[k] = true
		s.out = append(s.out, Finding{s.file, ptr, rule})
	}
}

func (s *scanner) value(ptr string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			p := ptr + "/" + k
			if !OutputKeys[k] {
				p = ptr + "/<key>"
				s.add(p, RuleUnexpectedKey)
			}
			s.text(p, k, false)
			s.value(p, x[k])
		}
	case []any:
		for i, e := range x {
			s.value(ptr+"/"+strconv.Itoa(i), e)
		}
	case json.Number:
		if strings.ContainsAny(string(x), ".eE") {
			s.add(ptr, RuleCoordinate)
		}
	case string:
		s.text(ptr, x, true)
	}
}

// text applies the string rules; marker identifiers and ISO dates are cut
// out first.
func (s *scanner) text(ptr, v string, words bool) {
	v = isoDateRE.ReplaceAllString(v, " ")
	v = markerUUIDRE.ReplaceAllString(v, " ")
	if uuidRE.MatchString(v) {
		s.add(ptr, RuleUUID)
		v = uuidRE.ReplaceAllString(v, " ")
	}
	for _, run := range hexAnyRE.FindAllString(v, -1) {
		if !strings.HasPrefix(run, "00000000") && strings.ContainsAny(run, "0123456789") && strings.ContainsAny(run, "abcdefABCDEF") {
			s.add(ptr, RuleHex)
		}
	}
	v = hexAnyRE.ReplaceAllString(v, " ")
	if coordRE.MatchString(v) || strings.Contains(v, "°") {
		s.add(ptr, RuleCoordinate)
	}
	if emailRE.MatchString(v) {
		s.add(ptr, RuleEmail)
	}
	if isLocalPath(v) {
		s.add(ptr, RulePath)
	}
	if !words {
		return
	}
	for _, r := range letterRuns(v) {
		if len(clusters(r)) < 2 {
			continue
		}
		w := lowerNFC(r)
		if !s.words[w] && !AllowedWord(w) {
			s.add(ptr, RuleWord)
			return
		}
	}
}

func isLocalPath(v string) bool {
	if strings.HasPrefix(v, "/") || strings.HasPrefix(v, "~") || driveRE.MatchString(v) {
		return true
	}
	for _, p := range localPaths {
		if strings.Contains(v, p) {
			return true
		}
	}
	return false
}

// hasSalt reports whether data holds the salt raw, as hex in either case or
// as base64 (standard or URL alphabet, with or without padding).
func hasSalt(data, salt []byte) bool {
	h := hex.EncodeToString(salt)
	forms := [][]byte{
		salt, []byte(h), []byte(strings.ToUpper(h)),
		[]byte(base64.RawStdEncoding.EncodeToString(salt)),
		[]byte(base64.RawURLEncoding.EncodeToString(salt)),
	}
	for _, f := range forms {
		if bytes.Contains(data, f) {
			return true
		}
	}
	return false
}
