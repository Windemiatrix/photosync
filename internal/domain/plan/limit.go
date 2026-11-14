package plan

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Limit returns p cut down to its first n assets, for a run that changes no
// more than n of them; what stays keeps its order. An operation belongs to
// the asset that its Asset names, and an import with a zero Asset to a new
// asset of its own, and the assets come in the order of their first
// operations in p.Ops. An asset past the first n leaves with all its
// operations, in every phase and remote, and with its Links and Conflicts,
// so that the state keeps it as it is. The operations of no asset, such as
// an OpAlbumCreate or an OpExpire, stay, as do Albums, Pairs, Notices and
// the Links and Conflicts of an asset without operations. An n below 1 sets
// no limit. Limit does not change p.
func Limit(p domain.Plan, n int) domain.Plan {
	if n < 1 {
		return p
	}
	kept, cut := map[domain.AssetID]bool{}, map[domain.AssetID]bool{}
	p.Ops = slices.DeleteFunc(slices.Clone(p.Ops), func(o domain.Op) bool {
		switch {
		case o.Asset == 0 && o.Kind != domain.OpImport, o.Asset != 0 && kept[o.Asset]:
			return false
		case n > 0:
			n, kept[o.Asset] = n-1, true
			return false
		}
		cut[o.Asset] = true
		return true
	})
	p.Links = slices.DeleteFunc(slices.Clone(p.Links), func(l domain.Link) bool { return cut[l.AssetID] })
	p.Conflicts = slices.DeleteFunc(slices.Clone(p.Conflicts), func(c domain.Conflict) bool { return cut[c.AssetID] })
	return p
}
