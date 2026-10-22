//go:build fixtures_verify

package main

import (
	"flag"
	"os"
	"strings"
	"testing"
)

var (
	materialsDir = flag.String("anonymize.in", "", "absolute path to the manual-sync materials")
	fixturesDir  = flag.String("anonymize.out", "", "absolute path to the fixtures written by the tool")
)

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

// TestFixturesNoInputWords cross-checks the fixtures with the real
// materials: no input word of four and more letters is in the fixtures,
// alone or glued to other letters. Equal words of three letters and
// pseudonyms that happen to equal an input word are counted only. The log
// holds aggregates only.
func TestFixturesNoInputWords(t *testing.T) {
	if !isDir(*materialsDir) || !isDir(*fixturesDir) {
		t.Skip("pass -anonymize.in and -anonymize.out")
	}
	input := inputWords(materialOf(t, *materialsDir))
	files, err := readOutput(*fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	dict := dictionaryOf(files)
	r := crossCheck(input, dict, files)
	t.Logf("files=%d dictionary=%d; %s", len(files), len(dict), r)
	if len(dict) == 0 {
		t.Error("no dictionary of generated words in the fixtures")
	}
	if r.Failed() {
		t.Errorf("input words in the fixtures: %d equal, %d glued", r.Hits[class4], r.Glued)
	}
}

// TestFixturesDigest prints a one-screen sample of the fixtures for the
// owner: anonymized values only.
func TestFixturesDigest(t *testing.T) {
	if !isDir(*fixturesDir) {
		t.Skip("pass -anonymize.out")
	}
	files, err := readOutput(*fixturesDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("\n%s", strings.Join(digest(files, 30), "\n"))
}
