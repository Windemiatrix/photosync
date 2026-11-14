package plan_test

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/Windemiatrix/photosync/internal/domain"
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

// Each row is the whole listing of the remotes of a run. A file leaves the
// quarantine of its root for good once it has stayed there longer than the
// policy says, counted by the UTC date of its folder there. The expected
// operations follow that rule, not the code, and must not depend on the
// order of the input.
func TestExpireOps(t *testing.T) {
	t.Parallel()
	const personal, shared = domain.LibraryPersonal, domain.LibraryShared
	type (
		scans = map[string][]domain.RemoteEntry
		specs = []domain.RemoteSpec
		ops   = []domain.Op
	)
	// A day ahead of UTC, so that the clock of the run would take one day
	// more than the policy says.
	now := time.Date(2026, 11, 14, 1, 30, 0, 0, time.FixedZone("UTC+3", 3*60*60))
	alpha, beta := domain.RemoteSpec{Name: "alpha", Mode: domain.ModeTwoWay}, domain.RemoteSpec{Name: "beta", Mode: domain.ModeDownloadOnly}
	file := func(root domain.LibraryKind, at string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: root, Path: at, Size: 10, MD5: "q1", MTime: now.Add(-time.Hour)}
	}
	dir := func(at string) domain.RemoteEntry {
		return domain.RemoteEntry{Root: personal, Path: at, IsDir: true, MTime: now.Add(-time.Hour)}
	}
	inQuarantine := func(day, name string) string {
		return ".photosync/quarantine/" + day + "/2024/05-01 - Holidays/" + name
	}
	over, last := file(personal, inQuarantine("2026-08-14", "IMG_0001.HEIC")), file(personal, inQuarantine("2026-08-15", "IMG_0002.HEIC"))
	older, yesterday := file(personal, inQuarantine("2026-11-11", "IMG_0003.HEIC")), file(personal, inQuarantine("2026-11-12", "IMG_0004.HEIC"))
	inShared, inPersonal := file(shared, inQuarantine("2026-08-01", "IMG_0011.HEIC")), file(personal, inQuarantine("2026-08-01", "IMG_0011.HEIC"))
	inBeta, inGamma := file(personal, inQuarantine("2026-08-02", "IMG_0012.HEIC")), file(personal, inQuarantine("2026-08-03", "IMG_0013.HEIC"))
	expire := func(remote string, e domain.RemoteEntry) domain.Op {
		return domain.Op{Kind: domain.OpExpire, Remote: remote, Root: e.Root, Src: e.Path, Rule: "R-STATUS-09", Irreversible: true}
	}
	for _, c := range []struct {
		name    string
		scans   scans
		remotes specs
		keep    int
		want    ops
	}{
		{"nothing at all", nil, nil, 90, nil},
		{"a file kept longer than the policy says leaves the cloud for good, and one kept as long stays",
			scans{"alpha": {last, over}}, specs{alpha}, 90, ops{expire("alpha", over)}}, // covers: R-STATUS-09
		{"under a single day a file of the day before yesterday leaves, and one of yesterday stays",
			scans{"alpha": {yesterday, older}}, specs{alpha}, 1, ops{expire("alpha", older)}},
		{"a folder of the quarantine is no file to delete",
			scans{"alpha": {dir(".photosync/quarantine/2026-08-01/2024"), dir(".photosync/quarantine/2026-08-01")}}, specs{alpha}, 90, nil},
		{"a name that is no date of the quarantine keeps its file",
			scans{"alpha": {
				file(personal, inQuarantine("2026-02-30", "IMG_0005.HEIC")), file(personal, inQuarantine("2026-8-01", "IMG_0006.HEIC")),
				file(personal, ".photosync/quarantine/2026-08-01"),
			}}, specs{alpha}, 90, nil},
		{"a file outside the quarantine stays, whatever its folders are named",
			scans{"alpha": {
				file(personal, "2026-08-01/IMG_0007.HEIC"), file(personal, ".photosync/tmp/2026-08-01/IMG_0008.HEIC"),
				file(personal, "Albums/.photosync/quarantine/2026-08-01/IMG_0009.HEIC"), file(personal, ".photosync/Quarantine/2026-08-01/IMG_0010.HEIC"),
			}}, specs{alpha}, 90, nil},
		{"each remote of the run loses the expired files of both its roots, and a remote outside the run keeps its own",
			scans{"alpha": {inShared, inPersonal}, "beta": {inBeta}, "gamma": {inGamma}}, specs{beta, alpha}, 90,
			ops{expire("alpha", inPersonal), expire("alpha", inShared), expire("beta", inBeta)}},
		{"a policy that keeps files no day at all expires nothing",
			scans{"alpha": {older, yesterday}}, specs{alpha}, 0, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			input := fmt.Sprintf("%+v %+v", c.scans, c.remotes)
			got := plan.ExpireOps(c.scans, c.remotes, c.keep, now)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("ExpireOps =\n%+v\nwant\n%+v", got, c.want)
			}
			for _, o := range c.want {
				if _, ok := domain.Rules[o.Rule]; !ok {
					t.Errorf("%s is not in domain.Rules", o.Rule)
				}
			}
			reversed, remotes := scans{}, slices.Clone(c.remotes)
			for name, es := range c.scans {
				reversed[name] = slices.Clone(es)
				slices.Reverse(reversed[name])
			}
			slices.Reverse(remotes)
			if again := plan.ExpireOps(reversed, remotes, c.keep, now); !reflect.DeepEqual(again, got) {
				t.Errorf("with the scans and the remotes reversed:\n%+v\nwant\n%+v", again, got)
			}
			if after := fmt.Sprintf("%+v %+v", c.scans, c.remotes); after != input {
				t.Errorf("ExpireOps changed its input:\n%s\nwant\n%s", after, input)
			}
		})
	}
}
