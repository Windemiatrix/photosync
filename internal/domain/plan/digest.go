package plan

import (
	"cmp"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// Digest returns "sha256:" and the SHA-256, in lowercase hex, of the JSON of
// the operations of p, by which a run tells whether p is the plan that the
// user saw; delta is the delta that Build gives with p. There an operation
// names its asset by the UUID that delta gives it, else by its ID, which the
// state keeps, and its album and ParentID each by the UUID that delta gives
// it, else by the key that layout.Key gives an album of p.Albums, else by
// none; its links leave out their IDs, assets, albums and MTime. So the digest
// stays the same when only the IDs that a run gives new assets, albums and
// links, a cloud GUID of an asset, the times of files, the order of the
// operations or what p keeps beside them change, and it changes with anything
// else that an operation does. The error is that of encoding/json, and with it
// the digest means nothing. The inputs are not changed.
func Digest(p domain.Plan, delta domain.SnapshotDelta) (string, error) {
	assets, albums := map[domain.AssetID]string{}, map[domain.AlbumID]string{}
	for _, a := range delta.Assets {
		assets[a.ID] = a.Key.UUID
	}
	for _, a := range slices.Concat(delta.Albums, p.Albums) {
		key, _ := layout.Key(a)
		albums[a.ID] = cmp.Or(a.UUID, key)
	}
	type entry struct {
		Op                   domain.Op
		Asset, Album, Parent string
	}
	var entries []entry
	for _, o := range p.Ops {
		e := entry{Op: o, Asset: cmp.Or(assets[o.Asset], fmt.Sprint(o.Asset)), Album: albums[o.AlbumID], Parent: albums[o.ParentID]}
		e.Op.Asset, e.Op.AlbumID, e.Op.ParentID, e.Op.Links = 0, 0, 0, slices.Clone(o.Links)
		for i := range e.Op.Links {
			l := &e.Op.Links[i]
			l.ID, l.AssetID, l.AlbumID, l.MTime = 0, 0, 0, time.Time{}
		}
		entries = append(entries, e)
	}
	slices.SortFunc(entries, func(x, y entry) int {
		return cmp.Or(x.Op.Compare(y.Op), cmp.Compare(x.Asset, y.Asset), cmp.Compare(x.Album, y.Album))
	})
	h := sha256.New()
	err := json.NewEncoder(h).Encode(entries)
	return fmt.Sprintf("sha256:%x", h.Sum(nil)), err
}
