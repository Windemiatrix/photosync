package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// Word classes by length in runes of the lower NFC form.
const (
	classShort = iota // 1-2
	class3            // 3
	class4            // 4 and more
)

func wordClass(w string) int {
	switch n := utf8.RuneCountInString(w); {
	case n >= 4:
		return class4
	case n == 3:
		return class3
	}
	return classShort
}

// idRE matches identifiers: a UUID or a long hexadecimal run. Their hex
// letters are not words on either side.
var idRE = regexp.MustCompile(`[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}|[0-9A-Fa-f]{16,}`)

// textRuns returns the letter runs (a letter with the marks that follow it)
// of s outside identifiers, in lower case NFC. It does not use anon on
// purpose: the privacy tests must not share a mistake of the code they check.
func textRuns(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(norm.NFC.String(string(cur))))
			cur = cur[:0]
		}
	}
	for _, r := range idRE.ReplaceAllString(s, " ") {
		if unicode.IsLetter(r) || len(cur) > 0 && unicode.Is(unicode.M, r) {
			cur = append(cur, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// addWords adds the letter runs of s that are not kept by design
// (structural words, known extensions, format words) to words.
func addWords(words map[string]bool, s string) {
	for _, r := range textRuns(s) {
		if !anon.AllowedWord(r) {
			words[r] = true
		}
	}
}

// inputWords returns the words of every text the tool pseudonymizes: asset
// names, album titles and folder names, cloud paths (every folder and the
// file name).
func inputWords(m *anon.Material) map[string]bool {
	out := map[string]bool{}
	for _, a := range m.Assets {
		addWords(out, a.OriginalFilename)
		addWords(out, a.Filename)
		for _, al := range a.Albums {
			addWords(out, al.Title)
			for _, f := range al.FolderNames {
				addWords(out, f)
			}
		}
	}
	for _, f := range m.Files {
		addWords(out, f.Path)
	}
	return out
}

// crossResult holds the aggregates of a cross-check; it holds no word.
type crossResult struct {
	Input  [3]int // input words by class
	Output int    // output runs checked
	// Hits are output runs equal to an input word that are not generated
	// words: a word that reached the output. Collisions are generated words
	// (pseudonyms, random letters) that happen to equal an input word.
	Hits, Collisions [3]int
	// Glued counts output runs that are not generated words and contain an
	// input word of 4+ letters inside; GluedCollisions the same in generated
	// words.
	Glued, GluedCollisions int
}

// Failed reports a word of four or more letters that reached the output.
func (r crossResult) Failed() bool { return r.Hits[class4] > 0 || r.Glued > 0 }

func (r crossResult) String() string {
	return fmt.Sprintf("input words: short=%d len3=%d len4+=%d; output runs=%d; hits len4+=%d len3=%d short=%d; "+
		"glued len4+=%d; pseudonym collisions len4+=%d len3=%d glued=%d",
		r.Input[classShort], r.Input[class3], r.Input[class4], r.Output, r.Hits[class4], r.Hits[class3], r.Hits[classShort],
		r.Glued, r.Collisions[class4], r.Collisions[class3], r.GluedCollisions)
}

// jsonStrings returns the string values and the keys outside anon.OutputKeys of
// a JSON document, or the whole text of a file that is not JSON.
func jsonStrings(data []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return []string{string(data)}
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, e := range x {
				if !anon.OutputKeys[k] {
					out = append(out, k)
				}
				walk(e)
			}
		case []any:
			for _, e := range x {
				walk(e)
			}
		case string:
			out = append(out, x)
		}
	}
	walk(v)
	return out
}

// dictionaryOf returns the generated words of tokens.json in the output.
func dictionaryOf(files map[string][]byte) map[string]bool {
	var tk anon.Tokens
	_ = json.Unmarshal(files[nameTokens], &tk) // a missing or broken dictionary is empty: every run counts as kept
	dict := map[string]bool{}
	for _, w := range tk.Words {
		dict[w] = true
	}
	return dict
}

// crossCheck compares the letter runs of every output file (tokens.json, the
// dictionary itself, aside) with the input words.
func crossCheck(input, dict map[string]bool, files map[string][]byte) crossResult {
	var r crossResult
	maxLen := 0
	for w := range input {
		r.Input[wordClass(w)]++
		maxLen = max(maxLen, utf8.RuneCountInString(w))
	}
	names := make([]string, 0, len(files))
	for n := range files {
		if n != nameTokens {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	for _, n := range names {
		for _, s := range jsonStrings(files[n]) {
			for _, run := range textRuns(s) {
				if anon.AllowedWord(run) {
					continue
				}
				r.Output++
				generated := dict[run]
				if input[run] {
					if generated {
						r.Collisions[wordClass(run)]++
					} else {
						r.Hits[wordClass(run)]++
					}
				}
				if glued(run, input, maxLen) {
					if generated {
						r.GluedCollisions++
					} else {
						r.Glued++
					}
				}
			}
		}
	}
	return r
}

// glued reports whether run holds an input word of four or more runes as a
// proper substring.
func glued(run string, input map[string]bool, maxLen int) bool {
	rs := []rune(run)
	for i := range rs {
		for n := 4; n <= maxLen && i+n <= len(rs); n++ {
			if n == len(rs) {
				continue
			}
			if input[string(rs[i:i+n])] {
				return true
			}
		}
	}
	return false
}

// digestHeader explains the digest lines to the owner.
const digestHeader = "digest: anonymized values; month and day are real by design, the year is shifted, names and folders are pseudonyms"

// digest returns at most limit lines: the header and records taken at an
// even step across the cases in name order: "name | album folders/title |
// date" for a Photos record, "path | date" for a cloud record.
func digest(files map[string][]byte, limit int) []string {
	var names []string
	for n := range files {
		if strings.HasPrefix(n, "anon-") && strings.HasSuffix(n, "/case.json") {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var recs []string
	for _, n := range names {
		var c anon.Case
		if json.Unmarshal(files[n], &c) != nil {
			continue
		}
		for _, a := range c.ICloud {
			album := "-"
			if len(a.Albums) > 0 {
				album = strings.Join(append(append([]string{}, a.Albums[0].FolderNames...), a.Albums[0].Title), "/")
			}
			recs = append(recs, a.Name()+" | "+album+" | "+a.Date)
		}
		for _, f := range c.Cloud {
			recs = append(recs, f.Path+" | "+f.DateTaken)
		}
	}
	out := []string{fmt.Sprintf("%s (%d records)", digestHeader, len(recs))}
	step := max(1, (len(recs)+limit-2)/(limit-1))
	for i := 0; i < len(recs) && len(out) < limit; i += step {
		out = append(out, recs[i])
	}
	return out
}

// readTree reads every regular file under dir by its slash path; errors
// name no path.
func readTree(dir string) (map[string][]byte, error) {
	fsys := os.DirFS(dir)
	out := map[string][]byte{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		data, err := fs.ReadFile(fsys, p)
		out[p] = data
		return err
	})
	if err != nil {
		return nil, errors.New("cannot read the directory")
	}
	return out, nil
}

func materialOf(t *testing.T, dir string) *anon.Material {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal("cannot open the materials directory")
	}
	defer closeQuietly(root)
	m, _, err := readMaterial(root.FS())
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	return m
}

func TestCrossCheck(t *testing.T) {
	t.Parallel()
	in, out, _, _, _ := buildCorpus(t, leakSalt)
	input := inputWords(materialOf(t, in))
	files := mustReadTree(t, out)
	dict := dictionaryOf(files)
	r := crossCheck(input, dict, files)
	t.Logf("corpus: %s", r)
	if r.Failed() || r.Input[class4] == 0 || r.Output == 0 || len(dict) == 0 {
		t.Errorf("pseudonymized corpus: %s", r)
	}
	// Words of the corpus planted into a fake output: a value, a key and a
	// run glued to other letters.
	planted := map[string][]byte{nameTokens: files[nameTokens],
		"anon-live/case.json": []byte(`{"icloud": [{"title": "Пельменевых", "zinaida": 1, "path": "a/ыыЗинаидыы.jpg"}]}`)}
	p := crossCheck(input, dict, planted)
	if p.Hits[class4] != 2 || p.Glued != 1 || !p.Failed() {
		t.Errorf("planted words: %s", p)
	}
	for _, w := range []string{"пельменевых", "zinaida", "зинаиды"} {
		if strings.Contains(p.String(), w) {
			t.Errorf("the aggregates hold the word %q", w)
		}
	}
	// A generated word equal to an input word is a collision, not a hit.
	dict["пельменевых"] = true
	if c := crossCheck(input, dict, planted); c.Hits[class4] != 1 || c.Collisions[class4] != 1 {
		t.Errorf("collision: %s", c)
	}
	if glued("abcd", map[string]bool{"abcd": true}, 4) || !glued("xabcd", map[string]bool{"abcd": true}, 4) {
		t.Error("glued")
	}
}

func TestDigest(t *testing.T) {
	t.Parallel()
	_, out, c, _, _ := buildCorpus(t, leakSalt)
	files := mustReadTree(t, out)
	d := digest(files, 30)
	if len(d) > 30 || len(d) < 20 || !strings.HasPrefix(d[0], digestHeader) || !reflect.DeepEqual(d, digest(files, 30)) {
		t.Errorf("digest: %d lines", len(d))
	}
	for _, line := range d[1:] {
		if strings.Count(line, " | ") < 1 {
			t.Errorf("digest line %q", line)
		}
		for _, v := range c.canaries {
			if strings.Contains(line, v) {
				t.Errorf("digest holds a canary")
			}
		}
	}
	if s := digest(map[string][]byte{}, 30); len(s) != 1 {
		t.Errorf("empty digest: %v", s)
	}
}
