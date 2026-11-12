package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DecideConflict returns c as the run leaves it and whether the decisions on
// the links of its asset in its remote take part in the plan. c is the
// conflict that the state keeps open or skipped for the asset there, or one
// with only AssetID and Remote when the state keeps none; decisions are the
// Links that DecideObject gives for the asset there, nil when there are none.
// The Resolution of c, when set, stands in for policy. While a decision is
// ActionConflict, skip leaves c skipped and ask or any other policy leaves it
// open, and the decisions stay out of the plan; remote_wins and icloud_wins
// settle c, and the run applies that policy to the decisions. Once no
// decision is ActionConflict, c closes by itself (R-STATUS-07). A settled or
// closed c is resolved unless its Status was empty; AssetID, Remote and
// Resolution stay as they are.
func DecideConflict(c domain.Conflict, decisions []domain.LinkDecision, policy domain.ConflictPolicy) (domain.Conflict, bool) {
	policy = cmp.Or(c.Resolution, policy)
	switch {
	case !slices.ContainsFunc(decisions, func(d domain.LinkDecision) bool { return d.Action == domain.ActionConflict }),
		policy == domain.ConflictRemoteWins, policy == domain.ConflictICloudWins:
		if c.Status != "" {
			c.Status = domain.ConflictStatusResolved
		}
		return c, true
	case policy == domain.ConflictSkip:
		c.Status = domain.ConflictStatusSkipped
	default:
		c.Status = domain.ConflictStatusOpen
	}
	return c, false
}
