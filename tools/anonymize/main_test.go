package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// runTool runs the command line and returns the exit code and the output.
func runTool(args ...string) (code int, stdout, stderr string) {
	var o, e bytes.Buffer
	code = safely(func() int { return run(args, &o, &e) }, &e)
	return code, o.String(), e.String()
}

// setup writes a corpus and returns the paths of a build run: the corpus,
// a missing output directory and a missing salt file.
func setup(t *testing.T, opt corpusOptions) (in, out, salt string, c corpus) {
	t.Helper()
	base := t.TempDir()
	in = filepath.Join(base, "in")
	c = writeCorpusWith(t, in, opt)
	return in, filepath.Join(base, "out"), filepath.Join(base, "keys", "anonymize.key"), c
}

func TestUsageErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{"no flags", nil},
		{"mixed modes", []string{"--check", "x", "--in", "y"}},
		{"check with out", []string{"--check", "x", "--out", "y"}},
		{"no out", []string{"--in", "x", "--salt-file", "s"}},
		{"no in", []string{"--out", "x", "--salt-file", "s"}},
		{"no salt", []string{"--in", "x", "--out", "y"}},
		{"empty in", []string{"--in=", "--out", "y", "--salt-file", "s"}},
		{"extra argument", []string{"--in", "x", "--out", "y", "--salt-file", "s", "extra"}},
		{"extra argument in check", []string{"--check", "x", "extra"}},
		{"unknown flag", []string{"--in", "x", "--out", "y", "--salt-file", "s", "--verbose"}},
		{"flag without value", []string{"--check"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			code, stdout, stderr := runTool(tt.args...)
			if code != 2 || stdout != "" || !strings.Contains(stderr, "usage:") {
				t.Errorf("code %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			if strings.Contains(stderr, "verbose") {
				t.Errorf("stderr repeats the argument: %q", stderr)
			}
		})
	}
}

func TestHelp(t *testing.T) {
	t.Parallel()
	for _, h := range []string{"-h", "--help"} {
		code, stdout, stderr := runTool(h)
		if code != 0 || !strings.Contains(stdout, "usage:") || stderr != "" {
			t.Errorf("%s: code %d, stdout %q, stderr %q", h, code, stdout, stderr)
		}
	}
}

var aggregateLine = regexp.MustCompile(`^(inventory: (current|prev)|links: pairs=\d+( [a-z_]+=\d+)+|units: total=\d+ unsupported=\d+ too_large=\d+ exhausted=\d+|unsupported:( [a-z0-9-]+=\d+)+|relations:( [a-z_]+=\d+)*|case anon-[a-z0-9-]+: (empty|records=\d+ pairs=\d+( [a-z_]+=\d+)*)|cases: \d+)$`)

func TestBuildAndCheck(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{})
	code, stdout, stderr := runTool("--in", in, "--out", out, "--salt-file", salt)
	if code != 0 {
		t.Fatalf("build: code %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
	}
	if stderr != "anonymize: created a new salt file\n" {
		t.Errorf("stderr = %q", stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	for _, l := range lines {
		if !aggregateLine.MatchString(l) {
			t.Errorf("stdout line %q is not an aggregate", l)
		}
	}
	if lines[0] != "inventory: current" || !strings.HasPrefix(lines[len(lines)-1], "cases: ") {
		t.Errorf("stdout = %q", stdout)
	}
	if !strings.Contains(stdout, "case anon-level-exact: records=") {
		t.Errorf("stdout = %q", stdout)
	}
	for _, p := range []string{"tokens.json", "anon-level-exact/case.json", "anon-nfd/case.json"} {
		if _, err := os.Stat(filepath.Join(out, p)); err != nil {
			t.Errorf("%s: %v", p, err)
		}
	}
	code, stdout, stderr = runTool("--check", out)
	if code != 0 || !regexp.MustCompile(`^check: 0 findings in \d+ files\n$`).MatchString(stdout) || stderr != "" {
		t.Errorf("check: code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
	code, stdout, _ = runTool("--check", out, "--salt-file", salt)
	if code != 0 || !strings.HasPrefix(stdout, "check: 0 findings") {
		t.Errorf("check with salt: code %d, stdout %q", code, stdout)
	}
}

func TestPrevInventory(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{prev: true})
	code, stdout, stderr := runTool("--in", in, "--out", out, "--salt-file", salt)
	if code != 0 || !strings.HasPrefix(stdout, "inventory: prev\n") {
		t.Errorf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

// gitTree makes dir a git working tree and returns a symbolic link to it
// next to dir.
func gitTree(t *testing.T, dir string) string {
	t.Helper()
	writeFile(t, dir, ".git/HEAD", []byte("ref: refs/heads/main\n"))
	link := dir + "-link"
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	return link
}

func TestBuildErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		opt  corpusOptions
		prep func(t *testing.T, in, out, salt string) (string, string, string)
		kind string
	}{
		{"out not empty", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			writeFile(t, out, "x", []byte("x"))
			return in, out, salt
		}, "out dir: not empty"},
		{"out inside git tree", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			repo := filepath.Join(filepath.Dir(out), "repo")
			gitTree(t, repo)
			return in, filepath.Join(repo, "out"), salt
		}, "out dir: inside a git working tree"},
		{"out inside git tree through a link", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			link := gitTree(t, filepath.Join(filepath.Dir(out), "repo"))
			return in, filepath.Join(link, "out"), salt
		}, "out dir: inside a git working tree"},
		{"out inside in", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			return in, filepath.Join(in, "out"), salt
		}, "out dir: inside --in"},
		{"in inside out", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			return in, filepath.Dir(in), salt
		}, "out dir: not empty"},
		{"in missing", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			return in + "-missing", out, salt
		}, "in dir: not found"},
		{"salt inside git tree through a link", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			link := gitTree(t, filepath.Join(filepath.Dir(out), "repo"))
			return in, out, filepath.Join(link, "keys", "salt.key")
		}, "salt: inside a git working tree"},
		{"salt file is a link into a git tree", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			repo := filepath.Join(filepath.Dir(out), "repo")
			gitTree(t, repo)
			writeFile(t, repo, "salt.key", []byte(strings.Repeat("ab", 32)+"\n"))
			if err := os.MkdirAll(filepath.Dir(salt), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(repo, "salt.key"), salt); err != nil {
				t.Fatal(err)
			}
			return in, out, salt
		}, "salt: inside a git working tree"},
		{"salt too open", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			writeFile(t, filepath.Dir(salt), filepath.Base(salt), []byte(strings.Repeat("ab", 32)+"\n"))
			if err := os.Chmod(salt, 0o644); err != nil { //nolint:gosec // the test needs a mode that is too open
				t.Fatal(err)
			}
			return in, out, salt
		}, "salt: permissions too open"},
		{"salt not hex", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			writeFile(t, filepath.Dir(salt), filepath.Base(salt), []byte(strings.Repeat("zz", 32)+"\n"))
			return in, out, salt
		}, "salt: not 64 hex digits"},
		{"plan rerun differs", corpusOptions{rerunDiffers: true}, nil, "plan rerun: plan rerun differs from the reviewed plan"},
		{"no inventory passes", corpusOptions{noPrevMatch: true}, nil, "partition: no inventory passes the check: current: "},
		{"broken catalog", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			writeFile(t, in, "icloud_catalog.json", []byte(`[{"uuid": "A"}, {"uuid": `))
			return in, out, salt
		}, "read catalog: syntax error at offset 25"},
		{"wrong field type", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			writeFile(t, in, "yandex_inventory.json", []byte(`{"files": [{"path": "2023/x.jpg", "size": "big"}]}`))
			return in, out, salt
		}, "read inventory: wrong type (string)"},
		{"missing links", corpusOptions{}, func(t *testing.T, in, out, salt string) (string, string, string) {
			if err := os.Remove(filepath.Join(in, "plan-rerun", "links.json")); err != nil {
				t.Fatal(err)
			}
			return in, out, salt
		}, "read links: not found"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in, out, salt, _ := setup(t, tt.opt)
			if tt.prep != nil {
				in, out, salt = tt.prep(t, in, out, salt)
			}
			code, stdout, stderr := runTool("--in", in, "--out", out, "--salt-file", salt)
			if code != 1 || !strings.Contains(stderr, "anonymize: "+tt.kind) {
				t.Errorf("code %d, stdout %q, stderr %q; want %q", code, stdout, stderr, tt.kind)
			}
			if strings.Contains(stderr, in) || strings.Contains(stderr, out) {
				t.Errorf("stderr holds a path: %q", stderr)
			}
		})
	}
}

func TestSaltCreation(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{})
	if code, _, stderr := runTool("--in", in, "--out", out, "--salt-file", salt); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	st, err := os.Stat(salt)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Errorf("salt mode %v", st.Mode().Perm())
	}
	dst, err := os.Stat(filepath.Dir(salt))
	if err != nil || dst.Mode().Perm() != 0o700 {
		t.Errorf("salt directory mode %v, %v", dst.Mode().Perm(), err)
	}
	data, err := os.ReadFile(salt)
	if err != nil || !regexp.MustCompile(`^[0-9a-f]{64}\n$`).Match(data) {
		t.Errorf("salt file = %d bytes, %v", len(data), err)
	}
	out2 := out + "2"
	code, _, stderr := runTool("--in", in, "--out", out2, "--salt-file", salt)
	if code != 0 || stderr != "" {
		t.Fatalf("second run: code %d, stderr %q", code, stderr)
	}
	again, _ := os.ReadFile(salt)
	if !bytes.Equal(again, data) {
		t.Error("salt changed")
	}
	a, _ := os.ReadFile(filepath.Join(out, "tokens.json"))
	b, _ := os.ReadFile(filepath.Join(out2, "tokens.json"))
	if !bytes.Equal(a, b) || len(a) == 0 {
		t.Error("the second run did not use the same salt")
	}
	if code, _, _ := runTool("--check", out, "--salt-file", salt+"-missing"); code != 1 {
		t.Error("check created or accepted a missing salt file")
	}
	if _, err := os.Stat(salt + "-missing"); err == nil {
		t.Error("check created a salt file")
	}
}

func TestOutputPermissions(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{})
	if code, _, stderr := runTool("--in", in, "--out", out, "--salt-file", salt); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	n := 0
	err := filepath.WalkDir(out, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		}
		if info.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", p, info.Mode().Perm(), want)
		}
		n++
		return nil
	})
	if err != nil || n < 10 {
		t.Errorf("walk: %d entries, %v", n, err)
	}
}

// TestOutputDirs: --out may be an existing empty directory or a path whose
// parents are missing.
func TestOutputDirs(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{})
	if err := os.Mkdir(out, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := runTool("--in", in, "--out", out, "--salt-file", salt); code != 0 {
		t.Fatalf("existing empty: code %d: %s", code, stderr)
	}
	deep := filepath.Join(out+"2", "a", "b")
	if code, _, stderr := runTool("--in", in, "--out", deep, "--salt-file", salt); code != 0 {
		t.Fatalf("missing parents: code %d: %s", code, stderr)
	}
	if st, err := os.Stat(filepath.Join(out+"2", "a")); err != nil || st.Mode().Perm() != 0o700 {
		t.Errorf("created parent: %v", err)
	}
}

// snapshot records the names, sizes, modes, modification times and content
// hashes of a tree.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rec := info.Mode().String() + " " + info.ModTime().Format(time.RFC3339Nano)
		if !d.IsDir() {
			data, err := os.ReadFile(p) //nolint:gosec // G122: a file the test wrote into t.TempDir()
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			rec += " " + hex.EncodeToString(sum[:]) + " " + filepath.Base(p)
		}
		out[p] = rec
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestInputUnchanged(t *testing.T) {
	t.Parallel()
	in, out, salt, _ := setup(t, corpusOptions{prev: true})
	before := snapshot(t, in)
	if code, _, stderr := runTool("--in", in, "--out", out, "--salt-file", salt); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	after := snapshot(t, in)
	if len(before) != len(after) {
		t.Fatalf("%d entries before, %d after", len(before), len(after))
	}
	for p, rec := range before {
		if after[p] != rec {
			t.Errorf("%s changed", p)
		}
	}
}

// TestErrorsWithoutValues breaks the corpus right next to personal values and
// checks that no canary reaches the output.
func TestErrorsWithoutValues(t *testing.T) {
	t.Parallel()
	canary := "Зинаида Пельменева"
	tests := []struct {
		name, file, content, kind string
	}{
		{"syntax error after a canary", "icloud_catalog.json", `[{"uuid": "A", "description": "` + canary + `" x}]`, "read catalog: syntax error at offset"},
		{"coordinate in a string field", "icloud_catalog.json", `[{"uuid": "A", "original_filename": 55.7539}]`, "read catalog: wrong type (number)"},
		{"coordinate in an integer field", "icloud_catalog.json", `[{"uuid": "A", "tzoffset": 55.7539}]`, "read catalog: wrong type (number)"},
		{"canary in a number field", "icloud_catalog.json", `[{"uuid": "A", "original_filesize": "` + canary + `"}]`, "read catalog: wrong type (string)"},
		{"canary in an album title", "icloud_catalog.json", `[{"uuid": "A", "album_info": [{"title": ["` + canary + `"]}]}]`, "read catalog: wrong type (array)"},
		{"canary in width", "yandex_inventory.json", `{"files": [{"path": "2023/x.jpg", "width": "` + canary + `"}]}`, "read inventory: wrong type (number)"},
		{"canary in a links value", "plan-rerun/links.json", `{"` + canary + `": [["a", 5]]}`, "read links: wrong type (number)"},
		{"canary as a links key", "plan-rerun/links.json", `{"` + canary + `": 7}`, "read links: wrong type (number)"},
		{"canary in a link kind", "plan-rerun/links.json", `{"A": [["a", "` + canary + `"]]}`, "read links: invalid value in field links"},
		{"missing uuid", "icloud_catalog.json", `[{"original_filename": "` + canary + `.jpg"}]`, "read catalog: missing field uuid in record 1"},
		{"absolute path", "yandex_inventory.json", `{"files": [{"path": "/Users/` + canary + `/x.jpg"}]}`, "read inventory: invalid value in field files.path in record 1"},
		{"canary in live_pair", "yandex_inventory.json", `{"files": [{"path": "2023/x.jpg"}, {"path": "2023/y.jpg", "live_pair": "` + canary + `"}]}`, "read inventory: invalid value in field files.live_pair in record 2"},
		{"catalog not an array", "icloud_catalog.json", `{"` + canary + `": 1}`, "read catalog: wrong type (object)"},
		{"trailing data", "plan/plan.json", `{} "` + canary + `"`, "read plan: syntax error at offset 4"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in, out, salt, c := setup(t, corpusOptions{})
			writeFile(t, in, tt.file, []byte(tt.content))
			code, stdout, stderr := runTool("--in", in, "--out", out, "--salt-file", salt)
			if code != 1 || !strings.Contains(stderr, tt.kind) {
				t.Errorf("code %d, stderr %q; want %q", code, stderr, tt.kind)
			}
			for _, cn := range append(c.canaries, canary, "Пельменева", "55.7539") {
				if strings.Contains(stdout+stderr, cn) {
					t.Errorf("output holds a canary: %q", stdout+stderr)
				}
			}
		})
	}
}

func TestSafely(t *testing.T) {
	t.Parallel()
	var e bytes.Buffer
	code := safely(func() int { panic("Зинаида Пельменева /Users/zinaida") }, &e)
	if code != 1 || e.String() != "anonymize: internal error\n" {
		t.Errorf("code %d, stderr %q", code, e.String())
	}
	e.Reset()
	if code := safely(func() int { return 2 }, &e); code != 2 || e.Len() != 0 {
		t.Errorf("code %d, stderr %q", code, e.String())
	}
}

func TestCheckFindings(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	salt := filepath.Join(t.TempDir(), "salt.key")
	saltHex := strings.Repeat("0123456789abcdef", 4)
	writeFile(t, filepath.Dir(salt), "salt.key", []byte(saltHex+"\n"))
	writeFile(t, dir, "anon-unmatched-icloud/case.json", []byte(`{"icloud": [{"original_filename": "Пельменева.jpg", "uuid": "3F2A8C41-9D07-4B1E-8A0C-77F25B1E6A0C"}], "note": "`+saltHex+`"}`))
	writeFile(t, dir, "anon-unmatched-icloud/notes.txt", []byte("Пельменева"))
	writeFile(t, dir, "anon-y/broken.json", []byte(`{"a": `))
	writeFile(t, dir, "anon-y/empty.json", nil)
	writeFile(t, dir, "Пельменева/case.json", []byte(`{"note": 1}`))
	// The form of an output name with a private string that is no category.
	writeFile(t, dir, "anon-zinaida/case.json", []byte(`{"note": 1}`))
	if err := os.Symlink("case.json", filepath.Join(dir, "anon-unmatched-icloud", "link.json")); err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runTool("--check", dir, "--salt-file", salt)
	if code != 1 || stderr != "" {
		t.Errorf("code %d, stderr %q", code, stderr)
	}
	for _, want := range []string{
		"tokens.json: : no-dictionary", "anon-unmatched-icloud/case.json: /icloud/0/original_filename: word",
		"anon-unmatched-icloud/case.json: /icloud/0/uuid: uuid", "anon-unmatched-icloud/case.json: : salt",
		"anon-unmatched-icloud/case.json: /<key>: unexpected-key",
	} {
		if !strings.Contains(stdout, want+"\n") {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
	// Names the build never writes are not printed.
	for line, n := range map[string]int{
		"<file>: /<key>: unexpected-key\n": 2, // Пельменева/case.json and anon-zinaida/case.json
		"<file>: : unexpected-file\n":      2, // notes.txt and the symbolic link
		"<file>: : not-json\n":             2, // broken.json and empty.json
	} {
		if got := strings.Count(stdout, line); got != n {
			t.Errorf("%d lines %q; want %d:\n%s", got, line, n, stdout)
		}
	}
	for _, name := range []string{"notes.txt", "empty.json", "broken.json", "link.json", "zinaida"} {
		if strings.Contains(stdout, name) {
			t.Errorf("stdout holds the file name %q", name)
		}
	}
	if !regexp.MustCompile(`(?m)^check: \d+ findings in 7 files$`).MatchString(stdout) {
		t.Errorf("summary: %q", stdout)
	}
	for _, v := range []string{"Пельменева", "3F2A8C41", saltHex} {
		if strings.Contains(stdout, v) {
			t.Errorf("stdout holds a value %q", v)
		}
	}
	if code, _, _ := runTool("--check", filepath.Join(dir, "missing")); code != 1 {
		t.Errorf("check of a missing directory: code %d", code)
	}
}

func TestCheckBrokenDictionary(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeFile(t, dir, "tokens.json", []byte(`{"format": 1, "words": [`))
	code, stdout, stderr := runTool("--check", dir)
	if code != 1 || stderr != "" || stdout != "tokens.json: : not-json\ncheck: 1 findings in 1 files\n" {
		t.Errorf("code %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}
