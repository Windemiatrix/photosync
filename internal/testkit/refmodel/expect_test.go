package refmodel_test

import (
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
	"github.com/Windemiatrix/photosync/internal/testkit/refmodel"
)

const holidaysDir = "2020/06-02 - Holidays"

// distinct is simple without PerturbSameName: no two files that a sync puts
// in one directory of its worlds share a name, and the model does not number
// names.
const distinct = simple &^ gen.PerturbSameName

var (
	holidays = refmodel.Album{Folder: "2020", Title: "06/02 Holidays"}
	lake     = refmodel.Album{Folder: "2020", Title: "07/03 Lake"}
)

// policies are the policies of the tests on gen worlds. Under a strict pair
// the sync removes what a side deleted and settles every conflict, so it
// links every file and every asset outside Recently Deleted.
var policies = []struct {
	deletes  domain.DeletesPolicy
	conflict domain.ConflictPolicy
	strict   bool
}{
	{domain.DeletesNone, domain.ConflictAsk, false},
	{domain.DeletesQuarantine, domain.ConflictRemoteWins, true},
	{domain.DeletesPropagate, domain.ConflictICloudWins, true},
	{domain.DeletesQuarantine, domain.ConflictSkip, false},
}

// before returns the World of synced with asset 4 and its file in Holidays,
// so that a link can move to an album other than the two it knows.
func before() refmodel.World {
	w := synced()
	w.Objects[key(4)] = refmodel.Object{Album: holidays, Name: "IMG_0004.HEIC", Size: 1_000_004}
	w.Files[holidaysDir+"/IMG_0004.HEIC"] = refmodel.File{Key: key(4), Size: 1_000_004}
	return w
}

func moveFile(w *refmodel.World, from, to string) {
	w.Files[to] = w.Files[from]
	delete(w.Files, from)
}

func place(w *refmodel.World, k domain.AssetKey, a refmodel.Album) {
	o := w.Objects[k]
	o.Album = a
	w.Objects[k] = o
}

func trash(w *refmodel.World, k domain.AssetKey) {
	o := w.Objects[k]
	o.Trashed = true
	w.Objects[k] = o
}

// imported adds to w the asset that a sync imports into the album a from the
// file at p of size bytes, keyed by that path as Expected keys it.
func imported(w *refmodel.World, p string, a refmodel.Album, size int64) {
	k := domain.AssetKey{UUID: p}
	w.Objects[k] = refmodel.Object{Album: a, Name: path.Base(p), Size: size}
	w.Files[p] = refmodel.File{Key: k, Size: size}
}

// cloneWorld returns w with maps of its own.
func cloneWorld(w refmodel.World) refmodel.World {
	return refmodel.World{Objects: maps.Clone(w.Objects), Files: maps.Clone(w.Files)}
}

func TestExpected(t *testing.T) {
	t.Parallel()

	const (
		p1   = tripDir + "/IMG_0001.HEIC"
		p2   = tripDir + "/IMG_0002.HEIC"
		p3   = trip2Dir + "/IMG_0003.HEIC"
		in2  = trip2Dir + "/IMG_0001.HEIC"
		inH  = holidaysDir + "/IMG_0001.HEIC"
		new1 = tripDir + "/NEW_0001.HEIC"
		new9 = tripDir + "/NEW_0009.HEIC"
		big  = 5_000_000
	)
	none, quarantine, propagate := domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate
	remote, icloud, ask, skip := domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip
	k1, k2, k3 := key(1), key(2), key(3)
	apart := func(m *refmodel.Model) {
		place(&m.Now, k1, trip2)
		moveFile(&m.Now, p1, inH)
	}
	waits := func(w *refmodel.World) {
		place(w, k1, trip2)
		delete(w.Files, p1)
		w.Files[inH] = refmodel.File{Size: 1_000_001}
	}

	for _, c := range []struct {
		name     string
		deletes  domain.DeletesPolicy
		conflict domain.ConflictPolicy
		edit     func(m *refmodel.Model)
		want     func(w *refmodel.World)
	}{
		{"nothing changed", quarantine, remote, func(*refmodel.Model) {}, func(*refmodel.World) {}},
		{
			"R-LINK-04: an asset moves to another album", quarantine, ask,
			func(m *refmodel.Model) { place(&m.Now, k1, trip2) },
			func(w *refmodel.World) {
				place(w, k1, trip2)
				moveFile(w, p1, in2)
			},
		},
		{
			"R-LINK-05: a file moves to another directory", quarantine, ask,
			func(m *refmodel.Model) { moveFile(&m.Now, p1, in2) },
			func(w *refmodel.World) {
				place(w, k1, trip2)
				moveFile(w, p1, in2)
			},
		},
		{
			"a file is renamed in its directory", quarantine, remote,
			func(m *refmodel.Model) { moveFile(&m.Now, p1, new1) },
			func(w *refmodel.World) { moveFile(w, p1, new1) },
		},
		{
			"R-LINK-06: a file is replaced", quarantine, remote,
			func(m *refmodel.Model) { m.Now.Files[p3] = refmodel.File{Size: big} },
			func(w *refmodel.World) {
				trash(w, k3)
				imported(w, p3, trip2, big)
			},
		},
		{
			"R-LINK-01: a new asset", quarantine, remote,
			func(m *refmodel.Model) {
				m.Now.Objects[key(9)] = refmodel.Object{Album: trip, Name: "NEW_0009.HEIC", Size: big + 9}
			},
			func(w *refmodel.World) {
				w.Objects[key(9)] = refmodel.Object{Album: trip, Name: "NEW_0009.HEIC", Size: big + 9}
				w.Files[new9] = refmodel.File{Key: key(9), Size: big + 9}
			},
		},
		{
			"a new asset in Recently Deleted", quarantine, remote,
			func(m *refmodel.Model) {
				m.Now.Objects[key(9)] = refmodel.Object{Album: trip, Name: "NEW_0009.HEIC", Size: big + 9, Trashed: true}
			},
			func(w *refmodel.World) {
				w.Objects[key(9)] = refmodel.Object{Album: trip, Name: "NEW_0009.HEIC", Size: big + 9, Trashed: true}
			},
		},
		{
			"R-LINK-02: a new file", quarantine, remote,
			func(m *refmodel.Model) { m.Now.Files[new9] = refmodel.File{Size: big + 9} },
			func(w *refmodel.World) { imported(w, new9, trip, big+9) },
		},
		{
			"R-LINK-08: an asset is deleted, deletes none", none, remote,
			func(m *refmodel.Model) { trash(&m.Now, k1) },
			func(w *refmodel.World) {
				trash(w, k1)
				w.Files[p1] = refmodel.File{Size: 1_000_001}
			},
		},
		{
			"R-LINK-08: an asset is deleted, deletes quarantine", quarantine, remote,
			func(m *refmodel.Model) { trash(&m.Now, k1) },
			func(w *refmodel.World) {
				trash(w, k1)
				delete(w.Files, p1)
			},
		},
		{
			"R-LINK-08: an asset is deleted, deletes propagate", propagate, remote,
			func(m *refmodel.Model) { trash(&m.Now, k1) },
			func(w *refmodel.World) {
				trash(w, k1)
				delete(w.Files, p1)
			},
		},
		{
			"R-LINK-08: an asset is gone from Photos, deletes none", none, remote,
			func(m *refmodel.Model) { delete(m.Now.Objects, k1) },
			func(w *refmodel.World) {
				delete(w.Objects, k1)
				w.Files[p1] = refmodel.File{Size: 1_000_001}
			},
		},
		{
			"R-LINK-08: an asset is deleted in an album without files", quarantine, remote,
			func(m *refmodel.Model) {
				place(&m.Now, k1, lake)
				trash(&m.Now, k1)
			},
			func(w *refmodel.World) {
				place(w, k1, lake)
				trash(w, k1)
				delete(w.Files, p1)
			},
		},
		{
			"R-LINK-09: a file is deleted, deletes none", none, remote,
			func(m *refmodel.Model) { delete(m.Now.Files, p3) },
			func(w *refmodel.World) { delete(w.Files, p3) },
		},
		{
			"R-LINK-09: a file is deleted, deletes quarantine", quarantine, remote,
			func(m *refmodel.Model) { delete(m.Now.Files, p3) },
			func(w *refmodel.World) {
				trash(w, k3)
				delete(w.Files, p3)
			},
		},
		{
			"R-LINK-09: a file is deleted, deletes propagate", propagate, remote,
			func(m *refmodel.Model) { delete(m.Now.Files, p3) },
			func(w *refmodel.World) {
				trash(w, k3)
				delete(w.Files, p3)
			},
		},
		{
			"R-LINK-10: an asset moves and its file is deleted", quarantine, remote,
			func(m *refmodel.Model) {
				place(&m.Now, k2, trip2)
				delete(m.Now.Files, p2)
			},
			func(w *refmodel.World) {
				place(w, k2, trip2)
				delete(w.Files, p2)
				w.Files[trip2Dir+"/IMG_002398.HEIC"] = refmodel.File{Key: k2, Size: 1_000_002}
			},
		},
		{
			"R-LINK-10: the file comes back with the size of its asset", quarantine, remote,
			func(m *refmodel.Model) {
				place(&m.Now, k3, trip)
				delete(m.Now.Files, p3)
			},
			func(w *refmodel.World) {
				place(w, k3, trip)
				delete(w.Files, p3)
				w.Files[tripDir+"/IMG_0003.HEIC"] = refmodel.File{Key: k3, Size: 1_000_003}
			},
		},
		{
			"R-LINK-11: an asset is deleted and its file moves", none, ask,
			func(m *refmodel.Model) {
				trash(&m.Now, k1)
				moveFile(&m.Now, p1, in2)
			},
			func(w *refmodel.World) {
				trash(w, k1)
				delete(w.Files, p1)
				imported(w, in2, trip2, 1_000_001)
			},
		},
		{
			"R-LINK-11: an asset is deleted and its file is replaced", none, remote,
			func(m *refmodel.Model) {
				trash(&m.Now, k1)
				m.Now.Files[p1] = refmodel.File{Size: big}
			},
			func(w *refmodel.World) {
				trash(w, k1)
				imported(w, p1, trip, big)
			},
		},
		{
			"R-LINK-11: the album of a deleted asset does not count", none, remote,
			func(m *refmodel.Model) {
				place(&m.Now, k1, trip2)
				trash(&m.Now, k1)
				m.Now.Files[p1] = refmodel.File{Size: big}
			},
			func(w *refmodel.World) {
				place(w, k1, trip2)
				trash(w, k1)
				imported(w, p1, trip, big)
			},
		},
		{
			"R-LINK-11: an asset is gone from Photos and its file moves", none, remote,
			func(m *refmodel.Model) {
				delete(m.Now.Objects, k1)
				moveFile(&m.Now, p1, in2)
			},
			func(w *refmodel.World) {
				delete(w.Objects, k1)
				delete(w.Files, p1)
				imported(w, in2, trip2, 1_000_001)
			},
		},
		{
			"R-LINK-12: both sides delete", none, remote,
			func(m *refmodel.Model) {
				trash(&m.Now, k1)
				delete(m.Now.Files, p1)
			},
			func(w *refmodel.World) {
				trash(w, k1)
				delete(w.Files, p1)
			},
		},
		{
			"R-LINK-13: the same move on both sides", quarantine, ask,
			func(m *refmodel.Model) {
				place(&m.Now, k1, trip2)
				moveFile(&m.Now, p1, in2)
			},
			func(w *refmodel.World) {
				place(w, k1, trip2)
				moveFile(w, p1, in2)
			},
		},
		{
			"R-LINK-14: an asset moves and its file is renamed", quarantine, ask,
			func(m *refmodel.Model) {
				place(&m.Now, k1, trip2)
				moveFile(&m.Now, p1, new1)
			},
			func(w *refmodel.World) {
				place(w, k1, trip2)
				moveFile(w, p1, trip2Dir+"/NEW_0001.HEIC")
			},
		},
		{
			"R-LINK-14: an asset moves and its file is replaced", quarantine, ask,
			func(m *refmodel.Model) {
				place(&m.Now, k3, trip)
				m.Now.Files[p3] = refmodel.File{Size: big}
			},
			func(w *refmodel.World) {
				place(w, k3, trip)
				trash(w, k3)
				delete(w.Files, p3)
				imported(w, tripDir+"/IMG_0003.HEIC", trip, big)
			},
		},
		{
			"R-LINK-07: moves apart, remote_wins", quarantine, remote, apart,
			func(w *refmodel.World) {
				place(w, k1, holidays)
				moveFile(w, p1, inH)
			},
		},
		{
			"R-LINK-07: moves apart, icloud_wins", quarantine, icloud, apart,
			func(w *refmodel.World) {
				place(w, k1, trip2)
				moveFile(w, p1, in2)
			},
		},
		{"R-LINK-07: moves apart, ask", quarantine, ask, apart, waits},
		{"R-LINK-07: moves apart, skip", quarantine, skip, apart, waits},
		{
			"two new paths of one size: no move", quarantine, remote,
			func(m *refmodel.Model) {
				moveFile(&m.Now, p1, in2)
				m.Now.Files[holidaysDir+"/NEW_0009.HEIC"] = refmodel.File{Size: 1_000_001}
			},
			func(w *refmodel.World) {
				trash(w, k1)
				delete(w.Files, p1)
				imported(w, in2, trip2, 1_000_001)
				imported(w, holidaysDir+"/NEW_0009.HEIC", holidays, 1_000_001)
			},
		},
		{
			"two gone files of one size: no move", quarantine, remote,
			func(m *refmodel.Model) {
				m.Base.Files[p2] = refmodel.File{Key: k2, Size: 1_000_001}
				delete(m.Now.Files, p1)
				delete(m.Now.Files, p2)
				m.Now.Files[in2] = refmodel.File{Size: 1_000_001}
			},
			func(w *refmodel.World) {
				trash(w, k1)
				trash(w, k2)
				delete(w.Files, p1)
				delete(w.Files, p2)
				imported(w, in2, trip2, 1_000_001)
			},
		},
	} {
		m := refmodel.Model{Base: before(), Now: unlinked(before())}
		c.edit(&m)
		was := refmodel.Model{Base: cloneWorld(m.Base), Now: cloneWorld(m.Now)}
		want := before()
		c.want(&want)
		got, err := m.Expected(c.deletes, c.conflict)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("%s:\n got %+v, error %v\nwant %+v", c.name, got, err, want)
		}
		if !reflect.DeepEqual(m, was) {
			t.Errorf("%s: Expected left the model %+v; want %+v", c.name, m, was)
		}
	}
}

func TestExpectedOutside(t *testing.T) {
	t.Parallel()

	for _, c := range []struct {
		name string
		edit func(m *refmodel.Model)
	}{
		{"an album renamed in Photos", func(m *refmodel.Model) {
			retitled := refmodel.Album{Folder: "2020", Title: "05/01 Trip 3"}
			place(&m.Now, key(1), retitled)
			place(&m.Now, key(2), retitled)
		}},
		{"a directory renamed in the cloud", func(m *refmodel.Model) {
			moveFile(&m.Now, tripDir+"/IMG_0001.HEIC", "2020/05-01 - Trip 3/IMG_0001.HEIC")
			moveFile(&m.Now, tripDir+"/IMG_0002.HEIC", "2020/05-01 - Trip 3/IMG_0002.HEIC")
		}},
		{"an asset in an album without files", func(m *refmodel.Model) { place(&m.Now, key(3), lake) }},
		{"a new asset in an album without files", func(m *refmodel.Model) {
			m.Now.Objects[key(9)] = refmodel.Object{Album: lake, Name: "NEW_0009.HEIC", Size: 5_000_009}
		}},
		{"a new file in a new directory", func(m *refmodel.Model) {
			m.Now.Files["2021/01-01 - Fair/NEW_0009.HEIC"] = refmodel.File{Size: 5_000_009}
		}},
	} {
		m := refmodel.Model{Base: before(), Now: unlinked(before())}
		c.edit(&m)
		got, err := m.Expected(domain.DeletesQuarantine, domain.ConflictRemoteWins)
		if err == nil || !reflect.DeepEqual(got, refmodel.World{}) {
			t.Errorf("%s: got %+v, error %v; want no world and an error", c.name, got, err)
		}
	}
}

// TestExpectedOnGenWorlds runs the model on samples drawn as in the actions
// test of package gen, on worlds of distinct names. Fixed seeds give the
// same samples on every run; those inside the model are what the property
// tests of planning can check against it.
func TestExpectedOnGenWorlds(t *testing.T) {
	t.Parallel()

	const samples = 100
	accepted, membership := 0, 0
	drawn := map[gen.Action]bool{}
	for seed := range samples {
		w := gen.Worlds(gen.Config{Frames: 6, Perturb: distinct}).Example(seed)
		actions := gen.Actions(w).Example(samples + seed)
		after := w.Apply(actions...)
		unchanged, err := refmodel.New(w, w)
		if err != nil {
			continue
		}
		m, err := refmodel.New(w, after)
		if err != nil {
			continue
		}
		out := renamed(w, after)
		for _, p := range policies {
			if got, err := unchanged.Expected(p.deletes, p.conflict); err != nil || !reflect.DeepEqual(got, unchanged.Base) {
				t.Errorf("seed %d, %s %s, unchanged: got %+v, error %v; want base %+v",
					seed, p.deletes, p.conflict, got, err, unchanged.Base)
			}
			got, err := m.Expected(p.deletes, p.conflict)
			switch {
			case out && (err == nil || !reflect.DeepEqual(got, refmodel.World{})):
				t.Errorf("seed %d, %s %s: got %+v, error %v; want no world and an error: an album or a directory is new",
					seed, p.deletes, p.conflict, got, err)
			case !out && err != nil:
				t.Errorf("seed %d, %s %s: %v; want no error", seed, p.deletes, p.conflict, err)
			case !out:
				if err := consistent(got, p.strict); err != nil {
					t.Errorf("seed %d, %s %s: %v", seed, p.deletes, p.conflict, err)
				}
				if !p.strict {
					continue
				}
				again := refmodel.Model{Base: got, Now: unlinked(cloneWorld(got))}
				if next, err := again.Expected(p.deletes, p.conflict); err != nil || !reflect.DeepEqual(next, got) {
					t.Errorf("seed %d, %s %s: a second sync gives %+v, error %v; want %+v",
						seed, p.deletes, p.conflict, next, err, got)
				}
			}
		}
		if out {
			continue
		}
		accepted++
		for _, a := range actions {
			drawn[gen.Action{Side: a.Side, Kind: a.Kind}] = true
		}
		if slices.ContainsFunc(actions, func(a gen.Action) bool { return a.Kind == gen.AlbumAdd || a.Kind == gen.AlbumRemove }) {
			membership++
		}
	}
	t.Logf("%d of %d samples lie inside the model; %d of them add an asset to an album or remove it from one",
		accepted, samples, membership)
	var missing []string
	if accepted < 10 {
		missing = append(missing, fmt.Sprintf("10 samples inside the model (%d are)", accepted))
	}
	for _, pair := range []gen.Action{
		{Side: gen.ICloud, Kind: gen.Create}, {Side: gen.ICloud, Kind: gen.Delete},
		{Side: gen.Remote, Kind: gen.Create}, {Side: gen.Remote, Kind: gen.Move}, {Side: gen.Remote, Kind: gen.Rename},
		{Side: gen.Remote, Kind: gen.Replace}, {Side: gen.Remote, Kind: gen.Delete},
	} {
		if !drawn[pair] {
			missing = append(missing, fmt.Sprintf("the pair %s %s inside it", pair.Side, pair.Kind))
		}
	}
	if len(missing) > 0 {
		t.Errorf("samples of seeds 0 to %d lack %s", samples-1, strings.Join(missing, "; "))
	}
}

// consistent returns a way in which w is not a world that a sync leaves, or
// nil: a file under .photosync/, a link to no asset, two files of one asset;
// when strict, also a file without a link or an asset outside Recently
// Deleted without a file.
func consistent(w refmodel.World, strict bool) error {
	linked := map[domain.AssetKey]string{}
	for p, f := range w.Files {
		_, known := w.Objects[f.Key]
		other, twice := linked[f.Key]
		switch {
		case strings.HasPrefix(p, ".photosync/"):
			return fmt.Errorf("file %q is in the service directory", p)
		case f.Key == (domain.AssetKey{}):
			if strict {
				return fmt.Errorf("file %q has no link", p)
			}
			continue
		case !known:
			return fmt.Errorf("file %q links %+v, which is no asset", p, f.Key)
		case twice:
			return fmt.Errorf("asset %+v has the files %q and %q", f.Key, other, p)
		}
		linked[f.Key] = p
	}
	for k, o := range w.Objects {
		if _, ok := linked[k]; strict && !ok && !o.Trashed {
			return fmt.Errorf("asset %+v has no file", k)
		}
	}
	return nil
}

// renamed reports whether now holds an asset outside Recently Deleted in an
// album, or a file in a directory, that no asset or file of base shows: the
// ways in which the actions of package gen take a world outside the model.
func renamed(base, now gen.World) bool {
	known := map[string]bool{}
	for _, f := range base.Files {
		known[path.Dir(f.Path)] = true
	}
	for _, a := range base.Snapshot.Assets {
		known[placeOf(base, a)] = true
	}
	for _, f := range now.Files {
		if !known[path.Dir(f.Path)] {
			return true
		}
	}
	return slices.ContainsFunc(now.Snapshot.Assets, func(a domain.Asset) bool {
		return !a.InTrash && !known[placeOf(now, a)]
	})
}

// placeOf returns the folders and title of the one album of a in w, or ""
// when a is not in one album.
func placeOf(w gen.World, a domain.Asset) string {
	for _, album := range w.Snapshot.Albums {
		if len(a.AlbumUUIDs) == 1 && album.UUID == a.AlbumUUIDs[0] {
			return fmt.Sprint(album.FolderTitles, album.Title)
		}
	}
	return ""
}
