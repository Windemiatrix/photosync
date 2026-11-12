package match

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Match pairs assets with candidates at a first sync: ByName, then ByTime on
// what ByName leaves, then the files beside Main. Of an asset it reads what
// ByTime reads and the roles of its resources; candidates are as Group gives
// them. A pair keeps the Live or RAW of its candidate only when the asset
// has a resource of that role, live_video or raw_pair; otherwise that file
// is left as a candidate of its own, with the role original, and pairs with
// no asset. A candidate left whose Main is a MOV or a RAW, as Group tells
// them, becomes the Live or RAW of the first pair, in the order of Root and
// Path of Main, whose Main has the same Root, directory and export.StemKey,
// whose asset has a resource of that role and whose candidate has no such
// file; the pair keeps the level of its Main. pairs and candidatesLeft are
// in the order of Root and Path of Main, assetsLeft of AssetKey.Compare;
// each is nil when empty, and the inputs are not changed.
func Match(assets []domain.Asset, candidates []Candidate) (pairs []Pair, assetsLeft []domain.Asset, candidatesLeft []Candidate) {
	pairs, assetsLeft, candidatesLeft = ByName(assets, candidates)
	byTime, assetsLeft, candidatesLeft := ByTime(assetsLeft, candidatesLeft)
	pairs = slices.Concat(pairs, byTime)
	slices.SortFunc(pairs, func(a, b Pair) int { return byPlace(a.Candidate.Main, b.Candidate.Main) })
	roles := map[domain.AssetKey][]domain.ResourceRole{}
	for _, a := range assets {
		for _, r := range a.Resources {
			roles[a.Key] = append(roles[a.Key], r.Role)
		}
	}
	beside := map[groupKey][]int{}
	for i := range pairs {
		for _, k := range []kind{mov, raw} {
			// Photos has no file of a role that the asset lacks to restore it
			// from (R-COMPONENT-01), and the file may be an asset of its own.
			if file, role := component(&pairs[i].Candidate, k); *file != nil && !slices.Contains(roles[pairs[i].Key], role) {
				candidatesLeft = append(candidatesLeft, Candidate{Main: **file, Role: domain.RoleOriginal})
				*file = nil
			}
		}
		key := keyOf(pairs[i].Candidate.Main)
		beside[key] = append(beside[key], i)
	}
	slices.SortFunc(candidatesLeft, func(a, b Candidate) int { return byPlace(a.Main, b.Main) })
	var alone []Candidate
	for _, c := range candidatesLeft {
		if !join(pairs, beside[keyOf(c.Main)], roles, c.Main) {
			alone = append(alone, c)
		}
	}
	return pairs, assetsLeft, alone
}

// Group puts a MOV of the stem of Main in Live and a RAW in RAW, and Photos
// keeps them as resources of the roles returned.
func component(c *Candidate, k kind) (**domain.RemoteEntry, domain.ResourceRole) {
	if k == mov {
		return &c.Live, domain.ResourceLiveVideo
	}
	return &c.RAW, domain.ResourceRAWPair
}

// at lists the pairs whose Main lies beside e, in the order of place.
func join(pairs []Pair, at []int, roles map[domain.AssetKey][]domain.ResourceRole, e domain.RemoteEntry) bool {
	k := kindOf(e.Path)
	if k != mov && k != raw {
		return false
	}
	for _, i := range at {
		if file, role := component(&pairs[i].Candidate, k); *file == nil && slices.Contains(roles[pairs[i].Key], role) {
			*file = &e
			return true
		}
	}
	return false
}
