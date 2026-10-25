//go:build fixtures_verify

package main

import (
	"bytes"
	"flag"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var (
	materialsDir = flag.String("anonymize.in", "", "absolute path to the manual-sync materials")
	saltFile     = flag.String("anonymize.salt", "", "absolute path to the salt file the fixtures were built with")
)

const committedFixtures = "../../testdata/fixtures"

func isDir(p string) bool {
	st, err := os.Stat(p)
	return p != "" && err == nil && st.IsDir()
}

func TestFixturesVerify(t *testing.T) {
	if !isDir(*materialsDir) {
		t.Skip("no materials directory: pass -anonymize.in")
	}
	verifyMaterials(t, *materialsDir)
}

// TestFixturesRebuild fails when a change of the tool changed the fixtures.
// The log names the differing files: the names are anonymized.
func TestFixturesRebuild(t *testing.T) {
	if !isDir(*materialsDir) || *saltFile == "" {
		t.Skip("pass -anonymize.in and -anonymize.salt")
	}
	if _, err := os.Stat(*saltFile); err != nil {
		t.Fatal("salt file: not readable")
	}
	out := filepath.Join(t.TempDir(), "fixtures")
	if err := build(*materialsDir, out, *saltFile, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	want, err := readTree(committedFixtures)
	if err != nil {
		t.Fatal(err)
	}
	got, err := readTree(out)
	if err != nil {
		t.Fatal(err)
	}
	var differ []string
	for name, w := range want {
		if g, ok := got[name]; !ok || !bytes.Equal(g, w) {
			differ = append(differ, name)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			differ = append(differ, name)
		}
	}
	slices.Sort(differ)
	t.Logf("files=%d differ=%d", len(want), len(differ))
	if len(differ) > 0 {
		t.Errorf("rebuilt fixtures differ from testdata/fixtures: %s", strings.Join(differ, ", "))
	}
}

// The log of TestFixturesNoInputWords holds aggregates only.
func TestFixturesNoInputWords(t *testing.T) {
	if !isDir(*materialsDir) {
		t.Skip("no materials directory: pass -anonymize.in")
	}
	input := inputWords(materialOf(t, *materialsDir))
	files, err := readTree(committedFixtures)
	if err != nil {
		t.Fatal(err)
	}
	dict := dictionaryOf(files)
	r := crossCheck(input, dict, files)
	t.Logf("files=%d dictionary=%d; %s", len(files), len(dict), r)
	if len(dict) == 0 {
		t.Error("no dictionary of generated words in the fixtures")
	}
	if r.Leaks4PlusOrGluedWord() {
		t.Errorf("input words in the fixtures: %d equal, %d glued", r.Hits[class4Plus], r.Glued)
	}
}

// The owner reads the log of TestFixturesDigest: anonymized values only.
func TestFixturesDigest(t *testing.T) {
	files, err := readTree(committedFixtures)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("\n%s", strings.Join(digest(files, 30), "\n"))
}
