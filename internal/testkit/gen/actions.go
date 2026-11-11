package gen

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Side is where a user acts: Photos or the cloud.
type Side string

// The sides.
const (
	ICloud Side = "icloud"
	Remote Side = "remote"
)

// ActionKind is what a user does. In Photos a user creates and deletes an
// asset, adds it to an album, removes it from one and renames an album. In
// the cloud a user creates a file, moves it to another directory, renames it
// in its own, replaces its content and deletes it, and renames a directory.
// Apply ignores the other pairs of side and kind.
type ActionKind string

// The kinds of actions.
const (
	Create      ActionKind = "create"
	Move        ActionKind = "move"
	Rename      ActionKind = "rename"
	Replace     ActionKind = "replace"
	Delete      ActionKind = "delete"
	AlbumAdd    ActionKind = "album_add"
	AlbumRemove ActionKind = "album_remove"
	AlbumRename ActionKind = "album_rename"
)

// Action is one change that a user makes on one side; the fields that its
// kind does not use are zero. In Photos, Key is the asset and Album the UUID
// of the album that a new asset goes to, that Key joins or leaves, or that
// takes the title To. A Create adds a photo with the key Key whose original,
// named Path, has Size bytes and was taken at Taken. A Delete moves Key to
// Recently Deleted, where it stays in its albums. In the cloud, Path is a
// file, or the directory of an AlbumRename, and To its new path. A Create
// adds a file of Size bytes taken at Taken to the personal root; a Replace
// gives the file at Path new content of Size bytes and the same metadata.
type Action struct {
	Side  Side
	Kind  ActionKind
	Key   domain.AssetKey
	Album string
	Path  string
	To    string
	Size  int64
	Taken time.Time
}

// Apply returns w after actions, in their order, and leaves w as it was. An
// action on an asset, album or path that w does not hold, and a Create of a
// key or path that w holds, change nothing. Truth follows the files that move
// and drops the deleted ones. The Start of an album follows the assets that
// join or leave it and counts those in Recently Deleted.
func (w World) Apply(actions ...Action) World {
	w.Snapshot.Assets = slices.Clone(w.Snapshot.Assets)
	w.Snapshot.Albums = slices.Clone(w.Snapshot.Albums)
	w.Files = slices.Clone(w.Files)
	w.Truth = maps.Clone(w.Truth)
	for _, a := range actions {
		switch a.Side {
		case ICloud:
			w.inICloud(a)
		case Remote:
			w.inRemote(a)
		}
	}
	return w
}

func (w *World) inICloud(a Action) {
	assets, albums := w.Snapshot.Assets, w.Snapshot.Albums
	i := slices.IndexFunc(assets, func(x domain.Asset) bool { return x.Key == a.Key })
	j := slices.IndexFunc(albums, func(x domain.Album) bool { return x.UUID == a.Album })
	switch {
	case a.Kind == Create && i < 0 && j >= 0:
		w.Snapshot.Assets = append(assets, domain.Asset{
			Key: a.Key, Library: domain.LibraryPersonal, OriginalName: a.Path, DateSource: domain.DateICloud, Taken: a.Taken,
			Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: a.Path, Size: a.Size}},
			AlbumUUIDs: []string{a.Album},
		})
	case a.Kind == Delete && i >= 0:
		assets[i].InTrash = true
	case a.Kind == AlbumAdd && i >= 0 && j >= 0 && !slices.Contains(assets[i].AlbumUUIDs, a.Album):
		assets[i].AlbumUUIDs = slices.Concat(assets[i].AlbumUUIDs, []string{a.Album})
	case a.Kind == AlbumRemove && i >= 0:
		assets[i].AlbumUUIDs = slices.DeleteFunc(slices.Clone(assets[i].AlbumUUIDs), func(u string) bool { return u == a.Album })
	case a.Kind == AlbumRename && j >= 0:
		albums[j].Title = a.To
	}
	if j >= 0 {
		albums[j].Start = earliest(w.Snapshot.Assets, a.Album)
	}
}

func earliest(assets []domain.Asset, album string) time.Time {
	var start time.Time
	for _, x := range assets {
		if slices.Contains(x.AlbumUUIDs, album) && (start.IsZero() || x.Taken.Before(start)) {
			start = x.Taken
		}
	}
	return start
}

func (w *World) inRemote(a Action) {
	k := slices.IndexFunc(w.Files, func(f domain.RemoteEntry) bool { return f.Path == a.Path })
	switch {
	case a.Kind == Create && k < 0:
		w.Files = append(w.Files, domain.RemoteEntry{
			Root: domain.LibraryPersonal, Path: a.Path, Size: a.Size, Meta: domain.MediaMeta{Taken: a.Taken},
		})
	case a.Kind == Replace && k >= 0:
		w.Files[k].Size = a.Size
	case a.Kind == Delete && k >= 0:
		w.Files = slices.Delete(w.Files, k, k+1)
		delete(w.Truth, a.Path)
	case a.Kind == Move || a.Kind == Rename || a.Kind == AlbumRename:
		for i, f := range w.Files {
			if f.Path != a.Path && !strings.HasPrefix(f.Path, a.Path+"/") {
				continue
			}
			to := a.To + strings.TrimPrefix(f.Path, a.Path)
			w.Files[i].Path = to
			if key, ok := w.Truth[f.Path]; ok {
				delete(w.Truth, f.Path)
				w.Truth[to] = key
			}
		}
	}
}
