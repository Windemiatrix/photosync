package gen_test

import (
	"errors"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

func TestActions(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 30).Draw(t, "frames")
		checkDrawnActions(t, gen.Worlds(gen.Config{Frames: n}).Draw(t, "world"))
	})
}

func TestActionsOnEdgeWorlds(t *testing.T) {
	t.Parallel()

	w := fixture()
	var trash, suffixes []gen.Action
	for _, x := range w.Snapshot.Assets {
		trash = append(trash, gen.Action{Side: gen.ICloud, Kind: gen.Delete, Key: x.Key})
	}
	for n := range 8 {
		suffixes = append(suffixes, gen.Action{
			Side: gen.Remote, Kind: gen.Create, Path: fmt.Sprintf("%s %d/IMG_%04d.HEIC", tripDir, n, 100+n),
			Size: 1, Taken: atHour(1),
		})
	}
	twin := gen.Action{Side: gen.Remote, Kind: gen.Rename, Path: w.Files[2].Path, To: trip2Dir + "/IMG_0001.HEIC"}
	for _, c := range []struct {
		name  string
		world gen.World
	}{
		{"no files", gen.New(1, gen.Config{})},
		{"all in Recently Deleted", w.Apply(trash...)},
		{"a name in two directories", w.Apply(twin)},
		{"directories with every suffix", w.Apply(suffixes...)},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if err := checkDrawnWorld(c.world); err != nil {
				t.Fatalf("the world does not hold together: %v", err)
			}
			rapid.Check(t, func(t *rapid.T) { checkDrawnActions(t, c.world) })
		})
	}
}

func TestActionsTwice(t *testing.T) {
	t.Parallel()

	actions := gen.Actions(gen.New(1, gen.Config{Frames: 6}))
	for seed := range 10 {
		if first, again := actions.Example(seed), actions.Example(seed); !reflect.DeepEqual(first, again) {
			t.Errorf("seed %d: one generator gave %+v, then %+v", seed, first, again)
		}
	}
}

func TestActionsNotVacuous(t *testing.T) {
	t.Parallel()

	const samples = 100
	pairs := []gen.Action{
		{Side: gen.ICloud, Kind: gen.Create}, {Side: gen.ICloud, Kind: gen.Delete},
		{Side: gen.ICloud, Kind: gen.AlbumAdd}, {Side: gen.ICloud, Kind: gen.AlbumRemove},
		{Side: gen.ICloud, Kind: gen.AlbumMove}, {Side: gen.ICloud, Kind: gen.AlbumRename},
		{Side: gen.Remote, Kind: gen.Create}, {Side: gen.Remote, Kind: gen.Move},
		{Side: gen.Remote, Kind: gen.Rename}, {Side: gen.Remote, Kind: gen.Replace},
		{Side: gen.Remote, Kind: gen.Delete}, {Side: gen.Remote, Kind: gen.AlbumRename},
	}
	drawn := map[gen.Action]bool{}
	acting, longest, switching := 0, 0, 0
	var conflict, moved, deleted bool
	for seed := range samples {
		w := gen.Worlds(gen.Config{Frames: 6}).Example(seed)
		actions := gen.Actions(w).Example(samples + seed)
		if len(actions) > 0 {
			acting++
		}
		longest = max(longest, len(actions))
		for _, a := range actions {
			drawn[gen.Action{Side: a.Side, Kind: a.Kind}] = true
		}
		after := w.Apply(actions...)
		switched := false
		for _, k := range w.Truth {
			asset, file, _ := drawnPair(w, k)
			assetAfter, fileAfter, kept := drawnPair(after, k)
			inPhotos, inCloud := !reflect.DeepEqual(asset, assetAfter), !reflect.DeepEqual(file, fileAfter)
			conflict = conflict || inPhotos && inCloud
			moved = moved || kept && fileAfter.Path != file.Path
			deleted = deleted || !kept || assetAfter.InTrash
			switched = switched || !inCloud && !assetAfter.InTrash && !slices.Equal(asset.AlbumUUIDs, assetAfter.AlbumUUIDs)
		}
		// The reference model reads only worlds where every asset is in one album.
		if switched && !slices.ContainsFunc(after.Snapshot.Assets, func(x domain.Asset) bool { return len(x.AlbumUUIDs) != 1 }) {
			switching++
		}
	}
	t.Logf("%d of %d samples act; the longest has %d actions; %d of %d pairs drawn; "+
		"%d move an asset to another album while its file stays and leave every asset in one album",
		acting, samples, longest, len(drawn), len(pairs), switching)
	var missing []string
	if acting*10 < samples*7 {
		missing = append(missing, fmt.Sprintf("an action in 70 %% of them (%d act)", acting))
	}
	if longest < 4 {
		missing = append(missing, fmt.Sprintf("a sequence of 4 actions (the longest has %d)", longest))
	}
	for _, p := range pairs {
		if !drawn[p] {
			missing = append(missing, fmt.Sprintf("the pair %s %s", p.Side, p.Kind))
		}
	}
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{conflict, "an asset and its file that both changed"},
		{moved, "a moved file of an asset"},
		{deleted, "a deleted asset or file of an asset"},
		{switching > 0, "an asset moved to another album while its file stays, every asset in one album"},
	} {
		if !c.ok {
			missing = append(missing, c.what)
		}
	}
	if len(missing) > 0 {
		t.Errorf("samples of seeds 0 to %d lack %s", samples-1, strings.Join(missing, "; "))
	}
}

func checkDrawnActions(t *rapid.T, w gen.World) {
	actions := gen.Actions(w).Draw(t, "actions")
	if err := checkDrawnWorld(w); err != nil {
		t.Fatalf("Actions broke the world it was given: %v", err)
	}
	if len(actions) > 8 {
		t.Fatalf("%d actions; want at most 8", len(actions))
	}
	last := -1
	for _, a := range actions {
		attempt, err := checkDrawnAction(w, a)
		if err != nil {
			t.Fatalf("%+v: %v", a, err)
		}
		if attempt >= 0 && attempt <= last {
			t.Fatalf("%+v carries the values of attempt %d after those of attempt %d; want each attempt once, in order",
				a, attempt, last)
		}
		last = max(last, attempt)
		next := w.Apply(a)
		if reflect.DeepEqual(next, w) {
			t.Fatalf("%+v changes nothing", a)
		}
		if err := checkDrawnWorld(next); err != nil {
			t.Fatalf("after %+v: %v", a, err)
		}
		w = next
	}
}

type drawnNew struct {
	name   string
	size   int64
	taken  time.Time
	suffix string
}

// drawnNews holds the new values of the attempts 0 to 7, as the contract of
// Actions gives them.
func drawnNews() []drawnNew {
	news := make([]drawnNew, 8)
	for n := range news {
		news[n] = drawnNew{
			name:   fmt.Sprintf("NEW_%04d", n),
			size:   int64(5_000_000 + n),
			taken:  time.Date(2025, time.January, 1+n, 12, 0, 0, 0, time.UTC),
			suffix: fmt.Sprintf(" %d", n),
		}
	}
	return news
}

// checkDrawnAction returns the attempt whose new values a carries, or -1 for
// a kind that carries none, and how a, drawn on w, breaks the contract of
// Actions, or nil.
func checkDrawnAction(w gen.World, a gen.Action) (int, error) {
	dirs, live, events := map[string]bool{}, map[domain.AssetKey]bool{}, map[string]string{}
	for _, f := range w.Files {
		dirs[path.Dir(f.Path)] = true
	}
	for _, x := range w.Snapshot.Assets {
		live[x.Key] = !x.InTrash
	}
	for _, al := range w.Snapshot.Albums {
		if al.Kind != domain.KindFolder {
			events[al.UUID] = al.Title
		}
	}
	title, isEvent := events[a.Album]
	free := !slices.ContainsFunc(w.Files, func(f domain.RemoteEntry) bool {
		return f.Path == a.To || strings.HasPrefix(f.Path, a.To+"/")
	})
	news, attempt := drawnNews(), -1
	var used gen.Action
	var ok bool
	var rule string
	switch {
	case a.Side == gen.ICloud && a.Kind == gen.Create:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool {
			return a.Path == v.name+".HEIC" && a.Size == v.size && a.Taken.Equal(v.taken) &&
				a.Key == domain.AssetKey{CloudAssetGUID: "guid-" + v.name, UUID: "uuid-" + v.name}
		})
		used = gen.Action{Side: a.Side, Kind: a.Kind, Key: a.Key, Album: a.Album, Path: a.Path, Size: a.Size, Taken: a.Taken}
		ok, rule = isEvent && attempt >= 0, "the new asset of an attempt, in an album that is not a folder"
	case a.Side == gen.ICloud && a.Kind == gen.Delete:
		used = gen.Action{Side: a.Side, Kind: a.Kind, Key: a.Key}
		ok, rule = live[a.Key], "an asset outside Recently Deleted"
	case a.Side == gen.ICloud && (a.Kind == gen.AlbumAdd || a.Kind == gen.AlbumMove):
		used = gen.Action{Side: a.Side, Kind: a.Kind, Key: a.Key, Album: a.Album}
		ok, rule = live[a.Key] && isEvent, "an asset outside Recently Deleted and an album that is not a folder"
	case a.Side == gen.ICloud && a.Kind == gen.AlbumRemove:
		used = gen.Action{Side: a.Side, Kind: a.Kind, Key: a.Key, Album: a.Album}
		ok, rule = live[a.Key], "an asset outside Recently Deleted"
	case a.Side == gen.ICloud && a.Kind == gen.AlbumRename:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool { return a.To == title+v.suffix })
		used = gen.Action{Side: a.Side, Kind: a.Kind, Album: a.Album, To: a.To}
		ok, rule = isEvent && attempt >= 0, "an album that is not a folder, with the suffix of an attempt"
	case a.Side == gen.Remote && a.Kind == gen.Create:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool {
			return path.Base(a.Path) == v.name+".HEIC" && a.Size == v.size && a.Taken.Equal(v.taken)
		})
		used = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path, Size: a.Size, Taken: a.Taken}
		ok, rule = dirs[path.Dir(a.Path)] && attempt >= 0, "the new file of an attempt, in the directory of a file"
	case a.Side == gen.Remote && a.Kind == gen.Move:
		used = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path, To: a.To}
		ok, rule = free && dirs[path.Dir(a.To)] && path.Base(a.To) == path.Base(a.Path),
			"a free path under the old name in the directory of another file"
	case a.Side == gen.Remote && a.Kind == gen.Rename:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool { return path.Base(a.To) == v.name+path.Ext(a.Path) })
		used = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path, To: a.To}
		ok, rule = free && path.Dir(a.To) == path.Dir(a.Path) && attempt >= 0,
			"a free path in the same directory, with the new name of an attempt and the old extension"
	case a.Side == gen.Remote && a.Kind == gen.Replace:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool { return a.Size == v.size })
		used = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path, Size: a.Size}
		ok, rule = attempt >= 0, "the new size of an attempt"
	case a.Side == gen.Remote && a.Kind == gen.Delete:
		used, ok = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path}, true
	case a.Side == gen.Remote && a.Kind == gen.AlbumRename:
		attempt = slices.IndexFunc(news, func(v drawnNew) bool { return a.To == a.Path+v.suffix })
		used = gen.Action{Side: a.Side, Kind: a.Kind, Path: a.Path, To: a.To}
		ok, rule = free && dirs[a.Path] && attempt >= 0,
			"the directory of a file, to a free path with the suffix of an attempt"
	default:
		return -1, errors.New("not one of the 12 pairs of side and kind")
	}
	if a != used {
		return attempt, fmt.Errorf("sets the fields that its kind does not use; want %+v", used)
	}
	if !ok {
		return attempt, fmt.Errorf("breaks the rule: %s", rule)
	}
	return attempt, nil
}

// checkDrawnWorld returns the first way in which w does not hold together,
// or nil.
func checkDrawnWorld(w gen.World) error {
	paths := map[string]bool{}
	for _, f := range w.Files {
		if paths[f.Path] {
			return fmt.Errorf("path %q is repeated", f.Path)
		}
		paths[f.Path] = true
	}
	albums := map[string]bool{}
	for _, al := range w.Snapshot.Albums {
		if al.UUID == "" || albums[al.UUID] {
			return fmt.Errorf("album UUID %q is empty or repeated", al.UUID)
		}
		albums[al.UUID] = true
	}
	keys, start := map[domain.AssetKey]bool{}, map[string]time.Time{}
	for _, x := range w.Snapshot.Assets {
		if x.Key == (domain.AssetKey{}) || keys[x.Key] {
			return fmt.Errorf("asset key %+v is empty or repeated", x.Key)
		}
		keys[x.Key] = true
		for _, id := range x.AlbumUUIDs {
			if !albums[id] {
				return fmt.Errorf("asset %q is in album %q, which the snapshot lacks", x.Key.UUID, id)
			}
			if s, seen := start[id]; !seen || x.Taken.Before(s) {
				start[id] = x.Taken
			}
		}
	}
	for p, k := range w.Truth {
		if !paths[p] || !keys[k] {
			return fmt.Errorf("the truth gives path %q the key %+v; want a file and an asset", p, k)
		}
	}
	for _, al := range w.Snapshot.Albums {
		if !al.Start.Equal(start[al.UUID]) {
			return fmt.Errorf("album %q starts at %v; want %v, its earliest asset", al.UUID, al.Start, start[al.UUID])
		}
	}
	return nil
}

// drawnPair returns the asset of k in w, the file that the truth of w gives
// k, and whether there is such a file.
func drawnPair(w gen.World, k domain.AssetKey) (domain.Asset, domain.RemoteEntry, bool) {
	var asset domain.Asset
	if i := slices.IndexFunc(w.Snapshot.Assets, func(x domain.Asset) bool { return x.Key == k }); i >= 0 {
		asset = w.Snapshot.Assets[i]
	}
	for _, f := range w.Files {
		if key, ok := w.Truth[f.Path]; ok && key == k {
			return asset, f, true
		}
	}
	return asset, domain.RemoteEntry{}, false
}
