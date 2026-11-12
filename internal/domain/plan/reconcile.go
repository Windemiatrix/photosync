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
// pairs that share an id which no other item outside pairs has on either
// side.
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
		if _, done := pairs[i]; !done && olds[id] == 1 && nows[id] == 1 {
			pairs[i] = at[id]
		}
	}
}
