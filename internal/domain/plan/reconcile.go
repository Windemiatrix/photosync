package plan

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// ErrBadSnapshot is the error of a snapshot that breaks its contract, such
// as two albums with one cloud key or a reference to an album that the
// snapshot does not hold.
var ErrBadSnapshot = errors.New("plan: the snapshot breaks its contract")

// Reconcile returns snap, the snapshot of the library as a run reads it,
// reconciled with state, which the last applied run left: the delta that the
// run writes to the state before its operations. Its albums are those of
// ReconcileAlbums. An asset of snap is the asset of state, or the earlier
// identity that an entry of state.History names, with its cloud GUID, else
// with its UUID, when no other asset or entry left on either side has that
// nonempty GUID or UUID. An asset of state keeps its ID; an earlier identity
// is not in the delta, and its entry takes its key, with Deleted turning true
// once it is in Recently Deleted or gone from snap. Any other asset is new,
// with an ID above every asset ID of state in the order of AssetKey.Compare.
// The assets of the delta are as in snap, with Memberships for AlbumUUIDs,
// each with its FirstSeenRun in state, else runID; with the TargetLibrary of
// state while it differs from the Library there, else Library; and with the
// hashes of the resource of state with the same role and size. Assets go in
// the order of ID, memberships of AlbumID, History of AssetID and then key.
// An asset outside Recently Deleted whose entry is Deleted in state, which
// the user brought back, has a notice "plan.replaced_restored" with its name
// and UUID, in the order of keys. The error, with a zero delta and no
// notices, is that of ReconcileAlbums, or wraps ErrBadSnapshot when two
// assets of snap have one cloud GUID or an asset is in an album that snap
// lacks.
func Reconcile(state domain.State, snap domain.LibrarySnapshot, runID string) (domain.SnapshotDelta, []domain.Notice, error) {
	albums, err := ReconcileAlbums(state.Albums, snap.Albums)
	if err != nil {
		return domain.SnapshotDelta{}, nil, err
	}
	albumIDs := map[string]domain.AlbumID{}
	for _, a := range albums {
		albumIDs[a.UUID] = a.ID
	}
	assets := slices.Clone(snap.Assets)
	slices.SortFunc(assets, func(x, y domain.Asset) int { return x.Key.Compare(y.Key) })
	guids, uuids := make([]string, len(assets)), make([]string, len(assets))
	for i, a := range assets {
		if guid := a.Key.CloudAssetGUID; guid != "" && i > 0 && guid == assets[i-1].Key.CloudAssetGUID {
			return domain.SnapshotDelta{}, nil, fmt.Errorf("%w: two assets have the cloud GUID %q", ErrBadSnapshot, guid)
		}
		guids[i], uuids[i] = a.Key.CloudAssetGUID, a.Key.UUID
	}
	var last domain.AssetID
	var oldGUIDs, oldUUIDs []string
	for _, a := range state.Assets {
		oldGUIDs, oldUUIDs, last = append(oldGUIDs, a.Key.CloudAssetGUID), append(oldUUIDs, a.Key.UUID), max(last, a.ID)
	}
	for _, h := range state.History {
		oldGUIDs, oldUUIDs = append(oldGUIDs, h.Key.CloudAssetGUID), append(oldUUIDs, h.Key.UUID)
	}
	// iCloud keeps the GUID of an asset when a library is made anew, which
	// gives every asset a new UUID; an asset not yet in iCloud has no GUID.
	pairs := map[int]int{}
	matchIDs(pairs, oldGUIDs, guids)
	matchIDs(pairs, oldUUIDs, uuids)
	delta := domain.SnapshotDelta{Albums: albums, History: slices.Clone(state.History)}
	for k := range delta.History {
		delta.History[k].Deleted = true
	}
	var notices []domain.Notice
	for i := range assets {
		a := &assets[i]
		j, found := pairs[i]
		var old domain.Asset
		if found && j < len(state.Assets) {
			old = state.Assets[j]
		}
		if a.Memberships, err = reconcileMemberships(*a, old, albumIDs, runID); err != nil {
			return domain.SnapshotDelta{}, nil, err
		}
		if k := j - len(state.Assets); found && k >= 0 {
			deleted := state.History[k].Deleted
			if deleted && !a.InTrash {
				notices = append(notices, domain.Notice{Code: "plan.replaced_restored", Params: []string{a.OriginalName, a.Key.UUID}})
			}
			delta.History[k].Key, delta.History[k].Deleted = a.Key, deleted || a.InTrash
			continue
		}
		a.ID = old.ID
		if !found {
			last++
			a.ID = last
		}
		a.TargetLibrary = a.Library
		if old.TargetLibrary != old.Library {
			a.TargetLibrary = old.TargetLibrary
		}
		a.Resources = slices.Clone(a.Resources)
		for r, res := range a.Resources {
			if o := slices.IndexFunc(old.Resources, func(x domain.Resource) bool { return x.Role == res.Role && x.Size == res.Size }); o >= 0 {
				a.Resources[r].SHA256, a.Resources[r].MD5 = old.Resources[o].SHA256, old.Resources[o].MD5
			}
		}
		delta.Assets = append(delta.Assets, *a)
	}
	slices.SortFunc(delta.Assets, func(x, y domain.Asset) int { return cmp.Compare(x.ID, y.ID) })
	slices.SortFunc(delta.History, func(x, y domain.AssetHistory) int {
		return cmp.Or(cmp.Compare(x.AssetID, y.AssetID), x.Key.Compare(y.Key))
	})
	return delta, notices, nil
}

// ReconcileAlbums returns the albums and folders of snap with their IDs in
// the state: state is the albums that the state holds, read only for ID,
// UUID and Key. An album keeps the ID of the album of state with its UUID,
// else of the one with its key, when no other album left on either side has
// that key; any other album is new and gets an ID above every ID of state,
// in the order of UUID. ParentID is the ID of the folder that ParentUUID
// names; Kind, Key and KeySource come from layout.Classify and layout.Key,
// so albums may share a path key; the rest is as in snap. The result is in
// the order of ID. The error wraps ErrBadSnapshot when two albums of snap
// have one cloud key or a ParentUUID names no album of snap.
func ReconcileAlbums(state, snap []domain.Album) ([]domain.Album, error) {
	albums := slices.Clone(snap)
	uuids, keys, cloud := make([]string, len(albums)), make([]string, len(albums)), map[string]bool{}
	for i := range albums {
		a := &albums[i]
		a.Kind = layout.Classify(*a)
		a.Key, a.KeySource = layout.Key(*a)
		if a.KeySource != domain.KeyPath {
			if cloud[a.Key] {
				return nil, fmt.Errorf("%w: two albums have the cloud key %q", ErrBadSnapshot, a.Key)
			}
			cloud[a.Key] = true
		}
		uuids[i], keys[i] = a.UUID, a.Key
	}
	var last domain.AlbumID
	oldUUIDs, oldKeys := make([]string, len(state)), make([]string, len(state))
	for j, a := range state {
		oldUUIDs[j], oldKeys[j], last = a.UUID, a.Key, max(last, a.ID)
	}
	// Photos keeps the UUID of an album for its life, while a path key
	// changes with each rename or move: only a library made anew needs keys.
	pairs := map[int]int{}
	matchIDs(pairs, oldUUIDs, uuids)
	matchIDs(pairs, oldKeys, keys)
	var fresh []int
	for i := range albums {
		if j, ok := pairs[i]; ok {
			albums[i].ID = state[j].ID
		} else {
			fresh = append(fresh, i)
		}
	}
	slices.SortFunc(fresh, func(x, y int) int { return cmp.Compare(albums[x].UUID, albums[y].UUID) })
	for _, i := range fresh {
		last++
		albums[i].ID = last
	}
	ids := map[string]domain.AlbumID{}
	for _, a := range albums {
		ids[a.UUID] = a.ID
	}
	for i := range albums {
		a := &albums[i]
		a.ParentID = ids[a.ParentUUID]
		if a.ParentUUID != "" && a.ParentID == 0 {
			return nil, fmt.Errorf("%w: album %q is in folder %q, which the snapshot lacks", ErrBadSnapshot, a.UUID, a.ParentUUID)
		}
	}
	slices.SortFunc(albums, func(x, y domain.Album) int { return cmp.Compare(x.ID, y.ID) })
	return albums, nil
}

// matchIDs adds to pairs, as pairs[i] = j, every now[i] and old[j] outside
// pairs that share a nonempty id which no other item outside pairs has on
// either side.
func matchIDs(pairs map[int]int, old, now []string) {
	used := map[int]bool{}
	for _, j := range pairs {
		used[j] = true
	}
	at, olds, nows := map[string]int{}, map[string]int{}, map[string]int{}
	for j, id := range old {
		if !used[j] {
			at[id], olds[id] = j, olds[id]+1
		}
	}
	for i, id := range now {
		if _, done := pairs[i]; !done {
			nows[id]++
		}
	}
	for i, id := range now {
		if _, done := pairs[i]; !done && id != "" && olds[id] == 1 && nows[id] == 1 {
			pairs[i] = at[id]
		}
	}
}

// reconcileMemberships returns the albums of a, an asset of a snapshot, as
// memberships by AlbumID, with the FirstSeenRun of old, the asset of the
// state, else runID; ids maps UUIDs to IDs, and a UUID it lacks is an error.
func reconcileMemberships(a, old domain.Asset, ids map[string]domain.AlbumID, runID string) ([]domain.Membership, error) {
	var ms []domain.Membership
	for _, uuid := range a.AlbumUUIDs {
		id, ok := ids[uuid]
		if !ok {
			return nil, fmt.Errorf("%w: asset %q is in album %q, which the snapshot lacks", ErrBadSnapshot, a.Key.UUID, uuid)
		}
		run := runID
		if k := slices.IndexFunc(old.Memberships, func(m domain.Membership) bool { return m.AlbumID == id }); k >= 0 {
			run = old.Memberships[k].FirstSeenRun
		}
		ms = append(ms, domain.Membership{AlbumID: id, FirstSeenRun: run})
	}
	slices.SortFunc(ms, func(x, y domain.Membership) int { return cmp.Compare(x.AlbumID, y.AlbumID) })
	return ms, nil
}
