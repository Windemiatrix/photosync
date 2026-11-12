package match_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

func matchPairs(ps []match.Pair) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Key.UUID+" "+string(p.Level)+" "+describe([]match.Candidate{p.Candidate})[0])
	}
	return out
}

func filePlaces(cs []match.Candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, place(c.Main))
		for _, e := range []*domain.RemoteEntry{c.Live, c.RAW} {
			if e != nil {
				out = append(out, place(*e))
			}
		}
		out = append(out, places(c.Sidecars)...)
	}
	return out
}

// apart gives the Live video and RAW of each candidate a candidate of its
// own, as Group does when their stem holds another still.
func apart(cs []match.Candidate) []match.Candidate {
	var out []match.Candidate
	for _, c := range cs {
		for _, e := range []*domain.RemoteEntry{c.Live, c.RAW} {
			if e != nil {
				out = append(out, match.Candidate{Main: *e, Role: domain.RoleOriginal})
			}
		}
		c.Live, c.RAW = nil, nil
		out = append(out, c)
	}
	return out
}

func withoutComponents(as []domain.Asset) []domain.Asset {
	var out []domain.Asset
	for _, a := range as {
		a.Resources = slices.DeleteFunc(slices.Clone(a.Resources), func(r domain.Resource) bool { return r.Role != domain.ResourceOriginal })
		out = append(out, a)
	}
	return out
}

// Written from what a first sync must pair, not from the code. A shot is
// taken at 10:00:00.5 on 1 May 2024 at +03:00 and keeps 100 bytes of its
// original; its photo lies in the folder of that day with the same time and
// size. A Live video or a RAW beside it has the same size and no metadata.
func TestMatch(t *testing.T) {
	t.Parallel()
	if pairs, left, alone := match.Match(nil, nil); pairs != nil || left != nil || alone != nil {
		t.Errorf("Match(nil, nil) = %#v, %#v, %#v; want nil, nil, nil", pairs, left, alone)
	}
	plus3 := 3 * 3600
	at := time.Date(2024, 5, 1, 7, 0, 0, 0, time.UTC)
	const day = "2024/05-01 - Trip/"
	live, rawPair := domain.ResourceLiveVideo, domain.ResourceRAWPair
	shot := func(uuid, name string, roles ...domain.ResourceRole) domain.Asset {
		a := byTimeAsset(uuid, name, 100, at, &plus3, "500")
		for _, r := range roles {
			a.Resources = append(a.Resources, domain.Resource{Role: r, Filename: name, Size: 100})
		}
		return a
	}
	clip := byTimeAsset("v", "IMG_0001.MOV", 100, at, &plus3, "")
	clip.IsVideo = true
	file := func(name string) match.Candidate { return byTimeFile(day+name, 100, at, &plus3, "500") }
	lone := func(p string) match.Candidate {
		return match.Candidate{Main: domain.RemoteEntry{Root: domain.LibraryShared, Path: p, Size: 100}, Role: domain.RoleOriginal}
	}
	personal := func(c match.Candidate) match.Candidate {
		c.Main.Root = domain.LibraryPersonal
		return c
	}
	withLive := func(c match.Candidate, name string) match.Candidate {
		e := lone(day + name).Main
		c.Live = &e
		return c
	}
	withRAW := func(c match.Candidate, name string) match.Candidate {
		e := lone(day + name).Main
		c.RAW = &e
		return c
	}
	cases := []struct {
		name               string
		assets             []domain.Asset
		files              []match.Candidate
		pairs, left, alone []string
	}{
		{
			name:   "a Live Photo",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{withLive(file("IMG_0001.HEIC"), "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a RAW pair",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", rawPair)},
			files:  []match.Candidate{withRAW(file("IMG_0001.HEIC"), "IMG_0001.DNG")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC raw=2024/05-01 - Trip/IMG_0001.DNG"},
		},
		{
			name:   "a Live video that the asset lacks",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC")},
			files:  []match.Candidate{withLive(file("IMG_0001.HEIC"), "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a RAW that the asset lacks",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC")},
			files:  []match.Candidate{withRAW(file("IMG_0001.HEIC"), "IMG_0001.DNG")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.DNG"},
		},
		{
			name:   "a RAW but no Live video",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", rawPair)},
			files:  []match.Candidate{withRAW(withLive(file("IMG_0001.HEIC"), "IMG_0001.MOV"), "IMG_0001.DNG")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC raw=2024/05-01 - Trip/IMG_0001.DNG"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a lone Live video",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/IMG_0001.MOV"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG"},
		},
		{
			name:   "a lone RAW",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", rawPair)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.NEF")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC raw=2024/05-01 - Trip/IMG_0001.NEF"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG"},
		},
		{
			name:   "a lone Live video that the asset lacks",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC")},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG", "2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a still beside a RAW pair",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", rawPair)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG"},
		},
		{
			name:   "an MP4 is no Live video",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live, rawPair)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MP4")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG", "2024/05-01 - Trip/IMG_0001.MP4"},
		},
		{
			name:   "a lone file in another directory",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), lone("2024/05-02 - Trip/IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-02 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a lone file under another root",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), personal(lone(day + "IMG_0001.MOV"))},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"personal:2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a lone file of a numbered stem",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), lone(day + "IMG_0001 (1).MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001 (1).MOV"},
		},
		{
			name:   "a lone file of the stem in lower case",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "img_0001.mov")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/img_0001.mov"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG"},
		},
		{
			name:   "two lone Live videos",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.mov"), lone(day + "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/IMG_0001.MOV"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG", "2024/05-01 - Trip/IMG_0001.mov"},
		},
		{
			name:   "the first pair beside a lone file",
			assets: []domain.Asset{shot("b", "IMG_0001.JPG", live), shot("a", "IMG_0001.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MOV")},
			pairs: []string{
				"a exact 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/IMG_0001.MOV",
				"b exact 2024/05-01 - Trip/IMG_0001.JPG",
			},
		},
		{
			name:   "the first pair whose asset has the role",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC"), shot("b", "IMG_0001.JPG", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MOV")},
			pairs: []string{
				"a exact 2024/05-01 - Trip/IMG_0001.HEIC",
				"b exact 2024/05-01 - Trip/IMG_0001.JPG live=2024/05-01 - Trip/IMG_0001.MOV",
			},
		},
		{
			name:   "a pair by time",
			assets: []domain.Asset{shot("a", "IMG_002398.HEIC", live)},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0001.JPG"), lone(day + "IMG_0001.MOV")},
			pairs:  []string{"a same_time_size 2024/05-01 - Trip/IMG_0001.HEIC live=2024/05-01 - Trip/IMG_0001.MOV"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.JPG"},
		},
		{
			name:   "pairs by name and by time in the order of place",
			assets: []domain.Asset{shot("a", "IMG_0002.HEIC"), shot("b", "IMG_002398.HEIC")},
			files:  []match.Candidate{file("IMG_0002.HEIC"), file("IMG_0001.HEIC")},
			pairs: []string{
				"b same_time_size 2024/05-01 - Trip/IMG_0001.HEIC",
				"a exact 2024/05-01 - Trip/IMG_0002.HEIC",
			},
		},
		{
			name:   "the name before the time",
			assets: []domain.Asset{shot("a", "IMG_0002.HEIC")},
			files:  []match.Candidate{file("IMG_0001.HEIC"), file("IMG_0002.HEIC")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0002.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "left in the order of place",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC")},
			files:  []match.Candidate{withLive(file("IMG_0001.HEIC"), "IMG_0001.MOV"), file("IMG_0002.HEIC")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV", "2024/05-01 - Trip/IMG_0002.HEIC"},
		},
		{
			name:   "a video beside a Live Photo that lacks it",
			assets: []domain.Asset{shot("a", "IMG_0001.HEIC"), clip},
			files:  []match.Candidate{withLive(file("IMG_0001.HEIC"), "IMG_0001.MOV")},
			pairs:  []string{"a exact 2024/05-01 - Trip/IMG_0001.HEIC"},
			left:   []string{"v"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assets, files, before := slices.Clone(c.assets), slices.Clone(c.files), describe(c.files)
			pairs, left, alone := match.Match(assets, files)
			if !reflect.DeepEqual(assets, c.assets) || !slices.Equal(describe(files), before) {
				t.Errorf("%s: Match changed its input to %+q, %+q", c.name, assetUUIDs(assets), describe(files))
			}
			if got := matchPairs(pairs); !slices.Equal(got, c.pairs) {
				t.Errorf("%s: pairs = %+q, want %+q", c.name, got, c.pairs)
			}
			if got := assetUUIDs(left); !slices.Equal(got, c.left) {
				t.Errorf("%s: assets left = %+q, want %+q", c.name, got, c.left)
			}
			if got := describe(alone); !slices.Equal(got, c.alone) {
				t.Errorf("%s: candidates left = %+q, want %+q", c.name, got, c.alone)
			}
		})
	}
}

// checkMatch returns the first promise that the result of Match on assets and
// cands of the world w breaks: each asset once, in a pair or left; each file
// of cands once, in a pair or in a candidate left; a Live video or RAW in a
// pair only when the asset has a resource of its role, and only a file of the
// shot of Main.
func checkMatch(w gen.World, assets []domain.Asset, cands []match.Candidate, pairs []match.Pair, left []domain.Asset, alone []match.Candidate) error {
	keys, files := assetUUIDs(left), filePlaces(alone)
	for _, p := range pairs {
		keys, files = append(keys, p.Key.UUID), append(files, filePlaces([]match.Candidate{p.Candidate})...)
	}
	wantKeys, wantFiles := assetUUIDs(assets), filePlaces(cands)
	if len(keys) != len(wantKeys) || len(files) != len(wantFiles) {
		return fmt.Errorf("match.Match returns %d assets and %d files in pairs or left; want %d and %d", len(keys), len(files), len(wantKeys), len(wantFiles))
	}
	for _, s := range [][]string{keys, files, wantKeys, wantFiles} {
		slices.Sort(s)
	}
	if !slices.Equal(keys, wantKeys) || !slices.Equal(files, wantFiles) {
		return fmt.Errorf("match.Match returns assets %q and files %q; want each of %q and %q once", keys, files, wantKeys, wantFiles)
	}
	for _, p := range pairs {
		a := assets[slices.IndexFunc(assets, func(x domain.Asset) bool { return x.Key == p.Key })]
		for _, c := range []struct {
			file *domain.RemoteEntry
			role domain.ResourceRole
		}{{p.Candidate.Live, domain.ResourceLiveVideo}, {p.Candidate.RAW, domain.ResourceRAWPair}} {
			switch {
			case c.file == nil:
			case !slices.ContainsFunc(a.Resources, func(r domain.Resource) bool { return r.Role == c.role }):
				return fmt.Errorf("%q goes with asset %q, which has no %s resource", place(*c.file), a.Key.UUID, c.role)
			case otherShot(w, match.Pair{Key: w.Truth[p.Candidate.Main.Path], Candidate: match.Candidate{Main: *c.file}}):
				return fmt.Errorf("%q goes with %q of another shot", place(*c.file), place(p.Candidate.Main))
			}
		}
	}
	return nil
}

// covers: INV-9
func TestMatchProperties(t *testing.T) {
	t.Parallel()
	masks := []struct {
		name  string
		p     gen.Perturbation
		exact bool
	}{
		{"every perturbation but recompression and missing dates", gen.PerturbAll &^ (gen.PerturbRecompress | gen.PerturbNoDate), true},
		{"every perturbation", gen.PerturbAll, false},
	}
	rapid.Check(t, func(t *rapid.T) {
		mask := rapid.SampledFrom(masks).Draw(t, "perturb")
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(0, 30).Draw(t, "frames"), Perturb: mask.p}).Draw(t, "world")
		assets, bare := w.Snapshot.Assets, withoutComponents(w.Snapshot.Assets)
		cands, _ := match.Group(w.Files)
		lone := apart(cands)
		pairs, left, alone := match.Match(assets, cands)
		if err := checkMatch(w, assets, cands, pairs, left, alone); err != nil {
			t.Fatalf("%s: %v", mask.name, err)
		}
		pp, pl, pa := match.Match(rapid.Permutation(assets).Draw(t, "assets"), rapid.Permutation(cands).Draw(t, "candidates"))
		if !reflect.DeepEqual(pp, pairs) || !reflect.DeepEqual(pl, left) || !reflect.DeepEqual(pa, alone) {
			t.Fatalf("%s: permuted input gives %q, %q, %q; want %q, %q, %q", mask.name,
				matchPairs(pp), assetUUIDs(pl), describe(pa), matchPairs(pairs), assetUUIDs(left), describe(alone))
		}
		ap, al, aa := match.Match(assets, lone)
		if err := checkMatch(w, assets, lone, ap, al, aa); err != nil {
			t.Fatalf("%s, components apart: %v", mask.name, err)
		}
		if mask.exact && (!reflect.DeepEqual(ap, pairs) || !reflect.DeepEqual(al, left)) {
			t.Fatalf("%s: components apart give %q, %q; want %q, %q", mask.name,
				matchPairs(ap), assetUUIDs(al), matchPairs(pairs), assetUUIDs(left))
		}
		bp, bl, ba := match.Match(bare, cands)
		if err := checkMatch(w, bare, cands, bp, bl, ba); err != nil {
			t.Fatalf("%s, assets without components: %v", mask.name, err)
		}
		if !slices.Equal(pairStrings(bp), pairStrings(pairs)) || !slices.Equal(assetUUIDs(bl), assetUUIDs(left)) {
			t.Fatalf("%s: assets without components give %q, %q; want %q, %q", mask.name,
				pairStrings(bp), assetUUIDs(bl), pairStrings(pairs), assetUUIDs(left))
		}
	})
}

// Fixed seeds give the same counts on every run. A world has no lone Live
// videos or RAWs, so a run with those of Group apart makes them, and a run
// with assets that lack their roles sets them apart; each way must show.
func TestMatchOnWorlds(t *testing.T) {
	t.Parallel()
	components := func(ps []match.Pair) (n int) {
		for _, p := range ps {
			for _, e := range []*domain.RemoteEntry{p.Candidate.Live, p.Candidate.RAW} {
				if e != nil {
					n++
				}
			}
		}
		return n
	}
	var kept, joined, setApart int
	for seed := range uint64(100) {
		w := gen.New(seed, gen.Config{Frames: 30, Perturb: gen.PerturbAll})
		assets, bare := w.Snapshot.Assets, withoutComponents(w.Snapshot.Assets)
		cands, _ := match.Group(w.Files)
		lone := apart(cands)
		pairs, left, alone := match.Match(assets, cands)
		if err := checkMatch(w, assets, cands, pairs, left, alone); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		ap, al, aa := match.Match(assets, lone)
		if err := checkMatch(w, assets, lone, ap, al, aa); err != nil {
			t.Fatalf("seed %d, components apart: %v", seed, err)
		}
		bp, bl, ba := match.Match(bare, cands)
		if err := checkMatch(w, bare, cands, bp, bl, ba); err != nil {
			t.Fatalf("seed %d, assets without components: %v", seed, err)
		}
		kept, joined, setApart = kept+components(pairs), joined+components(ap), setApart+len(ba)-len(alone)
	}
	t.Logf("Live videos and RAWs in pairs in worlds of seeds 0 to 99: %d as Group gives them, %d from candidates of their own; %d set apart from assets that lack their roles",
		kept, joined, setApart)
	if kept == 0 || joined == 0 || setApart != kept {
		t.Errorf("%d kept, %d joined and %d set apart; want the first two above zero and every kept one set apart", kept, joined, setApart)
	}
}
