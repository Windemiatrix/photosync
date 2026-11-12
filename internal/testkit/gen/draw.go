package gen

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
)

var drawPairs = []Action{
	{Side: ICloud, Kind: Create}, {Side: ICloud, Kind: Delete}, {Side: ICloud, Kind: AlbumAdd},
	{Side: ICloud, Kind: AlbumRemove}, {Side: ICloud, Kind: AlbumMove}, {Side: ICloud, Kind: AlbumRename},
	{Side: Remote, Kind: Create}, {Side: Remote, Kind: Move}, {Side: Remote, Kind: Rename},
	{Side: Remote, Kind: Replace}, {Side: Remote, Kind: Delete}, {Side: Remote, Kind: AlbumRename},
}

// Actions returns the generator of the actions that a user takes on w, one
// after another: at most eight, each drawn on the world that the ones before
// it leave and each changing that world. In Photos they take assets outside
// Recently Deleted and albums that are not folders. In the cloud a new file
// goes to the directory of a file, a file moves under its name to the
// directory of another file or is renamed in its own, and no move or rename
// targets a path that holds a file or encloses one.
//
// Attempt n, from 0, names a new asset or file NEW_ and n in four digits with
// .HEIC, and a renamed file the same with its old extension; a new asset has
// the key guid- and uuid- followed by that name, without extension. A new
// asset or file was taken at noon UTC on January 1+n, 2025, and it and the
// content of a Replace have 5000000+n bytes; a renamed album or directory
// gets a space and n. No shot of New or Worlds has such a name, time or size,
// so nothing new pairs with a shot or with another new one by chance. Numbers
// start anew with every call: a world that earlier calls changed may hold
// them.
func Actions(w World) *rapid.Generator[[]Action] {
	return rapid.Custom(func(t *rapid.T) []Action {
		var actions []Action
		cur := w // rapid may run this function again on the same w
		for i := range rapid.IntRange(1, 8).Draw(t, "attempts") {
			a := drawAction(t, cur, i)
			if next := cur.Apply(a); !reflect.DeepEqual(next, cur) {
				actions, cur = append(actions, a), next
			}
		}
		return actions
	})
}

func drawAction(t *rapid.T, w World, i int) Action {
	a := rapid.SampledFrom(drawPairs).Draw(t, "pair")
	name := fmt.Sprintf("NEW_%04d", i)
	size, taken := int64(5_000_000+i), time.Date(2025, time.January, 1+i, 12, 0, 0, 0, time.UTC)
	asset := drawOne(t, slices.DeleteFunc(slices.Clone(w.Snapshot.Assets), func(x domain.Asset) bool { return x.InTrash }))
	album := drawOne(t, slices.DeleteFunc(slices.Clone(w.Snapshot.Albums), func(x domain.Album) bool { return x.Kind == domain.KindFolder }))
	file := drawOne(t, w.Files)
	dir := path.Dir(file.Path)
	switch {
	case a.Side == Remote && file.Path == "": // a new file would have no directory to go to
		return Action{}
	case a.Side == ICloud && a.Kind == Create:
		a.Key = domain.AssetKey{CloudAssetGUID: "guid-" + name, UUID: "uuid-" + name}
		a.Album, a.Path, a.Size, a.Taken = album.UUID, name+".HEIC", size, taken
	case a.Side == ICloud && a.Kind == Delete:
		a.Key = asset.Key
	case a.Side == ICloud && (a.Kind == AlbumAdd || a.Kind == AlbumMove):
		a.Key, a.Album = asset.Key, album.UUID
	case a.Side == ICloud && a.Kind == AlbumRemove:
		a.Key, a.Album = asset.Key, drawOne(t, asset.AlbumUUIDs)
	case a.Side == ICloud && a.Kind == AlbumRename:
		a.Album, a.To = album.UUID, fmt.Sprintf("%s %d", album.Title, i)
	case a.Kind == Create:
		a.Path, a.Size, a.Taken = path.Join(dir, name+".HEIC"), size, taken
	case a.Kind == Move:
		a.Path, a.To = file.Path, path.Join(path.Dir(drawOne(t, w.Files).Path), path.Base(file.Path))
	case a.Kind == Rename:
		a.Path, a.To = file.Path, path.Join(dir, name+path.Ext(file.Path))
	case a.Kind == Replace:
		a.Path, a.Size = file.Path, size
	case a.Kind == Delete:
		a.Path = file.Path
	case a.Kind == AlbumRename:
		a.Path, a.To = dir, fmt.Sprintf("%s %d", dir, i)
	}
	if a.Side == Remote && a.To != "" && slices.ContainsFunc(w.Files, func(f domain.RemoteEntry) bool {
		return f.Path == a.To || strings.HasPrefix(f.Path, a.To+"/")
	}) {
		return Action{}
	}
	return a
}

// SampledFrom panics on an empty slice; the zero value stands for no target.
func drawOne[T any](t *rapid.T, xs []T) T {
	var x T
	if len(xs) > 0 {
		x = rapid.SampledFrom(xs).Draw(t, "target")
	}
	return x
}
