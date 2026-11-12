package plan_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Written from the conflict policies and the life of a conflict, not from
// the code: ask and skip leave a conflict open or skipped and its asset out
// of the plan of the remote, run after run; a choice of the user or a policy
// with a winner settles it; and it closes by itself once no link of the
// asset there conflicts. Every row runs under each policy.
func TestDecideConflict(t *testing.T) {
	t.Parallel()
	const (
		unchanged, moved        = domain.ChangeUnchanged, domain.ChangeMoved
		personal, shared        = domain.LibraryPersonal, domain.LibraryShared
		open, skipped, resolved = domain.ConflictStatusOpen, domain.ConflictStatusSkipped, domain.ConflictStatusResolved
	)
	decided := func(id domain.LinkID, root domain.LibraryKind, icloud, remote domain.ChangeKind, same bool) domain.LinkDecision {
		d, _ := plan.DecideLink(icloud, remote, same)
		d.Link = domain.Link{ID: id, AssetID: 7, Remote: "disk", Root: root, Path: "2024/05-01 - Holidays/IMG_0001.HEIC"}
		return d
	}
	stored := func(status domain.ConflictStatus, resolution domain.ConflictPolicy) domain.Conflict {
		return domain.Conflict{AssetID: 7, Remote: "disk", Status: status, Resolution: resolution}
	}
	type outcome struct {
		status  domain.ConflictStatus
		planned bool
	}
	none, closed := outcome{"", true}, outcome{resolved, true}
	asked, aside := outcome{open, false}, outcome{skipped, false}
	all := func(o outcome) [5]outcome { return [5]outcome{o, o, o, o, o} }
	policies := [5]domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip, "keep"}
	bothMoved := decided(1, personal, moved, moved, false)
	for _, c := range []struct {
		name      string
		stored    domain.Conflict
		decisions []domain.LinkDecision
		want      [5]outcome // remote_wins, icloud_wins, ask, skip, unknown
	}{
		{"a new conflict", stored("", ""), []domain.LinkDecision{bothMoved}, [5]outcome{none, none, asked, aside, asked}},
		{"a new conflict on a link in the other root", stored("", ""),
			[]domain.LinkDecision{decided(1, personal, unchanged, unchanged, false), decided(2, shared, moved, moved, false), decided(3, personal, unchanged, unchanged, false)},
			[5]outcome{none, none, asked, aside, asked}},
		{"no conflict", stored("", ""), []domain.LinkDecision{decided(1, personal, unchanged, moved, false)}, all(none)},
		{"an open conflict, both sides still moved", stored(open, ""), []domain.LinkDecision{bothMoved},
			[5]outcome{closed, closed, asked, aside, asked}}, // covers: R-STATUS-07
		{"a skipped conflict, both sides still moved", stored(skipped, ""), []domain.LinkDecision{bothMoved},
			[5]outcome{closed, closed, asked, aside, asked}},
		{"the user chose the cloud", stored(open, domain.ConflictRemoteWins), []domain.LinkDecision{bothMoved}, all(closed)},
		{"the user chose iCloud", stored(open, domain.ConflictICloudWins), []domain.LinkDecision{bothMoved}, all(closed)},
		{"a choice unknown here", stored(open, "keep"), []domain.LinkDecision{bothMoved}, all(asked)},
		{"the user chose to skip", stored(open, domain.ConflictSkip), []domain.LinkDecision{bothMoved}, all(aside)},
		{"the user chose to ask", stored(skipped, domain.ConflictAsk), []domain.LinkDecision{bothMoved}, all(asked)},
		{"the asset returned to its base in iCloud", stored(open, ""),
			[]domain.LinkDecision{decided(1, personal, unchanged, moved, false)}, all(closed)}, // covers: R-STATUS-07
		{"the file returned to its base in the cloud", stored(skipped, ""),
			[]domain.LinkDecision{decided(1, personal, moved, unchanged, false)}, all(closed)},
		{"both sides moved to one place", stored(open, ""), []domain.LinkDecision{decided(1, personal, moved, moved, true)}, all(closed)},
		{"no decision on the asset there", stored(open, domain.ConflictRemoteWins), nil, all(closed)},
		{"an open conflict with no decisions", stored(open, ""), nil, all(closed)},
	} {
		for k, policy := range policies {
			got, planned := plan.DecideConflict(c.stored, c.decisions, policy)
			w := c.want[k]
			if got.Status != w.status || planned != w.planned ||
				got.AssetID != c.stored.AssetID || got.Remote != c.stored.Remote || got.Resolution != c.stored.Resolution {
				t.Errorf("%s under %q: DecideConflict = %+v, %v; want status %q, %v, the rest as stored", c.name, policy, got, planned, w.status, w.planned)
			}
		}
	}
}
