package match

import (
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// ByTime pairs assets with candidates one to one where the capture times of
// the original and of Main tell one shot although the names do not, as when
// Photos renamed the file at import. Of an asset it reads only fields that a
// snapshot fills: Key, OriginalName, Taken, TZOffset, SubSec, Width, Height,
// IsVideo and the size of the original resource; of a candidate, only Main.
// Times compare as wall clocks: Taken in whole seconds, shifted by TZOffset
// when there is one; two offsets that are both known must be equal. When
// SubSec on both sides holds one to nine digits, the fraction after the
// point ("075" is 75 ms, "75" is 750 ms), the times with their fractions
// must be equal; otherwise they may be a second apart. Main must also be a
// video by its extension exactly when the asset is one. A pair is
// same_time_size when Main has the size of the original, and probable when
// it has another size but the width and height of the asset, either way
// round. Pairs are taken greedily, each asset and candidate once:
// same_time_size before probable, then times equal with their fractions,
// then times equal to the second, then a second apart, then the extension
// of OriginalName, in any case, before another; within each, by
// AssetKey.Compare, then by Root and Path of Main. pairs is in the order of
// Root and Path of Main, assetsLeft of AssetKey.Compare and candidatesLeft
// of Root and Path of Main; each is nil when empty, and the inputs are not
// changed.
func ByTime(assets []domain.Asset, candidates []Candidate) (pairs []Pair, assetsLeft []domain.Asset, candidatesLeft []Candidate) {
	assets, candidates = inOrder(assets, candidates)
	seconds := map[int64][]int{}
	for j, c := range candidates {
		s := wall(c.Main.Meta.Taken, c.Main.Meta.TZOffset).Unix()
		seconds[s] = append(seconds[s], j)
	}
	var links []link
	for i, a := range assets {
		s := wall(a.Taken, a.TZOffset).Unix()
		near := slices.Concat(seconds[s-1], seconds[s], seconds[s+1])
		slices.Sort(near)
		for _, j := range near {
			if level, rank := timeLevel(a, candidates[j].Main); level != "" {
				links = append(links, link{asset: i, candidate: j, level: level, rank: rank})
			}
		}
	}
	return assign(assets, candidates, links)
}

// Of two files of one moment and frame only the size tells a copy of the
// original from a shot of another camera, so the size goes first; a copy in
// another format keeps the frame but not the size.
func timeLevel(a domain.Asset, f domain.RemoteEntry) (level domain.MatchLevel, rank int) {
	near, ok := moment(a, f.Meta)
	if !ok || isVideo(f.Path) != a.IsVideo {
		return "", 0
	}
	rank = 2 * near
	if !strings.EqualFold(path.Ext(f.Path), path.Ext(a.OriginalName)) {
		rank++
	}
	w, h := f.Meta.Width, f.Meta.Height
	switch {
	case slices.ContainsFunc(a.Resources, func(r domain.Resource) bool {
		return r.Role == domain.ResourceOriginal && r.Size == f.Size
	}):
		return domain.LevelSameTimeSize, rank
	case w != 0 && (w == a.Width && h == a.Height || w == a.Height && h == a.Width):
		return domain.LevelProbable, 6 + rank
	}
	return "", 0
}

// ByTime brings only files within a second of the wall clock of the asset.
// A camera may write no time zone, and a copy may drop the fraction of a
// second and round the second, so equal fractions say more than an equal
// second; offsets that differ put one wall clock at two moments.
func moment(a domain.Asset, m domain.MediaMeta) (near int, ok bool) {
	if a.TZOffset != nil && m.TZOffset != nil && *a.TZOffset != *m.TZOffset {
		return 0, false
	}
	at, mt := wall(a.Taken, a.TZOffset), wall(m.Taken, m.TZOffset)
	af, aok := fraction(a.SubSec)
	mf, mok := fraction(m.SubSec)
	if aok && mok {
		return 0, at.Add(af).Equal(mt.Add(mf))
	}
	if at.Equal(mt) {
		return 1, true
	}
	return 2, true
}

// A file without a time zone tells only its wall clock, and SubSec holds the
// fraction of the second, whatever Taken keeps of it.
func wall(t time.Time, offset *int) time.Time {
	t = t.Truncate(time.Second)
	if offset != nil {
		t = t.Add(time.Duration(*offset) * time.Second)
	}
	return t
}

// The digits are those after the point, and nine of them are nanoseconds.
func fraction(s string) (time.Duration, bool) {
	if s == "" || len(s) > 9 || strings.Trim(s, "0123456789") != "" {
		return 0, false
	}
	n, _ := strconv.Atoi((s + "00000000")[:9])
	return time.Duration(n), true
}
