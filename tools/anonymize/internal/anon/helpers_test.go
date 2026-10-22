package anon

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// testSalt is a fixed salt: tests that compare outputs are deterministic.
var testSalt = []byte("0123456789abcdef0123456789abcdef")

func newTestMapper(t *testing.T) *Mapper {
	t.Helper()
	m, err := NewMapper(testSalt)
	if err != nil {
		t.Fatalf("NewMapper: %v", err)
	}
	return m
}

// counterMAC is a fake MAC that ignores the input: every key gets the same
// output for the same counter, so the second key always collides once.
func counterMAC(_ string, _ []byte, counter uint64) [32]byte {
	var b [8]byte
	binary.BigEndian.PutUint64(b[:], counter)
	return sha256.Sum256(b[:])
}

// constantMAC is a fake MAC with a single output: the space is exhausted for
// the second key.
func constantMAC(string, []byte, uint64) [32]byte { return sha256.Sum256([]byte("x")) }

// caseMask returns the upper-case pattern of s rune by rune.
func caseMask(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsUpper(r) {
			b.WriteByte('U')
		} else {
			b.WriteByte('l')
		}
	}
	return b.String()
}

func runeLen(s string) int { return utf8.RuneCountInString(s) }

// testMaterial is a small consistent material: assets, files, links and a
// plan derived from them.
type testMaterial struct {
	assets []Asset
	files  []File
	links  Links
	// replace and keep list the (asset index, path) pairs with a plan decision.
	replace [][2]string
	keep    []string
}

func (tm testMaterial) build() *Material {
	m := &Material{Assets: tm.assets, Files: tm.files, Links: tm.links}
	for _, f := range tm.files {
		stem, _ := Splitext(BaseName(f.Path))
		fi := FolderInfoOf(f.Path)
		m.Derived = append(m.Derived, Derived{Stem: stem, Folder: fi.Folder, Year: fi.Year, EventMonth: fi.EventMonth})
	}
	linked := map[string]bool{}
	for _, ls := range tm.links {
		for _, l := range ls {
			linked[l.Path] = true
		}
	}
	for _, a := range tm.assets {
		if !a.InTrash && len(tm.links[a.UUID]) == 0 {
			m.Plan.Download = append(m.Plan.Download, PlanDownload{UUID: a.UUID})
		}
	}
	for _, f := range tm.files {
		if IsMedia(f.Kind) && !linked[f.Path] {
			m.Plan.Upload = append(m.Plan.Upload, PlanUpload{Path: f.Path})
		}
	}
	for _, r := range tm.replace {
		for _, l := range tm.links[r[0]] {
			if l.Path == r[1] {
				m.Plan.Replace = append(m.Plan.Replace, PlanReplace{UUID: r[0], YandexPath: r[1], Match: l.Kind})
			}
		}
	}
	for _, a := range tm.assets {
		for _, l := range tm.links[a.UUID] {
			if l.Kind == LevelProbable {
				m.Plan.Conflicts = append(m.Plan.Conflicts, []string{ConflictProbableMatch, a.Name(), l.Path, "note"})
			}
		}
	}
	for _, p := range tm.keep {
		m.Plan.Conflicts = append(m.Plan.Conflicts, []string{ConflictYandexBetter, "x", p, "note"})
	}
	m.Plan.Conflicts = append(m.Plan.Conflicts, []string{"yandex_duplicate", "a", "b", "note"})
	return m
}

func uuidN(n int) string { return fmt.Sprintf("1A2B3C4D-0000-4000-8000-%012d", n) }

func photo(n int, name, date string, size int64) Asset {
	return Asset{UUID: uuidN(n), OriginalFilename: name, Filename: uuidN(n) + ".heic", Date: date, TZOffset: 10800,
		OriginalFilesize: size, OriginalWidth: 4032, OriginalHeight: 3024, Width: 4032, Height: 3024, Albums: []AlbumRef{}}
}

func cloudFile(path, date string, size int64) File {
	return File{Path: path, Kind: KindOf(BaseName(path)), Size: size, DateTaken: date, Width: 4032, Height: 3024}
}
