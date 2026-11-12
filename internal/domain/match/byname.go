package match

import (
	"cmp"
	"path"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

// Pair is an asset, by its key, and the candidate that holds its original,
// with the level that paired them.
type Pair struct {
	Key       domain.AssetKey
	Candidate Candidate
	Level     domain.MatchLevel
}

// ByName pairs assets with candidates one to one where the names of the
// original and of Main tell one shot. Of an asset it reads only fields that
// a snapshot fills: Key, OriginalName, Taken, TZOffset, IsVideo and the
// size of the original resource; of a candidate, only Main. The two can
// pair only when their names agree in NFC and lower case, without the
// extension and without one " (N)" of ASCII digits at the end, and a name
// alone never pairs. A pair is exact when Main has the whole name of the
// original, in NFC and lower case likewise, and its size; same_shot when Main is a video by its extension
// exactly when the asset is one, and its capture time is that of the asset
// within 2 seconds, or a whole number of quarter-hours off up to 14 hours
// when only one of the two has a time zone; same_shot_weak when the kinds
// agree likewise but Main has no capture time and lies under a folder dated
// YYYY/MM of the month of the local capture date. Pairs are taken greedily,
// each asset and candidate once: exact, then same_shot, then
// same_shot_weak, then exact pairs that the capture time or the dated
// folder of Main contradicts; within each, by AssetKey.Compare, then by
// Root and Path of Main. pairs is in the order of Root and Path of Main,
// assetsLeft of AssetKey.Compare and candidatesLeft of Root and Path of
// Main; each is nil when empty, and the inputs are not changed.
func ByName(assets []domain.Asset, candidates []Candidate) (pairs []Pair, assetsLeft []domain.Asset, candidatesLeft []Candidate) {
	assets, candidates = inOrder(assets, candidates)
	stems := map[string][]int{}
	for j, c := range candidates {
		k := nameKey(path.Base(c.Main.Path))
		stems[k] = append(stems[k], j)
	}
	var links []link
	for i, a := range assets {
		for _, j := range stems[nameKey(a.OriginalName)] {
			if level, rank := levelOf(a, candidates[j].Main); level != "" {
				links = append(links, link{asset: i, candidate: j, level: level, rank: rank})
			}
		}
	}
	return assign(assets, candidates, links)
}

// Within a rank, links go in the order given: callers give them by asset and
// then by candidate, both in the order of inOrder, so that ties go by
// AssetKey.Compare and then by Root and Path of Main.
func assign(assets []domain.Asset, candidates []Candidate, links []link) (pairs []Pair, assetsLeft []domain.Asset, candidatesLeft []Candidate) {
	slices.SortStableFunc(links, func(x, y link) int { return cmp.Compare(x.rank, y.rank) })
	paired, taken := make([]bool, len(assets)), map[int]Pair{}
	for _, l := range links {
		if _, ok := taken[l.candidate]; !ok && !paired[l.asset] {
			paired[l.asset] = true
			taken[l.candidate] = Pair{Key: assets[l.asset].Key, Candidate: candidates[l.candidate], Level: l.level}
		}
	}
	for j, c := range candidates {
		if p, ok := taken[j]; ok {
			pairs = append(pairs, p)
		} else {
			candidatesLeft = append(candidatesLeft, c)
		}
	}
	for i, a := range assets {
		if !paired[i] {
			assetsLeft = append(assetsLeft, a)
		}
	}
	return pairs, assetsLeft, candidatesLeft
}

// Ties and results go by key and by place, whatever the order of the input
// (INV-9).
func inOrder(assets []domain.Asset, candidates []Candidate) ([]domain.Asset, []Candidate) {
	return slices.SortedFunc(slices.Values(assets), func(a, b domain.Asset) int { return a.Key.Compare(b.Key) }),
		slices.SortedFunc(slices.Values(candidates), func(a, b Candidate) int { return byPlace(a.Main, b.Main) })
}

type link struct {
	asset, candidate int
	level            domain.MatchLevel
	rank             int
}

// A date edited in Photos leaves the file its old one, so a name and a size
// pair even when the dates disagree; but only after every other pair, so
// that a file of another shot with the same name and size never takes the
// place of the file of the shot.
func levelOf(a domain.Asset, f domain.RemoteEntry) (level domain.MatchLevel, rank int) {
	sameFile := fold(path.Base(f.Path)) == fold(a.OriginalName) &&
		slices.ContainsFunc(a.Resources, func(r domain.Resource) bool {
			return r.Role == domain.ResourceOriginal && r.Size == f.Size
		})
	agrees, dated := sameTime(a, f)
	shot := agrees && isVideo(f.Path) == a.IsVideo
	switch {
	case sameFile && (agrees || !dated):
		return domain.LevelExact, 0
	case shot && !f.Meta.Taken.IsZero():
		return domain.LevelSameShot, 1
	case shot:
		return domain.LevelSameShotWeak, 2
	case sameFile:
		return domain.LevelExact, 3
	}
	return "", 0
}

// Two moments, or two wall clocks, of one shot are equal; a wall clock of an
// unknown zone is off its moment by the offset of the place. A folder is all
// a scan knows of when a file without a capture time was taken.
func sameTime(a domain.Asset, f domain.RemoteEntry) (agrees, dated bool) {
	if t := f.Meta.Taken; !t.IsZero() {
		if (a.TZOffset == nil) == (f.Meta.TZOffset == nil) {
			return dates.Same(t, a.Taken), true
		}
		_, shifted := dates.QuarterHourShift(t, a.Taken)
		return shifted, true
	}
	day, ok := dates.ParseFolder(path.Dir(f.Path))
	if !ok {
		return false, false
	}
	year, month, _ := dates.LocalDay(a.Taken, a.TZOffset)
	return day.Year() == year && day.Month() == month, true
}

// The video of a Live Photo shares the stem and the time of its photo and is
// no original of it.
func isVideo(name string) bool {
	return slices.Contains([]kind{mov, video, source}, kindOf(name))
}
