package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// DecideDerived applies the derived copy policy to linked, the decisions on
// the new links of a remote in mode m as JoinFresh gives them; assets are
// those of the snapshot reconciled with the state, and fresh is what ReadScan
// gives for the remote. A link of the original role at the level same_shot,
// same_shot_weak or probable, whose asset is in assets and whose entry is in
// fresh at its root and path, is a file that matching found for its asset by
// the shot, and match.CompareCopies tells which of the two is better. When
// the original is, every link of that asset in linked, its Live video, RAW
// and copies too, becomes ActionReplaceFile by R-OBJECT-10 under
// replace_with_original unless m is download-only, and takes the role derived
// under keep_both in every mode; keep_remote and any other policy leave them.
// When the file is better, notices hold "plan.cloud_better" with its root and
// path under every policy. decisions are linked in the order of link ID,
// notices in the order of their links, and the inputs are not changed.
func DecideDerived(linked []domain.LinkDecision, assets []domain.Asset, fresh []domain.RemoteEntry, m domain.Mode, policy domain.DerivedCopyPolicy) (decisions []domain.LinkDecision, notices []domain.Notice) {
	byID := map[domain.AssetID]domain.Asset{}
	for _, a := range assets {
		byID[a.ID] = a
	}
	byPlace := map[domain.RemoteEntry]domain.RemoteEntry{}
	for _, e := range fresh {
		byPlace[domain.RemoteEntry{Root: e.Root, Path: e.Path}] = e
	}
	decisions = slices.Clone(linked)
	slices.SortFunc(decisions, func(x, y domain.LinkDecision) int { return cmp.Compare(x.Link.ID, y.Link.ID) })
	shots := []domain.MatchLevel{domain.LevelSameShot, domain.LevelSameShotWeak, domain.LevelProbable}
	lesser := map[domain.AssetID]bool{}
	for _, d := range decisions {
		a, found := byID[d.Link.AssetID]
		e, scanned := byPlace[domain.RemoteEntry{Root: d.Link.Root, Path: d.Link.Path}]
		if !found || !scanned || d.Link.Role != domain.RoleOriginal || !slices.Contains(shots, d.Link.Level) {
			continue
		}
		switch v := match.CompareCopies(a, e); {
		case v < 0:
			notices = append(notices, domain.Notice{Code: "plan.cloud_better", Params: []string{string(e.Root), e.Path}})
		case v > 0:
			lesser[a.ID] = true
		}
	}
	// A Live video or RAW beside no file of the original role would go to
	// quarantine, and a copy of a lesser file is just as lesser.
	for i, d := range decisions {
		switch {
		case !lesser[d.Link.AssetID]:
		case policy == domain.DerivedKeepBoth:
			decisions[i].Link.Role = domain.RoleDerived
		case policy == domain.DerivedReplaceWithOriginal && ModeAllows(m, domain.ActionReplaceFile):
			decisions[i].Action, decisions[i].Rule = domain.ActionReplaceFile, "R-OBJECT-10"
		}
	}
	return decisions, notices
}
