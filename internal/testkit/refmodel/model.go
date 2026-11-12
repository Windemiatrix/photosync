package refmodel

import (
	"fmt"
	"strings"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

// Album is an album of Photos by its place: the titles of the folders that
// enclose it, outermost first and joined by "/", and its own title.
type Album struct {
	Folder, Title string
}

// Object is an asset of Photos: its album, the name and size of its
// original, and whether it is in Recently Deleted.
type Object struct {
	Album   Album
	Name    string
	Size    int64
	Trashed bool
}

// File is a file of the remote root: the asset that it is linked to, zero
// for none, and its size.
type File struct {
	Key  domain.AssetKey
	Size int64
}

// World is a moment of the simplified world: the photo library and one
// remote root, no two albums of one place, every asset in one album that is
// not a folder, with one resource and at most one linked file. Objects holds
// every asset by its key, those in Recently Deleted too; Files holds every
// file by its path.
type World struct {
	Objects map[domain.AssetKey]Object
	Files   map[string]File
}

// Model is a world as a sync left it, Base, and as users changed it since,
// Now. Now has no links: a sync knows only those of Base.
type Model struct {
	Base, Now World
}

// Read returns w as a World, with the links that the truth of w gives, or
// the first way in which w is outside the simplified world. Folders and
// directories are not part of a World.
func Read(w gen.World) (World, error) {
	albums, places := map[string]Album{}, map[Album]bool{}
	for _, a := range w.Snapshot.Albums {
		if a.Kind == domain.KindFolder {
			continue
		}
		place := Album{Folder: strings.Join(a.FolderTitles, "/"), Title: a.Title}
		if places[place] {
			return World{}, fmt.Errorf("album %q is %q in %q, as another album is", a.UUID, a.Title, place.Folder)
		}
		albums[a.UUID], places[place] = place, true
	}
	m := World{Objects: map[domain.AssetKey]Object{}, Files: map[string]File{}}
	for _, a := range w.Snapshot.Assets {
		var album Album
		ok := len(a.AlbumUUIDs) == 1 && len(a.Resources) == 1
		if ok {
			album, ok = albums[a.AlbumUUIDs[0]]
		}
		if !ok {
			return World{}, fmt.Errorf("asset %q is in albums %q and has %d resources; want one album that is not a folder and one resource",
				a.Key.UUID, a.AlbumUUIDs, len(a.Resources))
		}
		m.Objects[a.Key] = Object{Album: album, Name: a.OriginalName, Size: a.Resources[0].Size, Trashed: a.InTrash}
	}
	linked := map[domain.AssetKey]string{}
	for _, f := range w.Files {
		if f.IsDir {
			continue
		}
		key, isLinked := w.Truth[f.Path]
		if other, twice := linked[key]; twice {
			return World{}, fmt.Errorf("asset %q has the files %q and %q; want one", key.UUID, other, f.Path)
		}
		if isLinked {
			linked[key] = f.Path
		}
		m.Files[f.Path] = File{Key: key, Size: f.Size}
	}
	return m, nil
}

// New returns the model of now after base, or the first way in which base
// or now is outside the simplified world, after "base: " or "now: ". New
// reads now without its truth and leaves now as it was.
func New(base, now gen.World) (Model, error) {
	b, err := Read(base)
	if err != nil {
		return Model{}, fmt.Errorf("base: %w", err)
	}
	now.Truth = nil
	n, err := Read(now)
	if err != nil {
		return Model{}, fmt.Errorf("now: %w", err)
	}
	return Model{Base: b, Now: n}, nil
}
