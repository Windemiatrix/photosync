package testkit_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"maps"
	"os"
	"path"
	"slices"
	"strings"
	"testing"
)

const scanRoot = "github.com/Windemiatrix/photosync/internal"

// Only these adapters may read the value of a Secret: they send it to the
// cloud and keep it in the token file.
var readerDirs = []string{"adapters/yandexrest/"}

// fmt shows a Secret as *** only through its Format method, which it does not
// call on a value reached through an unexported field; a conversion, an index
// and a range give the value itself.
func TestSecretScan(t *testing.T) {
	t.Parallel()
	for _, f := range secretScan(t, os.DirFS(".."), ".go") {
		t.Errorf("%s; see the doc comment of domain.Secret", f)
	}
}

// The scan of internal finds nothing, so a tree that breaks each rule shows
// that it would. The lines of the fixture that are not listed do not leak: an
// exported field, a pointer, a recursive type without a Secret, a conversion
// to Secret, to its alias or to an interface, a slice of a Secret, its length,
// a comparison, an index of a type defined from Secret and the reads in an
// adapter that may read.
func TestSecretScanFixture(t *testing.T) {
	t.Parallel()
	got := secretScan(t, os.DirFS("testdata/secretscan"), ".go.txt")
	want := []string{
		// An adapter that may read the value may not print it.
		"adapters/yandexrest/client.go.txt:6: unexported field holds a Secret",
		// Its name only starts like that of an adapter that may read.
		"adapters/yandexrest2/client.go.txt:6: reads a Secret",
		// A field of another package of the tree, read from a test package.
		"leak/external.go.txt:6: reads a Secret",
		"leak/leak.go.txt:15: unexported field holds a Secret",
		"leak/leak.go.txt:16: unexported field holds a Secret",
		"leak/leak.go.txt:17: unexported field holds a Secret",
		"leak/leak.go.txt:18: unexported field holds a Secret",
		"leak/leak.go.txt:19: unexported field holds a Secret",
		"leak/leak.go.txt:20: unexported field holds a Secret",
		"leak/leak.go.txt:30: unexported field holds a Secret",
		"leak/leak.go.txt:33: unexported field holds a Secret",
		"leak/leak.go.txt:41: reads a Secret",
		"leak/leak.go.txt:42: reads a Secret",
		// A type defined from Secret has no Format method.
		"leak/leak.go.txt:43: reads a Secret",
		// An argument of a call into a package outside the tree.
		"leak/leak.go.txt:44: reads a Secret",
		"leak/leak.go.txt:45: reads a Secret",
		"leak/leak.go.txt:46: reads a Secret",
		"leak/leak.go.txt:55: unexported field holds a Secret",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("missing %s", w)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			t.Errorf("unexpected %s", g)
		}
	}
}

type secretTree struct {
	fset  *token.FileSet
	files map[string][]*ast.File
	pkgs  map[string]*types.Package
	info  *types.Info
}

// A package outside the tree is not read: when Import fails, the checker goes
// on with an empty package. None of those can name a Secret, and the type
// errors of their uses do not matter to the scan.
func (tr *secretTree) Import(p string) (*types.Package, error) {
	if pkg, ok := tr.pkgs[p]; ok {
		return pkg, nil
	}
	files, ok := tr.files[p]
	if !ok {
		return nil, fs.ErrNotExist
	}
	pkg := types.NewPackage(p, "")
	tr.pkgs[p] = pkg
	conf := types.Config{Importer: tr, Error: func(error) {}}
	_ = types.NewChecker(&conf, tr.fset, pkg, tr.info).Files(files)
	return pkg, nil
}

// Test packages are scanned too, since a test log reaches CI, and build
// constraints are ignored, so that the darwin files are scanned on Linux.
func secretScan(t *testing.T, fsys fs.FS, ext string) []string {
	t.Helper()
	tr := &secretTree{
		fset:  token.NewFileSet(),
		files: map[string][]*ast.File{},
		pkgs:  map[string]*types.Package{},
		info:  &types.Info{Types: map[ast.Expr]types.TypeAndValue{}},
	}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ext) {
			return err
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(tr.fset, p, src, 0)
		if err != nil {
			return err
		}
		key := path.Join(scanRoot, path.Dir(p))
		if strings.HasSuffix(f.Name.Name, "_test") {
			key += "_test"
		}
		tr.files[key] = append(tr.files[key], f)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range slices.Sorted(maps.Keys(tr.files)) {
		_, _ = tr.Import(key)
	}
	obj := tr.pkgs[scanRoot+"/domain"].Scope().Lookup("Secret")
	if obj == nil {
		t.Fatal("the tree has no type Secret in its domain package")
	}
	secret := obj.Type()
	isSecret := func(e ast.Expr) bool {
		tv, ok := tr.info.Types[e]
		return ok && types.Identical(tv.Type, secret)
	}
	var holds func(typ types.Type, seen map[types.Type]bool) bool
	holds = func(typ types.Type, seen map[types.Type]bool) bool {
		if types.Identical(typ, secret) {
			return true
		}
		if seen[typ] {
			return false
		}
		seen[typ] = true
		switch u := typ.Underlying().(type) {
		case *types.Struct:
			for v := range u.Fields() {
				if holds(v.Type(), seen) {
					return true
				}
			}
		case *types.Array:
			return holds(u.Elem(), seen)
		case *types.Slice:
			return holds(u.Elem(), seen)
		case *types.Map:
			return holds(u.Key(), seen) || holds(u.Elem(), seen)
		}
		return false
	}
	var found []string
	report := func(pos token.Pos, what string) {
		at := tr.fset.Position(pos)
		found = append(found, fmt.Sprintf("%s:%d: %s", at.Filename, at.Line, what))
	}
	for _, files := range tr.files {
		for _, f := range files {
			name := tr.fset.Position(f.Pos()).Filename
			mayRead := slices.ContainsFunc(readerDirs, func(dir string) bool {
				return strings.HasPrefix(name, dir)
			})
			read := func(n ast.Node) {
				if !mayRead {
					report(n.Pos(), "reads a Secret")
				}
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch n := n.(type) {
				case *ast.StructType:
					if st, ok := tr.info.TypeOf(n).(*types.Struct); ok {
						for v := range st.Fields() {
							if !v.Exported() && holds(v.Type(), map[types.Type]bool{}) {
								report(v.Pos(), "unexported field holds a Secret")
							}
						}
					}
				case *ast.CallExpr:
					tv := tr.info.Types[n.Fun]
					convert := tv.IsType() && !types.Identical(tv.Type, secret) && !types.IsInterface(tv.Type)
					if convert && slices.ContainsFunc(n.Args, isSecret) {
						read(n)
					}
				case *ast.IndexExpr:
					if isSecret(n.X) {
						read(n)
					}
				case *ast.RangeStmt:
					if isSecret(n.X) {
						read(n)
					}
				}
				return true
			})
		}
	}
	slices.Sort(found)
	return found
}
