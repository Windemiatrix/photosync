package main

import (
	"bytes"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// leakSalt is a fixed salt of the leak tests: a pseudonym that equals an
// input word by chance would make a random salt flaky; with a fixed one the
// result is stable.
var leakSalt = strings.Repeat("5a17", 16)

func writeSalt(t *testing.T, hexSalt string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "salt.key")
	writeFile(t, filepath.Dir(p), filepath.Base(p), []byte(hexSalt+"\n"))
	return p
}

// buildCorpus writes the corpus and builds it with a salt.
func buildCorpus(t *testing.T, hexSalt string) (in, out string, c corpus, stdout, stderr string) {
	t.Helper()
	in, out, _, c = setup(t, corpusOptions{})
	code, stdout, stderr := runTool("--in", in, "--out", out, "--salt-file", writeSalt(t, hexSalt))
	if code != 0 {
		t.Fatalf("build: code %d\n%s\n%s", code, stdout, stderr)
	}
	return in, out, c, stdout, stderr
}

var (
	digitRunRE  = regexp.MustCompile(`[0-9]+`)
	compactDate = regexp.MustCompile(`^(19|20)[0-9]{2}(0[1-9]|1[0-2])(0[1-9]|[12][0-9]|3[01])$`)
)

// letterRunsOf returns the letter runs (letters with their marks) of s.
func letterRunsOf(s string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, string(cur))
			cur = cur[:0]
		}
	}
	for _, r := range s {
		if unicode.IsLetter(r) || len(cur) > 0 && unicode.Is(unicode.M, r) {
			cur = append(cur, r)
		} else {
			flush()
		}
	}
	flush()
	return out
}

// stringValues returns the string values of a JSON document, or the whole
// text of another file.
func stringValues(data []byte) []string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return []string{string(data)}
	}
	var out []string
	var walk func(any)
	walk = func(v any) {
		switch x := v.(type) {
		case map[string]any:
			for _, e := range x {
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

func readTree(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		out[filepath.ToSlash(rel)] = data
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func allowed(w string) bool { return anon.AllowedWord(strings.ToLower(w)) }

// findLeaks returns the input letter runs of four and more runes, the input
// digit runs of seven and more digits (not dates) and the canaries that
// occur in the output.
func findLeaks(input, output map[string][]byte, canaries []string) []string {
	outWords := map[string]bool{}
	var outText strings.Builder
	for _, data := range output {
		outText.Write(data)
		for _, s := range stringValues(data) {
			for _, r := range letterRunsOf(s) {
				outWords[strings.ToLower(r)] = true
			}
		}
	}
	text := outText.String()
	var leaks []string
	seen := map[string]bool{}
	for _, data := range input {
		for _, s := range stringValues(data) {
			for _, r := range letterRunsOf(s) {
				w := strings.ToLower(r)
				if len([]rune(w)) >= 4 && !allowed(w) && outWords[w] && !seen[w] {
					seen[w] = true
					leaks = append(leaks, "word "+w)
				}
			}
			for _, loc := range digitRunRE.FindAllStringIndex(s, -1) {
				d := s[loc[0]:loc[1]]
				if afterCompactDate(s, loc[0]) && keptTime(d) {
					continue // the time of PXL_YYYYMMDD_HHMMSS[mmm] is kept by design
				}
				if len(d) >= 7 && !compactDate.MatchString(d) && strings.Contains(text, d) && !seen[d] {
					seen[d] = true
					leaks = append(leaks, "digits "+d)
				}
			}
		}
	}
	for _, c := range canaries {
		if strings.Contains(text, c) {
			leaks = append(leaks, "canary "+c)
		}
	}
	sort.Strings(leaks)
	return leaks
}

// afterCompactDate reports whether the digit run at i follows a YYYYMMDD
// date run and "_".
func afterCompactDate(s string, i int) bool {
	if i < 9 || s[i-1] != '_' || i >= 10 && s[i-10] >= '0' && s[i-10] <= '9' {
		return false
	}
	return compactDate.MatchString(s[i-9 : i-1])
}

// keptTime reports whether a digit run after a date is a valid HHMMSS or
// HHMMSSmmm, the only run the mapping keeps there.
func keptTime(d string) bool {
	if len(d) != 6 && len(d) != 9 {
		return false
	}
	_, err := time.Parse("150405", d[:6])
	return err == nil
}

func TestFindLeaksFindsPlantedRuns(t *testing.T) {
	t.Parallel()
	if !afterCompactDate("PXL_20210305_1011123", 13) || afterCompactDate("X_12345678_1011123", 11) || afterCompactDate("1011123", 0) {
		t.Error("afterCompactDate")
	}
	input := map[string][]byte{"a.json": []byte(`{"t": "Пельменева 1234567 ok", "p": "PXL_20210305_101112345 IMG_20190514_79161234567 IMG_20190514_256199123"}`), "b.txt": []byte("KanareykaX")}
	clean := map[string][]byte{"case.json": []byte(`{"t": "Ыжтьц 7654321", "p": "PXL_20610305_101112345"}`)}
	if l := findLeaks(input, clean, []string{"KanareykaX"}); len(l) != 0 {
		t.Errorf("clean output: %v", l)
	}
	planted := map[string][]byte{"case.json": []byte(`{"t": "пельменева", "n": "x1234567", "c": "KanareykaX", "p": "IMG_20590514_79161234567 IMG_20590514_256199123"}`)}
	if l := findLeaks(input, planted, []string{"KanareykaX"}); strings.Join(l, ",") != "canary KanareykaX,digits 1234567,digits 256199123,digits 79161234567,word kanareykax,word пельменева" {
		t.Errorf("planted output: %v", l)
	}
}

func TestNoLeaks(t *testing.T) {
	t.Parallel()
	in, out, c, stdout, stderr := buildCorpus(t, leakSalt)
	input, output := readTree(t, in), readTree(t, out)
	if l := findLeaks(input, output, c.canaries); len(l) != 0 {
		t.Errorf("leaks: %v", l)
	}
	// Rule names are code constants; one of them ("root-path") shares a word
	// with the CSV headers of the input.
	for _, r := range anon.UnsupportedRules {
		stdout = strings.ReplaceAll(stdout, " "+r+"=", " =")
	}
	if l := findLeaks(input, map[string][]byte{"stdout": []byte(stdout), "stderr": []byte(stderr)}, c.canaries); len(l) != 0 {
		t.Errorf("leaks in stdout or stderr: %v", l)
	}
	text := strings.ToUpper(string(bytes.Join(mapValues(output), nil)))
	for _, id := range append(append([]string{}, c.uuids...), c.sha1s...) {
		if strings.Contains(text, strings.ToUpper(id)) {
			t.Errorf("input identifier in the output: %s", id)
		}
	}
}

func mapValues(m map[string][]byte) [][]byte {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][]byte, len(keys))
	for i, k := range keys {
		out[i] = m[k]
	}
	return out
}

func TestOutputFieldsAndDates(t *testing.T) {
	t.Parallel()
	_, out, c, _, _ := buildCorpus(t, leakSalt)
	wantKeys := []string{"album_info", "burst", "burst_key", "date", "filename", "has_raw", "hasadjustments", "height",
		"ismovie", "israw", "live_photo", "original_filename", "original_filesize", "original_height", "original_width",
		"tzoffset", "uuid", "width"}
	years := map[string]bool{}
	for _, y := range c.years {
		years[y] = true
	}
	cases := 0
	for name, data := range readTree(t, out) {
		if name == "tokens.json" {
			continue
		}
		cases++
		var doc struct {
			ICloud []map[string]any `json:"icloud"`
			Cloud  []map[string]any `json:"cloud"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatal(err)
		}
		for _, a := range doc.ICloud {
			var keys []string
			for k := range a {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			if strings.Join(keys, ",") != strings.Join(wantKeys, ",") {
				t.Errorf("%s: icloud keys %v", name, keys)
			}
			if d, _ := a["date"].(string); len(d) >= 4 && years[d[:4]] {
				t.Errorf("%s: corpus year in %s", name, d[:4])
			}
		}
		for _, f := range doc.Cloud {
			if d, _ := f["date_taken"].(string); len(d) >= 4 && years[d[:4]] {
				t.Errorf("%s: corpus year in %s", name, d[:4])
			}
			if p, _ := f["path"].(string); len(p) >= 5 && p[4] == '/' && years[p[:4]] {
				t.Errorf("%s: corpus year folder %s", name, p[:4])
			}
		}
		for _, banned := range []string{`"latitude"`, `"longitude"`, `"place"`, `"persons"`, `"exif_info"`, `"path_edited"`, `"library"`, `"intrash"`} {
			if bytes.Contains(data, []byte(banned)) {
				t.Errorf("%s: field %s in the output", name, banned)
			}
		}
	}
	if cases < 25 {
		t.Errorf("%d cases", cases)
	}
}

func TestNoSaltInOutput(t *testing.T) {
	t.Parallel()
	_, out, _, stdout, stderr := buildCorpus(t, leakSalt)
	raw, _ := hex.DecodeString(leakSalt)
	forms := []string{string(raw), leakSalt, strings.ToUpper(leakSalt), base64.StdEncoding.EncodeToString(raw),
		base64.RawStdEncoding.EncodeToString(raw), base64.URLEncoding.EncodeToString(raw), base64.RawURLEncoding.EncodeToString(raw)}
	all := stdout + stderr
	for _, data := range readTree(t, out) {
		all += string(data)
	}
	for _, f := range forms {
		if strings.Contains(all, f) {
			t.Errorf("salt form %q... in the output", f[:6])
		}
	}
}

func TestDeterministic(t *testing.T) {
	t.Parallel()
	_, out1, _, stdout1, _ := buildCorpus(t, leakSalt)
	_, out2, _, stdout2, _ := buildCorpus(t, leakSalt)
	a, b := readTree(t, out1), readTree(t, out2)
	if len(a) != len(b) || stdout1 != stdout2 {
		t.Fatalf("%d files, %d files", len(a), len(b))
	}
	for name, data := range a {
		if !bytes.Equal(b[name], data) {
			t.Errorf("%s differs", name)
		}
	}
}

// TestOtherSalt maps every supported record of the corpus with two salts.
func TestOtherSalt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCorpus(t, dir)
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeQuietly(root)
	m, _, err := readMaterial(root.FS())
	if err != nil {
		t.Fatal(err)
	}
	units, err := anon.SupportedUnits(m)
	if err != nil {
		t.Fatal(err)
	}
	set := m.Subset(flatten(units))
	m1, _ := anon.NewMapper(bytes.Repeat([]byte{1}, 32))
	m2, _ := anon.NewMapper(bytes.Repeat([]byte{2}, 32))
	s1, err1 := m1.Set(set)
	s2, err2 := m2.Set(set)
	if err1 != nil || err2 != nil {
		t.Fatal(err1, err2)
	}
	var ids [][2]string
	sizes, sizesDiff := 0, 0
	for i := range s1.Assets {
		ids = append(ids, [2]string{s1.Assets[i].UUID, s2.Assets[i].UUID}, [2]string{s1.Assets[i].BurstKey, s2.Assets[i].BurstKey})
		if set.Assets[i].OriginalFilesize > 0 {
			sizes++
			if s1.Assets[i].OriginalFilesize != s2.Assets[i].OriginalFilesize {
				sizesDiff++
			}
		}
	}
	for i := range s1.Files {
		ids = append(ids, [2]string{s1.Files[i].ContentID, s2.Files[i].ContentID}, [2]string{s1.Files[i].SHA1, s2.Files[i].SHA1})
		if set.Files[i].Size > 0 {
			sizes++
			if s1.Files[i].Size != s2.Files[i].Size {
				sizesDiff++
			}
		}
	}
	for _, p := range ids {
		if p[0] != "" && p[0] == p[1] {
			t.Errorf("identifier %s is the same under two salts", p[0])
		}
	}
	if sizesDiff*10 < sizes*9 {
		t.Errorf("%d of %d sizes differ", sizesDiff, sizes)
	}
	words := map[string]bool{}
	for _, a := range set.Assets {
		for _, s := range []string{a.OriginalFilename} {
			for _, w := range letterRunsOf(s) {
				words[w] = true
			}
		}
		for _, al := range a.Albums {
			for _, w := range letterRunsOf(al.Title) {
				words[w] = true
			}
		}
	}
	for _, f := range set.Files {
		for _, w := range letterRunsOf(f.Path) {
			words[w] = true
		}
	}
	total, diff := 0, 0
	for w := range words {
		if len([]rune(w)) < 2 || allowed(w) {
			continue
		}
		a, err1 := m1.Text(w)
		b, err2 := m2.Text(w)
		if err1 != nil || err2 != nil {
			t.Fatal(err1, err2)
		}
		total++
		if a != b {
			diff++
		}
	}
	if total < 20 || diff*10 < total*9 {
		t.Errorf("%d of %d words differ", diff, total)
	}
}

func TestCorpusCoversCategories(t *testing.T) {
	t.Parallel()
	_, _, _, stdout, _ := buildCorpus(t, leakSalt)
	for _, c := range anon.Categories {
		empty := strings.Contains(stdout, "case anon-"+c.Name+": empty\n")
		if want := c.Name == "nfd" && !anon.NormalizationAvailable; empty != want {
			t.Errorf("category %s: empty %v", c.Name, empty)
		}
	}
	if !regexp.MustCompile(`units: total=\d+ unsupported=[1-9]\d* too_large=0 exhausted=0\n`).MatchString(stdout) {
		t.Errorf("stdout = %s", stdout)
	}
	// Every rule of "not supported" fires on the corpus; "mark" only without
	// normalization, "clusters" only with it.
	line := regexp.MustCompile(`(?m)^unsupported:(.*)$`).FindStringSubmatch(stdout)
	if line == nil {
		t.Fatalf("no unsupported line: %s", stdout)
	}
	for _, rule := range anon.UnsupportedRules {
		m := regexp.MustCompile(` ` + regexp.QuoteMeta(rule) + `=(\d+)`).FindStringSubmatch(line[1])
		if m == nil {
			t.Errorf("rule %s missing in %q", rule, line[0])
			continue
		}
		zero := rule == anon.RuleMark && anon.NormalizationAvailable || rule == anon.RuleClusters && !anon.NormalizationAvailable
		if (m[1] == "0") != zero {
			t.Errorf("rule %s: %s units", rule, m[1])
		}
	}
}
