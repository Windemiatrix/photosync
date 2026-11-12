package plan_test

import (
	"maps"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Windemiatrix/photosync/internal/domain/naming"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// A quarantined file keeps its place under the date, so that undo and the
// user find it; two files quarantined from one place on one day take two
// paths; and the longest path that names accept still fits the limit there.
func TestQuarantinePath(t *testing.T) {
	t.Parallel()
	const (
		file = "2024/05-01 - Holidays/IMG_0001.HEIC"
		at   = ".photosync/quarantine/2026-11-12/" + file
		one  = ".photosync/quarantine/2026-11-12/2024/05-01 - Holidays/IMG_0001 (1).HEIC"
		dot  = ".photosync/quarantine/2026-11-12/2024/05-01 - St. Ives/IMG_0001"
	)
	noon := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	taken := func(paths ...string) map[string]bool {
		m := map[string]bool{}
		for _, p := range paths {
			m[p] = true
		}
		return m
	}
	for _, c := range []struct {
		name  string
		day   time.Time
		p     string
		taken map[string]bool
		want  string
	}{
		{"a file to quarantine", noon, file, nil, at}, // covers: R-STATUS-01
		{"its place quarantined earlier that day", noon, file, taken(at), one},
		{"the first number that is free", noon, file, taken(at, one), ".photosync/quarantine/2026-11-12/2024/05-01 - Holidays/IMG_0001 (2).HEIC"},
		{"its place quarantined another day", noon, file, taken(".photosync/quarantine/2026-11-11/" + file), at},
		{"a name without an extension under a directory with a dot", noon, "2024/05-01 - St. Ives/IMG_0001", taken(dot), dot + " (1)"},
		{"the UTC date of a day in another time zone", time.Date(2026, 11, 12, 23, 30, 0, 0, time.FixedZone("UTC-5", -5*60*60)), file, nil,
			".photosync/quarantine/2026-11-13/" + file},
	} {
		before := maps.Clone(c.taken)
		if got := plan.QuarantinePath(c.day, c.p, c.taken); got != c.want {
			t.Errorf("%s: QuarantinePath = %q; want %q", c.name, got, c.want)
		}
		if !maps.Equal(c.taken, before) {
			t.Errorf("%s: QuarantinePath changed taken to %v", c.name, c.taken)
		}
	}
	level := strings.Repeat("b", 247)
	longest := strings.Join([]string{level, level, level, level}, "/")
	if !naming.IsValidPath(longest) || naming.IsValidPath(longest+"b") {
		t.Errorf("naming accepts %d characters: %v, %d: %v; want true and false", 991, naming.IsValidPath(longest), 992, naming.IsValidPath(longest+"b"))
	}
	if n := utf8.RuneCountInString(plan.QuarantinePath(noon, longest, nil)); n != 1024 {
		t.Errorf("the longest path that names accept is %d characters in quarantine; want 1024", n)
	}
}
