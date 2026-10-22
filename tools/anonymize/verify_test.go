package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/tools/anonymize/internal/anon"
)

// verifyMaterials checks the tool on a materials directory: the build
// succeeds with a fresh salt, the material passes the rerun and partition
// checks, the image of every supported unit (except those the mapping runs
// out of space for, counted in the log) keeps every relation, the expected
// pairs of each case keep the level distribution of the reference links of
// its source assets, and the output passes the leak scanner. Logs and
// messages hold aggregates and counters only; nothing is written outside
// t.TempDir().
func verifyMaterials(t *testing.T, dir string) {
	t.Helper()
	tmp := t.TempDir()
	saltPath := filepath.Join(tmp, "keys", "anonymize.key")
	out := filepath.Join(tmp, "out")
	code, stdout, stderr := runTool("--in", dir, "--out", out, "--salt-file", saltPath)
	if code != 0 {
		t.Fatalf("build: code %d: %s", code, stderr)
	}
	t.Logf("build:\n%s", stdout)

	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal("cannot open the materials directory")
	}
	defer closeQuietly(root)
	m, _, err := readMaterial(root.FS())
	if err != nil {
		t.Fatalf("material: %v", err)
	}
	if c := anon.CheckPartition(m); !c.Zero() {
		t.Fatalf("partition: %s", c)
	}
	salt, _, err := loadSalt(saltPath, false)
	if err != nil {
		t.Fatalf("salt: %v", err)
	}

	mp, err := anon.NewMapper(salt)
	if err != nil {
		t.Fatal(err)
	}
	units, err := anon.SupportedUnits(m)
	if err != nil {
		t.Fatalf("units: %v", err)
	}
	// One mapping unit by unit, as the build does: a unit the mapping runs
	// out of space for is skipped; the mapping of the union only repeats
	// registered outputs.
	var mapped [][]int
	exhausted := 0
	for _, u := range units {
		_, err := mp.Set(m.Subset(u))
		switch {
		case errors.Is(err, anon.ErrExhausted):
			exhausted++
		case err != nil:
			t.Fatalf("mapping of a supported unit: %v", err)
		default:
			mapped = append(mapped, u)
		}
	}
	recs := flatten(mapped)
	set := m.Subset(recs)
	img, err := mp.Set(set)
	if err != nil {
		t.Fatalf("mapping of all mapped units: %v", err)
	}
	if vs := anon.CheckRelations(set, img, mp.YearShift()); len(vs) > 0 {
		t.Errorf("relations on %d records: %v", len(recs), vs)
	}
	uuids, paths := map[string]bool{}, map[string]bool{}
	for _, a := range img.Assets {
		uuids[a.UUID] = true
	}
	for _, f := range img.Files {
		paths[f.Path] = true
	}
	if len(uuids) != len(img.Assets) || len(paths) != len(img.Files) {
		t.Errorf("mapping is not injective: %d of %d UUIDs, %d of %d paths", len(uuids), len(img.Assets), len(paths), len(img.Files))
	}
	t.Logf("relations: units=%d exhausted=%d records=%d assets=%d files=%d", len(units), exhausted, len(recs), len(img.Assets), len(img.Files))

	mp2, _ := anon.NewMapper(salt)
	res, err := anon.Build(m, mp2)
	if err != nil {
		t.Fatalf("build in memory: %v", err)
	}
	for _, cr := range res.Cases {
		p := filepath.Join(out, "anon-"+cr.Category, "case.json")
		data, err := os.ReadFile(p)
		if cr.Case == nil {
			if err == nil {
				t.Errorf("case %s: written but empty", cr.Category)
			}
			continue
		}
		if err != nil {
			t.Errorf("case %s: not written", cr.Category)
			continue
		}
		encoded, err := encodeJSON(cr.Case)
		if err != nil || !bytes.Equal(data, encoded) {
			t.Errorf("case %s: the written case differs from the build in memory", cr.Category)
		}
		var c anon.Case
		if err := json.Unmarshal(data, &c); err != nil {
			t.Errorf("case %s: does not parse", cr.Category)
			continue
		}
		// The level distribution of the written pairs equals the one of the
		// reference links of the case's source assets.
		var got, want [6]int
		for _, p := range c.Expected {
			got[levelIndex(p.Level)]++
		}
		for _, r := range cr.Src {
			if r >= len(m.Assets) {
				continue
			}
			for _, l := range m.Links[m.Assets[r].UUID] {
				level, _ := anon.LevelOf(l.Kind)
				want[levelIndex(level)]++
			}
		}
		if got != want {
			t.Errorf("case %s: levels %v; want %v", cr.Category, got, want)
		}
	}

	findings, files, err := scanDir(out, salt)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(findings) > 0 {
		rules := map[string]int{}
		for _, f := range findings {
			rules[f.Rule]++
		}
		t.Errorf("scanner: %d findings in %d files: %v", len(findings), files, rules)
	}
	if strings.Contains(stdout, dir) {
		t.Error("stdout holds the materials path")
	}
}

// levelIndex returns the position of a level in anon.Levels, or the last
// position for an unknown one (it then fails the comparison).
func levelIndex(level string) int {
	for i, l := range anon.Levels {
		if l == level {
			return i
		}
	}
	return len(anon.Levels) - 1
}

func flatten(units [][]int) []int {
	var out []int
	for _, u := range units {
		out = append(out, u...)
	}
	sort.Ints(out)
	return out
}

func TestVerifyCorpus(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeCorpus(t, dir)
	verifyMaterials(t, dir)
}
