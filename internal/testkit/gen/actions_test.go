package gen_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

const (
	tripDir  = "2020/05-01 - Trip"
	trip2Dir = "2020/05-01 - Trip 2"
)

func atHour(h int) time.Time { return time.Date(2020, time.May, 1, h, 0, 0, 0, time.UTC) }

func photo(n, hour int, albums []string) domain.Asset {
	name := fmt.Sprintf("IMG_%04d.HEIC", n)
	offset := 0
	return domain.Asset{
		Key:     domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", n), UUID: fmt.Sprintf("uuid-%d", n)},
		Library: domain.LibraryPersonal, OriginalName: name, DateSource: domain.DateICloud,
		Taken: atHour(hour), TZOffset: &offset, SubSec: fmt.Sprintf("%03d", n), Width: 4032, Height: 3024,
		Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: int64(1_000_000 + n)}},
		AlbumUUIDs: albums,
	}
}

func fileOf(x domain.Asset, dir string) domain.RemoteEntry {
	offset := *x.TZOffset
	return domain.RemoteEntry{
		Root: domain.LibraryPersonal, Path: dir + "/" + x.OriginalName, Size: x.Resources[0].Size,
		Meta: domain.MediaMeta{Taken: x.Taken, TZOffset: &offset, SubSec: x.SubSec, Width: x.Width, Height: x.Height},
	}
}

// fixture gives shot 1 an album list with room to grow in place, so that an
// append without a copy shows.
func fixture() gen.World {
	years := []string{"2020"}
	a := photo(1, 11, append(make([]string, 0, 2), "album-0"))
	b := photo(2, 12, []string{"album-0"})
	c := photo(3, 13, []string{"album-1"})
	f1, f2, f3 := fileOf(a, tripDir), fileOf(b, tripDir), fileOf(c, trip2Dir)
	return gen.World{
		Snapshot: domain.LibrarySnapshot{
			Albums: []domain.Album{
				{UUID: "folder-2020", Kind: domain.KindFolder, Title: "2020"},
				{UUID: "album-0", Title: "05/01 Trip", ParentUUID: "folder-2020", FolderTitles: years, Start: atHour(11)},
				{UUID: "album-1", Title: "05/01 Trip 2", ParentUUID: "folder-2020", FolderTitles: years, Start: atHour(13)},
				{UUID: "album-2", Title: "06/02 Holidays", ParentUUID: "folder-2020", FolderTitles: years},
			},
			Assets: []domain.Asset{a, b, c},
		},
		Files: []domain.RemoteEntry{f1, f2, f3},
		Truth: map[string]domain.AssetKey{f1.Path: a.Key, f2.Path: b.Key, f3.Path: c.Key},
	}
}

type applyCase struct {
	name    string
	actions []gen.Action
	edit    func(want *gen.World)
}

func applyCases() []applyCase {
	w := fixture()
	a, b, c := w.Snapshot.Assets[0].Key, w.Snapshot.Assets[1].Key, w.Snapshot.Assets[2].Key
	f1, f2, f3 := w.Files[0].Path, w.Files[1].Path, w.Files[2].Path
	newKey := domain.AssetKey{CloudAssetGUID: "guid-new", UUID: "uuid-new"}
	noKey := domain.AssetKey{CloudAssetGUID: "guid-9", UUID: "uuid-9"}
	moved := trip2Dir + "/IMG_0001.HEIC"
	renamed := tripDir + "/NEW_0002.HEIC"
	beach := "2020/05-01 - Beach"
	created := "2020/06-02 - Holidays/NEW_0001.HEIC"
	none := "2020/none.HEIC"
	return []applyCase{
		{
			name: "photos: create",
			actions: []gen.Action{{
				Side: gen.ICloud, Kind: gen.Create, Key: newKey, Album: "album-1",
				Path: "NEW_0001.HEIC", Size: 5_000_000, Taken: atHour(10),
			}},
			edit: func(want *gen.World) {
				want.Snapshot.Assets = append(want.Snapshot.Assets, domain.Asset{
					Key: newKey, Library: domain.LibraryPersonal, OriginalName: "NEW_0001.HEIC",
					DateSource: domain.DateICloud, Taken: atHour(10),
					Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: "NEW_0001.HEIC", Size: 5_000_000}},
					AlbumUUIDs: []string{"album-1"},
				})
				want.Snapshot.Albums[2].Start = atHour(10)
			},
		},
		{
			name: "photos: delete",
			actions: []gen.Action{
				{Side: gen.ICloud, Kind: gen.Delete, Key: a},
				{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: c, Album: "album-0"},
			},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[0].InTrash = true
				want.Snapshot.Assets[2].AlbumUUIDs = []string{"album-1", "album-0"}
			},
		},
		{
			name:    "photos: add to an album",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a, Album: "album-1"}},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[0].AlbumUUIDs = []string{"album-0", "album-1"}
				want.Snapshot.Albums[2].Start = atHour(11)
			},
		},
		{
			name:    "photos: add to an album it is in",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a, Album: "album-0"}},
			edit:    func(*gen.World) {},
		},
		{
			name: "photos: remove from an album",
			actions: []gen.Action{
				{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a, Album: "album-1"},
				{Side: gen.ICloud, Kind: gen.AlbumRemove, Key: a, Album: "album-0"},
			},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[0].AlbumUUIDs = []string{"album-1"}
				want.Snapshot.Albums[1].Start = atHour(12)
				want.Snapshot.Albums[2].Start = atHour(11)
			},
		},
		{
			name:    "photos: remove the last album",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumRemove, Key: c, Album: "album-1"}},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[2].AlbumUUIDs = []string{}
				want.Snapshot.Albums[2].Start = time.Time{}
			},
		},
		{
			name:    "photos: move to another album",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumMove, Key: a, Album: "album-1"}},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[0].AlbumUUIDs = []string{"album-1"}
				want.Snapshot.Albums[1].Start = atHour(12)
				want.Snapshot.Albums[2].Start = atHour(11)
			},
		},
		{
			name: "photos: move out of two albums",
			actions: []gen.Action{
				{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a, Album: "album-1"},
				{Side: gen.ICloud, Kind: gen.AlbumMove, Key: a, Album: "album-2"},
			},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[0].AlbumUUIDs = []string{"album-2"}
				want.Snapshot.Albums[1].Start = atHour(12)
				want.Snapshot.Albums[3].Start = atHour(11)
			},
		},
		{
			name:    "photos: move to the album it is in",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumMove, Key: a, Album: "album-0"}},
			edit:    func(*gen.World) {},
		},
		{
			name: "photos: move an asset in no album",
			actions: []gen.Action{
				{Side: gen.ICloud, Kind: gen.AlbumRemove, Key: c, Album: "album-1"},
				{Side: gen.ICloud, Kind: gen.AlbumMove, Key: c, Album: "album-2"},
			},
			edit: func(want *gen.World) {
				want.Snapshot.Assets[2].AlbumUUIDs = []string{"album-2"}
				want.Snapshot.Albums[2].Start = time.Time{}
				want.Snapshot.Albums[3].Start = atHour(13)
			},
		},
		{
			name:    "photos: rename an album",
			actions: []gen.Action{{Side: gen.ICloud, Kind: gen.AlbumRename, Album: "album-1", To: "05/01 Beach"}},
			edit:    func(want *gen.World) { want.Snapshot.Albums[2].Title = "05/01 Beach" },
		},
		{
			name:    "cloud: create",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.Create, Path: created, Size: 5_000_000, Taken: atHour(10)}},
			edit: func(want *gen.World) {
				want.Files = append(want.Files, domain.RemoteEntry{
					Root: domain.LibraryPersonal, Path: created, Size: 5_000_000, Meta: domain.MediaMeta{Taken: atHour(10)},
				})
			},
		},
		{
			name: "cloud: move a new file",
			actions: []gen.Action{
				{Side: gen.Remote, Kind: gen.Create, Path: created, Size: 5_000_000, Taken: atHour(10)},
				{Side: gen.Remote, Kind: gen.Move, Path: created, To: moved},
			},
			edit: func(want *gen.World) {
				want.Files = append(want.Files, domain.RemoteEntry{
					Root: domain.LibraryPersonal, Path: moved, Size: 5_000_000, Meta: domain.MediaMeta{Taken: atHour(10)},
				})
			},
		},
		{
			name:    "cloud: move",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.Move, Path: f1, To: moved}},
			edit: func(want *gen.World) {
				want.Files[0].Path = moved
				delete(want.Truth, f1)
				want.Truth[moved] = a
			},
		},
		{
			name:    "cloud: rename",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.Rename, Path: f2, To: renamed}},
			edit: func(want *gen.World) {
				want.Files[1].Path = renamed
				delete(want.Truth, f2)
				want.Truth[renamed] = b
			},
		},
		{
			name:    "cloud: replace",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.Replace, Path: f3, Size: 5_000_001}},
			edit:    func(want *gen.World) { want.Files[2].Size = 5_000_001 },
		},
		{
			name:    "cloud: delete",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.Delete, Path: f1}},
			edit: func(want *gen.World) {
				want.Files = want.Files[1:]
				delete(want.Truth, f1)
			},
		},
		{
			name:    "cloud: rename a directory",
			actions: []gen.Action{{Side: gen.Remote, Kind: gen.AlbumRename, Path: tripDir, To: beach}},
			edit: func(want *gen.World) {
				want.Files[0].Path = beach + "/IMG_0001.HEIC"
				want.Files[1].Path = beach + "/IMG_0002.HEIC"
				delete(want.Truth, f1)
				delete(want.Truth, f2)
				want.Truth[beach+"/IMG_0001.HEIC"] = a
				want.Truth[beach+"/IMG_0002.HEIC"] = b
			},
		},
		{
			name: "cloud: in order",
			actions: []gen.Action{
				{Side: gen.Remote, Kind: gen.Move, Path: f1, To: moved},
				{Side: gen.Remote, Kind: gen.Delete, Path: moved},
			},
			edit: func(want *gen.World) {
				want.Files = want.Files[1:]
				delete(want.Truth, f1)
			},
		},
		{
			name: "nothing to act on",
			actions: []gen.Action{
				{Side: gen.ICloud, Kind: gen.Delete, Key: noKey},
				{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a, Album: "album-9"},
				{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: noKey, Album: "album-1"},
				{Side: gen.ICloud, Kind: gen.AlbumRemove, Key: noKey, Album: "album-0"},
				{Side: gen.ICloud, Kind: gen.AlbumMove, Key: a, Album: "album-9"},
				{Side: gen.ICloud, Kind: gen.AlbumMove, Key: noKey, Album: "album-1"},
				{
					Side: gen.ICloud, Kind: gen.Create, Key: newKey, Album: "album-9",
					Path: "NEW_0001.HEIC", Size: 5_000_000, Taken: atHour(10),
				},
				{
					Side: gen.ICloud, Kind: gen.Create, Key: a, Album: "album-1",
					Path: "NEW_0002.HEIC", Size: 5_000_000, Taken: atHour(10),
				},
				{Side: gen.ICloud, Kind: gen.AlbumRename, Album: "album-9", To: "05/01 Beach"},
				{Side: gen.ICloud, Kind: gen.Move, Key: a, Album: "album-1"},
				{Side: gen.Remote, Kind: gen.Create, Path: f1, Size: 5_000_000, Taken: atHour(10)},
				{Side: gen.Remote, Kind: gen.Move, Path: none, To: "2020/other.HEIC"},
				{Side: gen.Remote, Kind: gen.Replace, Path: none, Size: 1},
				{Side: gen.Remote, Kind: gen.Delete, Path: none},
				{Side: gen.Remote, Kind: gen.AlbumAdd, Path: f1, Album: "album-1"},
				{Side: gen.Remote, Kind: gen.AlbumMove, Key: a, Album: "album-1"},
				{Kind: gen.Delete, Key: a, Path: f1},
			},
			edit: func(*gen.World) {},
		},
	}
}

func TestApply(t *testing.T) {
	t.Parallel()

	if err := check(fixture(), 3); err != nil {
		t.Fatalf("the fixture is not a world: %v", err)
	}
	for _, c := range applyCases() {
		want := fixture()
		c.edit(&want)
		if got := fixture().Apply(c.actions...); !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v\nwant %+v", c.name, got, want)
		}
	}
}

func TestApplyWithoutTruth(t *testing.T) {
	t.Parallel()

	w := fixture()
	w.Truth = nil
	got := w.Apply(gen.Action{Side: gen.Remote, Kind: gen.Move, Path: w.Files[0].Path, To: "2020/moved.HEIC"})
	if got.Truth != nil {
		t.Errorf("Truth = %v after a move in a world without truth, want nil", got.Truth)
	}
	if got.Files[0].Path != "2020/moved.HEIC" {
		t.Errorf("Files[0].Path = %q, want 2020/moved.HEIC", got.Files[0].Path)
	}
}

func TestApplyLeavesInput(t *testing.T) {
	t.Parallel()

	w := fixture()
	for _, c := range applyCases() {
		_ = w.Apply(c.actions...)
		if want := fixture(); !reflect.DeepEqual(w, want) {
			t.Errorf("%s: Apply changed the world it was given:\n got %+v\nwant %+v", c.name, w, want)
		}
	}

	key := w.Snapshot.Assets[0].Key
	first := w.Apply(gen.Action{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: key, Album: "album-1"})
	_ = w.Apply(gen.Action{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: key, Album: "album-2"})
	if albums := first.Snapshot.Assets[0].AlbumUUIDs; !slices.Equal(albums, []string{"album-0", "album-1"}) {
		t.Errorf("a later Apply changed an earlier result: albums %q, want [album-0 album-1]", albums)
	}
}
