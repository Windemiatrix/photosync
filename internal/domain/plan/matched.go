package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
)

// MatchFresh links the new files of remote r to the assets that they hold.
// fresh and files are what ReadScan gives for r, links are the links of the
// state in every remote, role and status, and assets are those of the
// snapshot reconciled with the state. The assets without an active link of
// the original or the source role in r meet in match.Match the candidates
// that match.Group makes of fresh. A candidate that match.Match leaves is a
// copy (R-LAYOUT-08) when its Main has, by the hash of kind
// r.Capabilities.ContentHash, the content of the Main of a pair or of the
// entry that files gives an active link of the original or the source role
// in r of an asset of assets; it goes with the asset of that file, the
// lowest AssetID when several hold the content. A pair or a copy links
// nothing and leaves nothing when IsNew does not hold for its asset under p
// or the asset has a link in r that is not active. Otherwise Main gets a
// link with the role of the candidate and the level of the pair, or exact
// for a copy, and so does each Live or RAW whose role the asset has a
// resource of, with the role live_video or raw_pair and the level
// pair_component; one whose role the asset lacks is left as a candidate of
// its own with the role original. A link lies at the root and path of its
// entry, with its size, hashes, ETag and MTime, and is active; its AlbumID
// is zero, since the album pairs of the run come after matching, and the IDs
// run from last+1 in the order of decisions: pairs, then copies, each in the
// order of Root and Path of Main, with Main before Live before RAW. Each
// decision links without a transfer: its Link is the new link, ICloud and
// Remote are new, Action is ActionLink and Rule is R-LINK-03, or R-LAYOUT-08
// for the links of a copy. left are the new files of r, and notices are
// "plan.unknown_file" with the root and path of each file of fresh that is
// neither media nor sidecar, both in the order of Root and Path. The inputs
// are not changed.
func MatchFresh(assets []domain.Asset, links []domain.Link, files map[domain.LinkID]domain.RemoteEntry, r domain.RemoteSpec, fresh []domain.RemoteEntry, last domain.LinkID, p domain.Policies) (decisions []domain.LinkDecision, left []match.Candidate, notices []domain.Notice) {
	cands, unknown := match.Group(fresh)
	for _, e := range unknown {
		notices = append(notices, domain.Notice{Code: "plan.unknown_file", Params: []string{string(e.Root), e.Path}})
	}
	byID, byKey := map[domain.AssetID]domain.Asset{}, map[domain.AssetKey]domain.Asset{}
	for _, a := range assets {
		byID[a.ID], byKey[a.Key] = a, a
	}
	content := func(e domain.RemoteEntry) string { return moveKey(e.MD5, e.SHA256, r.Capabilities.ContentHash) }
	held := map[string]domain.Asset{}
	hold := func(e domain.RemoteEntry, a domain.Asset) {
		k := content(e)
		if h, ok := held[k]; k != "" && (!ok || a.ID < h.ID) {
			held[k] = a
		}
	}
	linked, out := map[domain.AssetID]bool{}, map[domain.AssetID]bool{}
	for _, l := range links {
		if l.Remote != r.Name {
			continue
		}
		linked[l.AssetID] = linked[l.AssetID] || objectFile(l)
		out[l.AssetID] = out[l.AssetID] || l.Status != domain.StatusActive
		if a, found := byID[l.AssetID]; found && objectFile(l) {
			hold(files[l.ID], a)
		}
	}
	add := func(a domain.Asset, e domain.RemoteEntry, role domain.LinkRole, level domain.MatchLevel, rule string) {
		last++
		l := domain.Link{
			ID: last, AssetID: a.ID, Remote: r.Name, Root: e.Root, Path: e.Path, Role: role,
			Size: e.Size, SHA256: e.SHA256, MD5: e.MD5, ETag: e.ETag, MTime: e.MTime, Level: level, Status: domain.StatusActive,
		}
		decisions = append(decisions, domain.LinkDecision{Link: l, ICloud: domain.ChangeNew, Remote: domain.ChangeNew, Action: domain.ActionLink, Rule: rule})
	}
	take := func(a domain.Asset, c match.Candidate, level domain.MatchLevel, rule string) {
		if !IsNew(a, p) || out[a.ID] {
			return
		}
		add(a, c.Main, c.Role, level, rule)
		for _, part := range []struct {
			file *domain.RemoteEntry
			has  domain.ResourceRole
			role domain.LinkRole
		}{{c.Live, domain.ResourceLiveVideo, domain.RoleLiveVideo}, {c.RAW, domain.ResourceRAWPair, domain.RoleRAWPair}} {
			switch {
			case part.file == nil:
			case slices.ContainsFunc(a.Resources, func(res domain.Resource) bool { return res.Role == part.has }):
				add(a, *part.file, part.role, domain.LevelPairComponent, rule)
			default:
				left = append(left, match.Candidate{Main: *part.file, Role: domain.RoleOriginal})
			}
		}
	}
	pairs, _, rest := match.Match(slices.DeleteFunc(slices.Clone(assets), func(a domain.Asset) bool { return linked[a.ID] }), cands)
	for _, x := range pairs {
		hold(x.Candidate.Main, byKey[x.Key])
		take(byKey[x.Key], x.Candidate, x.Level, "R-LINK-03")
	}
	for _, c := range rest {
		if a, ok := held[content(c.Main)]; ok {
			take(a, c, domain.LevelExact, "R-LAYOUT-08")
		} else {
			left = append(left, c)
		}
	}
	slices.SortFunc(left, func(x, y match.Candidate) int {
		return cmp.Or(cmp.Compare(x.Main.Root, y.Main.Root), cmp.Compare(x.Main.Path, y.Main.Path))
	})
	return decisions, left, notices
}
