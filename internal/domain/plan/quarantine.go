package plan

import (
	"fmt"
	"path"
	"strings"
	"time"
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
