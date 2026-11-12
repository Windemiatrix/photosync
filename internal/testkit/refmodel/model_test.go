package refmodel_test

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
	"github.com/Windemiatrix/photosync/internal/testkit/refmodel"
)

const (
	tripDir  = "2020/05-01 - Trip"
	trip2Dir = "2020/05-01 - Trip 2"
)

// simple is every perturbation that leaves an asset one resource and one
// file and makes no copies; the kinds are named, so that a kind added to gen
// later does not enter.
const simple = gen.PerturbImportRename | gen.PerturbRecompress | gen.PerturbZoneShift | gen.PerturbRoundSubSec |
	gen.PerturbNoDate | gen.PerturbBurst | gen.PerturbSameName | gen.PerturbNFD

var (
	trip  = refmodel.Album{Folder: "2020", Title: "05/01 Trip"}
	trip2 = refmodel.Album{Folder: "2020", Title: "05/01 Trip 2"}
)

func key(n int) domain.AssetKey {
	return domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", n), UUID: fmt.Sprintf("uuid-%d", n)}
}

func asset(n int, name, album string, size int64) domain.Asset {
	return domain.Asset{
		Key: key(n), Library: domain.LibraryPersonal, OriginalName: name,
		Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		AlbumUUIDs: []string{album},
	}
}

func file(p string, size int64) domain.RemoteEntry {
	return domain.RemoteEntry{Root: domain.LibraryPersonal, Path: p, Size: size}
}

// fixture returns a synced world: assets 1 and 2 in album-0, asset 3 in
// album-1, album-2 empty. Asset 2 was renamed at import and the file of
// asset 3 is smaller than its original, so that a name or a size taken from
// the wrong side shows.
func fixture() gen.World {
	return gen.World{
		Snapshot: domain.LibrarySnapshot{
			Albums: []domain.Album{
				{UUID: "folder-2020", Kind: domain.KindFolder, Title: "2020"},
				{UUID: "album-0", Title: "05/01 Trip", ParentUUID: "folder-2020", FolderTitles: []string{"2020"}},
				{UUID: "album-1", Title: "05/01 Trip 2", ParentUUID: "folder-2020", FolderTitles: []string{"2020"}},
				{UUID: "album-2", Title: "06/02 Holidays", ParentUUID: "folder-2020", FolderTitles: []string{"2020"}},
			},
			Assets: []domain.Asset{
				asset(1, "IMG_0001.HEIC", "album-0", 1_000_001),
				asset(2, "IMG_002398.HEIC", "album-0", 1_000_002),
				asset(3, "IMG_0003.HEIC", "album-1", 1_000_003),
			},
		},
		Files: []domain.RemoteEntry{
			file(tripDir+"/IMG_0001.HEIC", 1_000_001),
			file(tripDir+"/IMG_0002.HEIC", 1_000_002),
			file(trip2Dir+"/IMG_0003.HEIC", 900_003),
		},
		Truth: map[string]domain.AssetKey{
			tripDir + "/IMG_0001.HEIC":  key(1),
			tripDir + "/IMG_0002.HEIC":  key(2),
			trip2Dir + "/IMG_0003.HEIC": key(3),
		},
	}
}

// synced returns the World of fixture.
func synced() refmodel.World {
	return refmodel.World{
		Objects: map[domain.AssetKey]refmodel.Object{
			key(1): {Album: trip, Name: "IMG_0001.HEIC", Size: 1_000_001},
			key(2): {Album: trip, Name: "IMG_002398.HEIC", Size: 1_000_002},
			key(3): {Album: trip2, Name: "IMG_0003.HEIC", Size: 1_000_003},
		},
		Files: map[string]refmodel.File{
			tripDir + "/IMG_0001.HEIC":  {Key: key(1), Size: 1_000_001},
			tripDir + "/IMG_0002.HEIC":  {Key: key(2), Size: 1_000_002},
			trip2Dir + "/IMG_0003.HEIC": {Key: key(3), Size: 900_003},
		},
	}
}

// unlinked returns m with no file linked; it edits the files of m in place.
func unlinked(m refmodel.World) refmodel.World {
	for p, f := range m.Files {
		m.Files[p] = refmodel.File{Size: f.Size}
	}
	return m
}

func TestRead(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		edit func(w *gen.World)
		want func(m *refmodel.World)
	}{
		{"a synced world", func(*gen.World) {}, func(*refmodel.World) {}},
		{"no truth", func(w *gen.World) { w.Truth = nil }, func(m *refmodel.World) { *m = unlinked(*m) }},
		{
			"Recently Deleted",
			func(w *gen.World) { w.Snapshot.Assets[0].InTrash = true },
			func(m *refmodel.World) {
				o := m.Objects[key(1)]
				o.Trashed = true
				m.Objects[key(1)] = o
			},
		},
		{
			"two folders",
			func(w *gen.World) { w.Snapshot.Albums[2].FolderTitles = []string{"2020", "May"} },
			func(m *refmodel.World) {
				o := m.Objects[key(3)]
				o.Album.Folder = "2020/May"
				m.Objects[key(3)] = o
			},
		},
		{
			"a directory and a file without a link",
			func(w *gen.World) {
				w.Files = append(w.Files, domain.RemoteEntry{Root: domain.LibraryPersonal, Path: "2020", IsDir: true},
					file("2020/06-02 - Holidays/NEW_0001.HEIC", 5_000_000))
			},
			func(m *refmodel.World) {
				m.Files["2020/06-02 - Holidays/NEW_0001.HEIC"] = refmodel.File{Size: 5_000_000}
			},
		},
	} {
		w := fixture()
		c.edit(&w)
		want := synced()
		c.want(&want)
		got, err := refmodel.Read(w)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v, error %v\nwant %+v", c.name, got, err, want)
		}
	}
}

func TestReadOutside(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name, names string
		edit        func(w *gen.World)
	}{
		{"two albums of one place", "album-2", func(w *gen.World) { w.Snapshot.Albums[3].Title = "05/01 Trip" }},
		{"an asset in two albums", "uuid-1", func(w *gen.World) {
			w.Snapshot.Assets[0].AlbumUUIDs = []string{"album-0", "album-2"}
		}},
		{"an asset in no album", "uuid-3", func(w *gen.World) { w.Snapshot.Assets[2].AlbumUUIDs = nil }},
		{"an asset in a folder", "uuid-3", func(w *gen.World) { w.Snapshot.Assets[2].AlbumUUIDs = []string{"folder-2020"} }},
		{"an asset with two resources", "uuid-1", func(w *gen.World) {
			a := &w.Snapshot.Assets[0]
			a.Resources = append(a.Resources, domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 1_000_001})
		}},
		{"an asset with two files", "uuid-1", func(w *gen.World) {
			w.Files = append(w.Files, file(tripDir+"/IMG_0001.MOV", 1_000_001))
			w.Truth[tripDir+"/IMG_0001.MOV"] = key(1)
		}},
	} {
		w := fixture()
		c.edit(&w)
		if got, err := refmodel.Read(w); err == nil || !strings.Contains(err.Error(), c.names) {
			t.Errorf("%s: got %+v, error %v; want an error that names %q", c.name, got, err, c.names)
		}
	}
}

func TestNew(t *testing.T) {
	t.Parallel()

	now := fixture()
	now.Files = append(now.Files, file(tripDir+"/IMG_0001.MOV", 1_000_001))
	now.Truth[tripDir+"/IMG_0001.MOV"] = key(1)
	want := refmodel.Model{Base: synced(), Now: unlinked(synced())}
	want.Now.Files[tripDir+"/IMG_0001.MOV"] = refmodel.File{Size: 1_000_001}
	if got, err := refmodel.New(fixture(), now); err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, error %v\nwant %+v", got, err, want)
	}
	if len(now.Truth) != 4 {
		t.Errorf("New left the truth of now %v; want its 4 links", now.Truth)
	}

	broken := fixture()
	broken.Snapshot.Assets[0].AlbumUUIDs = nil
	for _, c := range []struct {
		prefix    string
		base, now gen.World
	}{
		{"base: ", broken, fixture()},
		{"now: ", fixture(), broken},
	} {
		got, err := refmodel.New(c.base, c.now)
		if err == nil || !strings.HasPrefix(err.Error(), c.prefix) || !strings.Contains(err.Error(), "uuid-1") {
			t.Errorf("%sgot %+v, error %v; want an error that starts with %q and names uuid-1", c.prefix, got, err, c.prefix)
		}
	}
}

// TestNewOnGenWorlds reads samples drawn as in the actions test of package
// gen, on simple worlds. Fixed seeds give the same share of samples inside
// the simplified world on every run, the share that the property tests of
// planning have to work with.
func TestNewOnGenWorlds(t *testing.T) {
	t.Parallel()

	const samples = 100
	inside, membership := 0, 0
	drawn := map[gen.Action]bool{}
	for seed := range samples {
		w := gen.Worlds(gen.Config{Frames: 6, Perturb: simple}).Example(seed)
		actions := gen.Actions(w).Example(samples + seed)
		after := w.Apply(actions...)
		for _, c := range []struct {
			name string
			now  gen.World
			out  bool
		}{
			{"unchanged", w, outside(w)},
			{"after actions", after, outside(w) || outside(after)},
		} {
			m, err := refmodel.New(w, c.now)
			switch {
			case c.out && err == nil:
				t.Errorf("seed %d, %s: no error; want one: the world is outside", seed, c.name)
			case !c.out && err != nil:
				t.Errorf("seed %d, %s: %v; want no error", seed, c.name, err)
			case !c.out:
				if err := checkModel(m, w, c.now); err != nil {
					t.Errorf("seed %d, %s: %v", seed, c.name, err)
				}
			}
		}
		if outside(w) || outside(after) {
			continue
		}
		inside++
		for _, a := range actions {
			drawn[gen.Action{Side: a.Side, Kind: a.Kind}] = true
		}
		if slices.ContainsFunc(actions, func(a gen.Action) bool { return a.Kind == gen.AlbumAdd || a.Kind == gen.AlbumRemove }) {
			membership++
		}
	}
	t.Logf("%d of %d samples lie inside the simplified world; %d of them add an asset to an album or remove it from one",
		inside, samples, membership)
	var missing []string
	if inside < 20 {
		missing = append(missing, fmt.Sprintf("20 samples inside the simplified world (%d are)", inside))
	}
	for _, p := range []gen.Action{
		{Side: gen.ICloud, Kind: gen.Create}, {Side: gen.ICloud, Kind: gen.Delete}, {Side: gen.ICloud, Kind: gen.AlbumRename},
		{Side: gen.Remote, Kind: gen.Create}, {Side: gen.Remote, Kind: gen.Move}, {Side: gen.Remote, Kind: gen.Rename},
		{Side: gen.Remote, Kind: gen.Replace}, {Side: gen.Remote, Kind: gen.Delete}, {Side: gen.Remote, Kind: gen.AlbumRename},
	} {
		if !drawn[p] {
			missing = append(missing, fmt.Sprintf("the pair %s %s inside it", p.Side, p.Kind))
		}
	}
	if len(missing) > 0 {
		t.Errorf("samples of seeds 0 to %d lack %s", samples-1, strings.Join(missing, "; "))
	}
}

// outside reports whether w leaves the simplified world in one of the two
// ways open to a simple world of package gen and its actions: two albums of
// one place, or an asset in other than one album.
func outside(w gen.World) bool {
	places := map[string]bool{}
	for _, a := range w.Snapshot.Albums {
		if a.Kind == domain.KindFolder {
			continue
		}
		place := fmt.Sprint(a.FolderTitles, a.Title)
		if places[place] {
			return true
		}
		places[place] = true
	}
	return slices.ContainsFunc(w.Snapshot.Assets, func(a domain.Asset) bool { return len(a.AlbumUUIDs) != 1 })
}

// checkModel returns the first way in which m is not base and now as read:
// an asset or a file missing, a file of base without the link that the truth
// of base gives it, a file of now with a link; or nil.
func checkModel(m refmodel.Model, base, now gen.World) error {
	if len(m.Base.Objects) != len(base.Snapshot.Assets) || len(m.Now.Objects) != len(now.Snapshot.Assets) ||
		len(m.Base.Files) != len(base.Files) || len(m.Now.Files) != len(now.Files) {
		return fmt.Errorf("the model has %d and %d assets, %d and %d files; want %d and %d, %d and %d",
			len(m.Base.Objects), len(m.Now.Objects), len(m.Base.Files), len(m.Now.Files),
			len(base.Snapshot.Assets), len(now.Snapshot.Assets), len(base.Files), len(now.Files))
	}
	for _, f := range base.Files {
		if got, ok := m.Base.Files[f.Path]; !ok || got.Key != base.Truth[f.Path] {
			return fmt.Errorf("base file %q is %+v; want the link %+v", f.Path, got, base.Truth[f.Path])
		}
	}
	for _, f := range now.Files {
		if got, ok := m.Now.Files[f.Path]; !ok || got.Key != (domain.AssetKey{}) {
			return fmt.Errorf("now file %q is %+v; want it without a link", f.Path, got)
		}
	}
	return nil
}
