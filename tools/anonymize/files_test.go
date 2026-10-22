package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// recordingFS records the names opened through it.
type recordingFS struct {
	fsys   fs.FS
	mu     sync.Mutex
	opened map[string]bool
}

func (r *recordingFS) Open(name string) (fs.File, error) {
	r.mu.Lock()
	r.opened[name] = true
	r.mu.Unlock()
	return r.fsys.Open(name)
}

func (r *recordingFS) names() []string {
	var out []string
	for n := range r.opened {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func openedBy(t *testing.T, dir string) ([]string, error) {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer closeQuietly(root)
	rec := &recordingFS{fsys: root.FS(), opened: map[string]bool{}}
	_, _, err = readMaterial(rec)
	return rec.names(), err
}

func TestReadMaterialOpensOnlyListedFiles(t *testing.T) {
	t.Parallel()
	for _, opt := range []corpusOptions{{}, {prev: true}} {
		dir := t.TempDir()
		c := writeCorpusWith(t, dir, opt)
		got, err := openedBy(t, dir)
		if err != nil {
			t.Fatalf("readMaterial: %v", err)
		}
		if !reflect.DeepEqual(got, c.read) {
			t.Errorf("opened %v; want %v", got, c.read)
		}
		for _, n := range got {
			for _, ig := range c.ignored {
				if n == ig {
					t.Errorf("opened ignored file %s", n)
				}
			}
		}
	}
}

func TestReadMaterialFields(t *testing.T) {
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
	as, fs := corpusTable()
	if len(m.Assets) != len(as) || len(m.Files) != len(fs) {
		t.Fatalf("%d assets, %d files", len(m.Assets), len(m.Files))
	}
	for i := range as {
		if !reflect.DeepEqual(m.Assets[i], as[i].a) {
			t.Errorf("asset %d = %+v; want %+v", i, m.Assets[i], as[i].a)
		}
	}
	for i := range fs {
		want := fs[i]
		if want.DateTaken == "" {
			want.Width, want.Height = 0, 0
		}
		if m.Files[i] != want {
			t.Errorf("file %d = %+v; want %+v", i, m.Files[i], want)
		}
	}
	if len(m.Links) == 0 || len(m.Plan.Download) == 0 || len(m.Plan.Conflicts) == 0 {
		t.Errorf("links %d, plan %+v", len(m.Links), m.Plan)
	}
}

func TestParseDim(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want anon.Dim
		ok   bool
	}{
		{"", 0, true}, {"null", 0, true}, {`""`, 0, true}, {"4032", 4032, true}, {" 12 ", 12, true},
		{`"4032"`, 0, false}, {"40.5", 0, false}, {"true", 0, false}, {"[]", 0, false},
	}
	for _, tt := range tests {
		got, err := parseDim(json.RawMessage(tt.raw), "files.width")
		if got != tt.want || (err == nil) != tt.ok {
			t.Errorf("parseDim(%q) = %d, %v", tt.raw, got, err)
		}
		if err != nil && err.Error() != "wrong type in field files.width" {
			t.Errorf("parseDim error %q", err)
		}
	}
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	var v struct {
		Size int `json:"original_filesize"`
	}
	typeErr := json.Unmarshal([]byte(`{"original_filesize": "Пельменева"}`), &v)
	var m map[string][]string
	mapErr := json.Unmarshal([]byte(`{"Пельменева": [1]}`), &m)
	syntaxErr := json.Unmarshal([]byte(`{"a": Пельменева}`), &v)
	_, notFound := os.Open(filepath.Join(t.TempDir(), "Пельменева"))
	tests := []struct {
		err  error
		want string
	}{
		{typeErr, "read x: wrong type in field original_filesize"},
		{mapErr, "read x: wrong type in field links"},
		{syntaxErr, "read x: syntax error at offset 8"},
		{notFound, "read x: not found"},
		{fs.ErrPermission, "read x: permission denied"},
		{errors.New("Пельменева"), "read x: unexpected error"},
		{anon.ErrTrailingData, "read x: trailing data"},
		{fail("other", "kept"), "other: kept"},
	}
	for _, tt := range tests {
		got := translate("read x", tt.err, "links").Error()
		if got != tt.want {
			t.Errorf("translate(%T) = %q; want %q", tt.err, got, tt.want)
		}
		if strings.Contains(got, "Пельменева") {
			t.Errorf("translate leaks: %q", got)
		}
	}
}

func TestPathChecks(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "a", "b"), 0o700); err != nil {
		t.Fatal(err)
	}
	ra, err := realPath(filepath.Join(base, "a"))
	if err != nil {
		t.Fatal(err)
	}
	rb, err := realPath(filepath.Join(base, "a", "b", "missing", "x"))
	if err != nil {
		t.Fatal(err)
	}
	if !within(rb, ra) || within(ra, rb) || !within(ra, ra) || within(ra+"x", ra) {
		t.Errorf("within: %s, %s", ra, rb)
	}
	if err := os.Symlink(filepath.Join(base, "a"), filepath.Join(base, "link")); err != nil {
		t.Fatal(err)
	}
	rl, err := realPath(filepath.Join(base, "link", "b"))
	if err != nil || !within(rl, ra) {
		t.Errorf("symlink not resolved: %s, %v", rl, err)
	}
	if inGitTree(rb) {
		t.Error("temporary directory is inside a git tree")
	}
	writeFile(t, filepath.Join(base, "a"), ".git", []byte("gitdir: x\n"))
	if !inGitTree(rb) {
		t.Error(".git file not found")
	}
}

func TestLoadSalt(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	good := strings.Repeat("0a", 32)
	tests := []struct {
		name, content string
		mode          os.FileMode
		kind          string
	}{
		{"good", good + "\n", 0o600, ""},
		{"good without newline", good, 0o400, ""},
		{"upper case", strings.ToUpper(good) + "\n", 0o600, "salt: not 64 lower-case hex digits"},
		{"short", good[:62] + "\n", 0o600, "salt: not 64 lower-case hex digits"},
		{"group readable", good + "\n", 0o640, "salt: permissions too open"},
		{"too large", strings.Repeat("a", 129), 0o600, "salt: too large"},
	}
	for _, tt := range tests {
		p := filepath.Join(dir, strings.ReplaceAll(tt.name, " ", "-"))
		if err := os.WriteFile(p, []byte(tt.content), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(p, tt.mode); err != nil { //nolint:gosec // the test needs modes that are too open
			t.Fatal(err)
		}
		salt, _, err := loadSalt(p, false)
		switch {
		case tt.kind == "" && (err != nil || len(salt) != 32 || salt[0] != 0x0a):
			t.Errorf("%s: %v, %v", tt.name, salt, err)
		case tt.kind != "" && (err == nil || err.Error() != tt.kind):
			t.Errorf("%s: error %v; want %q", tt.name, err, tt.kind)
		}
	}
	if _, _, err := loadSalt(filepath.Join(dir, "missing"), false); err == nil || err.Error() != "salt: not found" {
		t.Errorf("missing salt: %v", err)
	}
	if _, _, err := loadSalt(dir, false); err == nil || err.Error() != "salt: not a regular file" {
		t.Errorf("directory salt: %v", err)
	}
	p := filepath.Join(dir, "new", "deeper", "salt.key")
	salt, created, err := loadSalt(p, true)
	if err != nil || !created || len(salt) != 32 {
		t.Fatalf("create: %v, %v", created, err)
	}
	again, created, err := loadSalt(p, true)
	if err != nil || created || !reflect.DeepEqual(again, salt) {
		t.Errorf("reload: %v, %v", created, err)
	}
}

func TestWriteOutputRefusesExistingFiles(t *testing.T) {
	t.Parallel()
	out := filepath.Join(t.TempDir(), "out")
	res := &anon.Result{Cases: []anon.CaseResult{{Category: "x", Case: &anon.Case{Format: 1}}, {Category: "y"}}}
	if err := writeOutput(out, res); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(out, "anon-x", "case.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "{\n \"format\": 1,\n") || !strings.HasSuffix(string(data), "}\n") {
		t.Errorf("case.json = %q", data)
	}
	tok, _ := os.ReadFile(filepath.Join(out, "tokens.json"))
	if string(tok) != "{\n \"format\": 1,\n \"words\": []\n}\n" {
		t.Errorf("tokens.json = %q", tok)
	}
	if _, err := os.Stat(filepath.Join(out, "anon-y")); err == nil {
		t.Error("empty case written")
	}
	if err := writeOutput(out, res); err == nil {
		t.Error("second write into the same directory succeeded")
	}
}

// TestWriteOutputChecksTheDirectory: right before writing, --out must still
// be an empty real directory, not a symbolic link.
func TestWriteOutputChecksTheDirectory(t *testing.T) {
	t.Parallel()
	base := t.TempDir()
	res := &anon.Result{Cases: []anon.CaseResult{{Category: "x", Case: &anon.Case{Format: 1}}}}
	target := filepath.Join(base, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := writeOutput(link, res); err == nil || err.Error() != "out dir: not a directory" {
		t.Errorf("write through a symbolic link: %v", err)
	}
	if entries, _ := os.ReadDir(target); len(entries) != 0 {
		t.Error("the link target was written")
	}
	full := filepath.Join(base, "full")
	writeFile(t, full, "x", []byte("x"))
	if err := writeOutput(full, res); err == nil || err.Error() != "out dir: not empty" {
		t.Errorf("write into a non-empty directory: %v", err)
	}
	if err := writeOutput(filepath.Join(base, "missing", "out"), res); err == nil {
		t.Error("write without a parent succeeded")
	}
}

func TestEncodeKeepsUnicode(t *testing.T) {
	t.Parallel()
	data, err := encodeJSON(map[string]string{"t": "Ёлка <&> ́"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{\n \"t\": \"Ёлка <&> ́\"\n}\n" {
		t.Errorf("encodeJSON = %q", data)
	}
}
