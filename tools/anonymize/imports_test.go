package main

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The tool has no network, starts no processes, does not log and reads no
// environment; the pure package does no I/O at all.
var (
	forbiddenEverywhere = []string{"net", "os/exec", "syscall", "unsafe", "plugin", "log", "log/slog", "crypto/tls"}
	forbiddenInAnon     = []string{"os", "io/fs", "path/filepath", "crypto/rand"}
	allowedModules      = []string{"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon", "golang.org/x/text/unicode/norm"}
	forbiddenCalls      = []string{"time.Now", "os.Getenv", "os.LookupEnv", "os.Environ", "os.Setenv", "fmt.Print", "os.TempDir", "os.CreateTemp"}
)

func TestImports(t *testing.T) {
	t.Parallel()
	n := 0
	err := filepath.WalkDir(".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") {
			return err
		}
		n++
		f, err := parser.ParseFile(token.NewFileSet(), p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		inAnon := strings.HasPrefix(filepath.ToSlash(p), "internal/anon/")
		for _, im := range f.Imports {
			path, _ := strconv.Unquote(im.Path.Value)
			for _, bad := range forbiddenEverywhere {
				if path == bad || strings.HasPrefix(path, "net/") {
					t.Errorf("%s imports %s", p, path)
				}
			}
			if inAnon {
				for _, bad := range forbiddenInAnon {
					if path == bad {
						t.Errorf("%s imports %s", p, path)
					}
				}
			}
			if first := strings.SplitN(path, "/", 2)[0]; strings.Contains(first, ".") && !contains(allowedModules, path) {
				t.Errorf("%s imports a module outside the whitelist: %s", p, path)
			}
		}
		if strings.HasSuffix(p, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		for _, call := range forbiddenIn(string(src)) {
			t.Errorf("%s calls %s", p, call)
		}
		if strings.Contains(string(src), "panic(") {
			t.Errorf("%s calls panic", p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if n < 10 {
		t.Errorf("only %d Go files found", n)
	}
}

// forbiddenIn returns the forbidden calls in a source: a name of the list
// with any suffix of letters (fmt.Print, fmt.Printf, fmt.Println).
func forbiddenIn(src string) []string {
	var out []string
	for _, call := range forbiddenCalls {
		if regexp.MustCompile(regexp.QuoteMeta(call) + `[A-Za-z]*\(`).MatchString(src) {
			out = append(out, call)
		}
	}
	return out
}

func TestForbiddenIn(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`fmt.Print("x")`, `fmt.Printf("%d", 1)`, `fmt.Println(x)`, `os.Getenv("X")`} {
		if len(forbiddenIn(src)) != 1 {
			t.Errorf("forbiddenIn(%q) = %v", src, forbiddenIn(src))
		}
	}
	for _, src := range []string{`fmt.Fprintf(w, "x")`, `fmt.Sprintf("x")`, `b.Print`} {
		if got := forbiddenIn(src); len(got) != 0 {
			t.Errorf("forbiddenIn(%q) = %v", src, got)
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
