package plan

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// QuarantinePath returns the path, relative to the remote root, that the
// cloud file at p, relative to the same root, takes in quarantine on day: p
// under .photosync/quarantine/YYYY-MM-DD/, the UTC date of day. While taken
// holds that path, " (N)" with the first N that frees it goes before the
// extension of the last element. taken holds the quarantine paths of the
// root that the state and the plan already give to files, and the caller
// adds each result before the next call. A p that naming.IsValidPath accepts
// gives at most 1024 characters before a number.
func QuarantinePath(day time.Time, p string, taken map[string]bool) string {
	q := ".photosync/quarantine/" + day.UTC().Format(time.DateOnly) + "/" + p
	ext := path.Ext(q)
	stem := strings.TrimSuffix(q, ext)
	for n := 1; taken[q]; n++ {
		q = fmt.Sprintf("%s (%d)%s", stem, n, ext)
	}
	return q
}

// ExpireOps returns the operations that delete for good, under R-STATUS-09,
// each file of the quarantine of each of remotes that has stayed there longer
// than keepDays days. scans give by the name of a remote its whole listing. A
// file of the quarantine lies at .photosync/quarantine/YYYY-MM-DD/ followed by
// its path, the date that QuarantinePath gives it; it has stayed too long once
// that date is before the UTC date of now less keepDays days. A directory,
// and a file whose path names no such date, stays; a keepDays below 1, which
// Policies.Validate rejects, expires nothing. The operations are in the
// order of domain.Op.Compare, and the inputs are not changed.
func ExpireOps(scans map[string][]domain.RemoteEntry, remotes []domain.RemoteSpec, keepDays int, now time.Time) []domain.Op {
	if keepDays < 1 {
		return nil
	}
	limit := now.UTC().AddDate(0, 0, -keepDays).Format(time.DateOnly)
	var ops []domain.Op
	for _, r := range remotes {
		for _, e := range scans[r.Name] {
			rest, quarantined := strings.CutPrefix(e.Path, ".photosync/quarantine/")
			day, _, dated := strings.Cut(rest, "/")
			_, err := time.Parse(time.DateOnly, day)
			if e.IsDir || !quarantined || !dated || err != nil || day >= limit {
				continue
			}
			ops = append(ops, domain.Op{Kind: domain.OpExpire, Remote: r.Name, Root: e.Root, Src: e.Path, Rule: "R-STATUS-09", Irreversible: true})
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops
}
