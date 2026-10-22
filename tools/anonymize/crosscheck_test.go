package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

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

func textRuns(s string) []string { return anon.LowerRuns(idRE.ReplaceAllString(s, " ")) }

// inputWords returns the letter runs (lower case NFC) of every text the tool
// pseudonymizes: asset names, album titles and folder names, cloud paths
// (every folder and the file name). Structural words, known extensions and
// format words are kept by design and left out.
func inputWords(m *anon.Material) map[string]bool {
	out := map[string]bool{}
	add := func(s string) {
		for _, r := range textRuns(s) {
			if !anon.AllowedWord(r) {
				out[r] = true
			}
		}
	}
	for _, a := range m.Assets {
		add(a.OriginalFilename)
		add(a.Filename)
		for _, al := range a.Albums {
			add(al.Title)
			for _, f := range al.FolderNames {
				add(f)
			}
		}
	}
	for _, f := range m.Files {
		add(f.Path)
	}
	return out
}

// formatKeys are the JSON field names of the output types: code constants,
// not checked.
var formatKeys = func() map[string]bool {
	keys := map[string]bool{}
	for _, v := range []any{anon.Case{}, anon.Tokens{}, anon.Asset{}, anon.AlbumRef{}, anon.File{}, anon.Pair{}, anon.Provenance{}} {
		t := reflect.TypeOf(v)
		for i := 0; i < t.NumField(); i++ {
			name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ",")
			keys[name] = true
		}
	}
	return keys
}()

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

// outputStrings returns the string values and the keys outside formatKeys of
// a JSON document, or the whole text of a file that is not JSON.
func outputStrings(data []byte) []string {
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
				if !formatKeys[k] {
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
		for _, s := range outputStrings(files[n]) {
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

// readOutput reads every regular file under dir by its slash path; errors
// name no path.
func readOutput(dir string) (map[string][]byte, error) {
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, fmt.Errorf("cannot open the output directory")
	}
	defer closeQuietly(root)
	out := map[string][]byte{}
	var walk func(string) error
	walk = func(p string) error {
		f, err := root.Open(p)
		if err != nil {
			return fmt.Errorf("cannot read the output directory")
		}
		entries, err := f.ReadDir(-1)
		closeQuietly(f)
		if err != nil {
			return fmt.Errorf("cannot read the output directory")
		}
		for _, e := range entries {
			q := e.Name()
			if p != "." {
				q = p + "/" + q
			}
			switch {
			case e.IsDir():
				if err := walk(q); err != nil {
					return err
				}
			case e.Type().IsRegular():
				data, err := readLimited(root.FS(), q)
				if err != nil {
					return fmt.Errorf("cannot read an output file")
				}
				out[q] = data
			}
		}
		return nil
	}
	return out, walk(".")
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
	files, err := readOutput(out)
	if err != nil {
		t.Fatal(err)
	}
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
	files, err := readOutput(out)
	if err != nil {
		t.Fatal(err)
	}
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
