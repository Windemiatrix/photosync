package match_test

import (
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

func byNameAsset(uuid, name string, size int64, taken time.Time, offset *int) domain.Asset {
	return domain.Asset{
		Key: domain.AssetKey{UUID: uuid}, OriginalName: name, Taken: taken, TZOffset: offset,
		Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
	}
}

func byNameFile(p string, size int64, taken time.Time, offset *int) match.Candidate {
	return match.Candidate{
		Main: domain.RemoteEntry{
			Root: domain.LibraryShared, Path: p, Size: size,
			Meta: domain.MediaMeta{Taken: taken, TZOffset: offset},
		},
		Role: domain.RoleOriginal,
	}
}

func pairStrings(ps []match.Pair) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.Key.UUID+" "+place(p.Candidate.Main)+" "+string(p.Level))
	}
	return out
}

func assetUUIDs(as []domain.Asset) []string {
	var out []string
	for _, a := range as {
		out = append(out, a.Key.UUID)
	}
	return out
}

func mainPlaces(cs []match.Candidate) []string {
	var out []string
	for _, c := range cs {
		out = append(out, place(c.Main))
	}
	return out
}

// Written from what the levels mean, not from the code. A shot is taken at
// 10:00 on 1 May 2024 at +03:00 and lies in the folder of that day unless a
// row says otherwise; its original has 100 bytes.
func TestByName(t *testing.T) {
	t.Parallel()
	if pairs, left, alone := match.ByName(nil, nil); pairs != nil || left != nil || alone != nil {
		t.Errorf("ByName(nil, nil) = %#v, %#v, %#v; want nil, nil, nil", pairs, left, alone)
	}
	nfc := "Ёлка.HEIC"
	nfd := norm.NFD.String(nfc)
	if nfd == nfc {
		t.Fatalf("%+q has no composed letter, so the NFD row tests nothing", nfc)
	}

	plus3 := 3 * 3600
	at := time.Date(2024, 5, 1, 7, 0, 0, 0, time.UTC)
	wall := at.Add(3 * time.Hour)
	in2012, in2013, in2022 := at.AddDate(-12, 0, 0), at.AddDate(-11, 0, 0), at.AddDate(-2, 0, 0)
	var none time.Time
	const day = "2024/05-01 - Trip/"
	shot := byNameAsset("a", "IMG_0001.HEIC", 100, at, &plus3)
	old := byNameAsset("a", "IMG_0279.JPG", 100, in2013, &plus3)
	video := func(a domain.Asset) domain.Asset {
		a.IsVideo = true
		return a
	}
	source := func(c match.Candidate) match.Candidate {
		c.Role = domain.RoleSource
		return c
	}
	withResources := func(rs ...domain.Resource) domain.Asset {
		a := shot
		a.Resources = rs
		return a
	}
	cases := []struct {
		name               string
		assets             []domain.Asset
		files              []match.Candidate
		pairs, left, alone []string
	}{
		{
			name:   "the same file",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC exact"},
		},
		{
			name:   "the name in another case",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"img_0001.heic", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/img_0001.heic exact"},
		},
		{
			name:   "the name in NFD",
			assets: []domain.Asset{byNameAsset("a", nfc, 100, at, &plus3)},
			files:  []match.Candidate{byNameFile(day+nfd, 100, at, &plus3)},
			pairs:  []string{"a " + day + nfd + " exact"},
		},
		{
			name:   "another size",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
		},
		{
			name:   "a numbered file is never exact",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001 (1).HEIC same_shot"},
		},
		{
			name:   "a numbered original",
			assets: []domain.Asset{byNameAsset("a", "IMG_0001 (1).HEIC", 100, at, &plus3)},
			files:  []match.Candidate{byNameFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001 (1).HEIC exact"},
		},
		{
			name:   "another extension",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.JPG", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.JPG same_shot"},
		},
		{
			name: "the size of another resource",
			assets: []domain.Asset{withResources(
				domain.Resource{Role: domain.ResourceLiveVideo, Filename: "IMG_0001.MOV", Size: 200},
				domain.Resource{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100},
			)},
			files: []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 200, at, &plus3)},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
		},
		{
			name:   "an asset without resources",
			assets: []domain.Asset{withResources()},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 100, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
		},
		{
			name:   "an hour off, both with a time zone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, at.Add(time.Hour), &plus3)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a file without a time zone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, wall, nil)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
		},
		{
			name:   "an hour off, neither with a time zone",
			assets: []domain.Asset{byNameAsset("a", "IMG_0001.HEIC", 100, wall, nil)},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, wall.Add(time.Hour), nil)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "an asset without a time zone",
			assets: []domain.Asset{byNameAsset("a", "IMG_0001.HEIC", 100, wall, nil)},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
		},
		{
			name:   "the same name in another year",
			assets: []domain.Asset{old},
			files:  []match.Candidate{byNameFile("2022/05-01 - Trip/IMG_0279.JPG", 90, in2022, &plus3)},
			left:   []string{"a"},
			alone:  []string{"2022/05-01 - Trip/IMG_0279.JPG"},
		},
		{
			name:   "a date edited in Photos",
			assets: []domain.Asset{old},
			files:  []match.Candidate{byNameFile("2022/05-01 - Trip/IMG_0279.JPG", 100, in2022, &plus3)},
			pairs:  []string{"a 2022/05-01 - Trip/IMG_0279.JPG exact"},
		},
		{
			name: "two shots of one name and size",
			assets: []domain.Asset{
				byNameAsset("a", "IMG_0279.JPG", 100, in2022, &plus3),
				byNameAsset("b", "IMG_0279.JPG", 100, in2013, &plus3),
			},
			files: []match.Candidate{
				byNameFile("2013/05-01 - Trip/IMG_0279.JPG", 100, in2013, &plus3),
				byNameFile("2022/05-01 - Trip/IMG_0279.JPG", 100, in2022, &plus3),
			},
			pairs: []string{"b 2013/05-01 - Trip/IMG_0279.JPG exact", "a 2022/05-01 - Trip/IMG_0279.JPG exact"},
		},
		{
			name:   "the file of the shot before one of another year",
			assets: []domain.Asset{old},
			files: []match.Candidate{
				byNameFile("2012/05-01 - Trip/IMG_0279.JPG", 100, in2012, &plus3),
				byNameFile("2013/05-01 - Trip/IMG_0279.JPG", 90, in2013, &plus3),
			},
			pairs: []string{"a 2013/05-01 - Trip/IMG_0279.JPG same_shot"},
			alone: []string{"2012/05-01 - Trip/IMG_0279.JPG"},
		},
		{
			name:   "an undated file of the shot before one of another year",
			assets: []domain.Asset{old},
			files: []match.Candidate{
				byNameFile("2012/05-01 - Trip/IMG_0279.JPG", 100, in2012, &plus3),
				byNameFile("2013/05-01 - Trip/IMG_0279.JPG", 90, none, nil),
			},
			pairs: []string{"a 2013/05-01 - Trip/IMG_0279.JPG same_shot_weak"},
			alone: []string{"2012/05-01 - Trip/IMG_0279.JPG"},
		},
		{
			name:   "a name and a size without any date",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byNameFile(day+"IMG_0001 (1).HEIC", 99, at, &plus3),
				byNameFile("IMG_0001.HEIC", 100, none, nil),
			},
			pairs: []string{"a IMG_0001.HEIC exact"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "a dated folder",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 99, none, nil)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot_weak"},
		},
		{
			name:   "a capture time before a folder",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byNameFile(day+"IMG_0001 (1).HEIC", 99, none, nil),
				byNameFile(day+"IMG_0001.HEIC", 99, at, &plus3),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_shot"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "a folder of another month",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile("2024/06-01 - Trip/IMG_0001.HEIC", 99, none, nil)},
			left:   []string{"a"},
			alone:  []string{"2024/06-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a folder of another year",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile("2023/05-01 - Trip/IMG_0001.HEIC", 99, none, nil)},
			left:   []string{"a"},
			alone:  []string{"2023/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "the month at the place of the shot",
			assets: []domain.Asset{byNameAsset("a", "IMG_0001.HEIC", 100, time.Date(2024, 5, 31, 22, 0, 0, 0, time.UTC), &plus3)},
			files:  []match.Candidate{byNameFile("2024/06-01 - Trip/IMG_0001.HEIC", 99, none, nil)},
			pairs:  []string{"a 2024/06-01 - Trip/IMG_0001.HEIC same_shot_weak"},
		},
		{
			name:   "a name alone never pairs",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byNameFile("2024/IMG_0001.HEIC", 99, none, nil),
				byNameFile("IMG_0001.HEIC", 99, none, nil),
			},
			left:  []string{"a"},
			alone: []string{"2024/IMG_0001.HEIC", "IMG_0001.HEIC"},
		},
		{
			name:   "a lone Live Photo video",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.MOV", 99, at, &plus3)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a lone Live Photo video without a date",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byNameFile(day+"IMG_0001.MOV", 99, none, nil)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a video",
			assets: []domain.Asset{video(byNameAsset("a", "IMG_0002.MOV", 1000, at, &plus3))},
			files:  []match.Candidate{byNameFile(day+"IMG_0002.mp4", 900, at, &plus3)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0002.mp4 same_shot"},
		},
		{
			name:   "the source of a converted video",
			assets: []domain.Asset{video(byNameAsset("a", "MVI_0001.MP4", 1000, at, &plus3))},
			files:  []match.Candidate{source(byNameFile(day+"MVI_0001.AVI", 900, at, &plus3))},
			pairs:  []string{"a 2024/05-01 - Trip/MVI_0001.AVI same_shot"},
		},
		{
			name:   "a photo of the name of a video",
			assets: []domain.Asset{video(byNameAsset("a", "IMG_0002.MOV", 1000, at, &plus3))},
			files:  []match.Candidate{byNameFile(day+"IMG_0002.JPG", 900, at, &plus3)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0002.JPG"},
		},
		{
			name: "a level before an asset",
			assets: []domain.Asset{
				byNameAsset("a", "IMG_0001.JPG", 100, at, &plus3),
				byNameAsset("b", "IMG_0001.HEIC", 100, at, &plus3),
			},
			files: []match.Candidate{byNameFile(day+"IMG_0001.HEIC", 100, at, &plus3)},
			pairs: []string{"b 2024/05-01 - Trip/IMG_0001.HEIC exact"},
			left:  []string{"a"},
		},
		{
			name:   "an asset twice in Photos",
			assets: []domain.Asset{byNameAsset("a-1", "IMG_0001.HEIC", 100, at, &plus3), shot},
			files: []match.Candidate{
				byNameFile(day+"IMG_0001.HEIC", 100, at, &plus3),
				byNameFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3),
			},
			pairs: []string{"a-1 2024/05-01 - Trip/IMG_0001 (1).HEIC same_shot", "a 2024/05-01 - Trip/IMG_0001.HEIC exact"},
		},
		{
			name: "results in order",
			assets: []domain.Asset{
				byNameAsset("d", "IMG_0006.HEIC", 100, at, &plus3),
				byNameAsset("b", "IMG_0001.HEIC", 100, at, &plus3),
				byNameAsset("c", "IMG_0003.HEIC", 100, at, &plus3),
				byNameAsset("a", "IMG_0002.HEIC", 100, at, &plus3),
			},
			files: []match.Candidate{
				byNameFile(day+"IMG_0005.HEIC", 100, at, &plus3),
				byNameFile(day+"IMG_0002.HEIC", 100, at, &plus3),
				byNameFile(day+"IMG_0004.HEIC", 100, at, &plus3),
				byNameFile(day+"IMG_0001.HEIC", 100, at, &plus3),
			},
			pairs: []string{"b 2024/05-01 - Trip/IMG_0001.HEIC exact", "a 2024/05-01 - Trip/IMG_0002.HEIC exact"},
			left:  []string{"c", "d"},
			alone: []string{"2024/05-01 - Trip/IMG_0004.HEIC", "2024/05-01 - Trip/IMG_0005.HEIC"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assets, files := slices.Clone(c.assets), slices.Clone(c.files)
			pairs, left, alone := match.ByName(assets, files)
			if !reflect.DeepEqual(assets, c.assets) || !reflect.DeepEqual(files, c.files) {
				t.Errorf("%s: ByName changed its input to %+q, %+q", c.name, assetUUIDs(assets), mainPlaces(files))
			}
			if got := pairStrings(pairs); !slices.Equal(got, c.pairs) {
				t.Errorf("%s: pairs = %+q, want %+q", c.name, got, c.pairs)
			}
			if got := assetUUIDs(left); !slices.Equal(got, c.left) {
				t.Errorf("%s: assets left = %+q, want %+q", c.name, got, c.left)
			}
			if got := mainPlaces(alone); !slices.Equal(got, c.alone) {
				t.Errorf("%s: candidates left = %+q, want %+q", c.name, got, c.alone)
			}
		})
	}
}

// checkByName returns the first promise that the result of ByName on the
// assets of w and on cands breaks: each asset and each candidate once, in a
// pair or left, and no pair across shots by the truth of w. Photos may hold
// one shot twice, as two assets equal but for their keys; a file of either
// is a file of that shot, so the oracle compares shots, not keys.
func checkByName(w gen.World, cands []match.Candidate, pairs []match.Pair, left []domain.Asset, alone []match.Candidate) error {
	assets := w.Snapshot.Assets
	if len(pairs)+len(left) != len(assets) || len(pairs)+len(alone) != len(cands) {
		return fmt.Errorf("ByName returns %d pairs, %d assets left and %d candidates left for %d assets and %d candidates",
			len(pairs), len(left), len(alone), len(assets), len(cands))
	}
	keys, mains := assetUUIDs(left), mainPlaces(alone)
	for _, p := range pairs {
		keys, mains = append(keys, p.Key.UUID), append(mains, place(p.Candidate.Main))
	}
	wantKeys, wantMains := assetUUIDs(assets), mainPlaces(cands)
	for _, s := range [][]string{keys, mains, wantKeys, wantMains} {
		slices.Sort(s)
	}
	if !slices.Equal(keys, wantKeys) || !slices.Equal(mains, wantMains) {
		return fmt.Errorf("ByName returns assets %q and candidates %q; want each of %q and %q once", keys, mains, wantKeys, wantMains)
	}
	assetOf := map[domain.AssetKey]domain.Asset{}
	for _, a := range assets {
		assetOf[a.Key] = a
	}
	for _, p := range pairs {
		owner := assetOf[w.Truth[p.Candidate.Main.Path]]
		owner.Key = p.Key
		if !reflect.DeepEqual(owner, assetOf[p.Key]) {
			return fmt.Errorf("%s pairs %q with asset %q of another shot", p.Level, place(p.Candidate.Main), p.Key.UUID)
		}
	}
	return nil
}

// covers: INV-9
func TestByNameProperties(t *testing.T) {
	t.Parallel()
	masks := []struct {
		name string
		p    gen.Perturbation
	}{
		{"no perturbation", 0},
		{"every perturbation but import rename", gen.PerturbAll &^ gen.PerturbImportRename},
		{"every perturbation", gen.PerturbAll},
	}
	rapid.Check(t, func(t *rapid.T) {
		mask := rapid.SampledFrom(masks).Draw(t, "perturb")
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(0, 30).Draw(t, "frames"), Perturb: mask.p}).Draw(t, "world")
		cands, _ := match.Group(w.Files)
		pairs, left, alone := match.ByName(w.Snapshot.Assets, cands)
		if err := checkByName(w, cands, pairs, left, alone); err != nil {
			t.Fatalf("%s: %v", mask.name, err)
		}
		if mask.p&gen.PerturbImportRename == 0 && (left != nil || alone != nil) {
			t.Fatalf("%s: assets %q and candidates %q are left; want none", mask.name, assetUUIDs(left), mainPlaces(alone))
		}
		for _, p := range pairs {
			if mask.p == 0 && p.Level != domain.LevelExact {
				t.Fatalf("%s: %q pairs at %s; want exact", mask.name, place(p.Candidate.Main), p.Level)
			}
		}
		permuted := rapid.Permutation(w.Snapshot.Assets).Draw(t, "assets")
		pp, pl, pa := match.ByName(permuted, rapid.Permutation(cands).Draw(t, "candidates"))
		if !reflect.DeepEqual(pp, pairs) || !reflect.DeepEqual(pl, left) || !reflect.DeepEqual(pa, alone) {
			t.Fatalf("%s: permuted input gives %q, %q, %q; want %q, %q, %q", mask.name,
				pairStrings(pp), assetUUIDs(pl), mainPlaces(pa), pairStrings(pairs), assetUUIDs(left), mainPlaces(alone))
		}
	})
}

// Fixed seeds give the same counts on every run. Without renames at import
// every asset keeps the file of its shot, so nothing is left, and each level
// shows.
func TestByNameOnWorlds(t *testing.T) {
	t.Parallel()
	levels := map[domain.MatchLevel]int{}
	for seed := range uint64(100) {
		for _, p := range []gen.Perturbation{gen.PerturbAll &^ gen.PerturbImportRename, gen.PerturbAll} {
			w := gen.New(seed, gen.Config{Frames: 30, Perturb: p})
			cands, _ := match.Group(w.Files)
			pairs, left, alone := match.ByName(w.Snapshot.Assets, cands)
			if err := checkByName(w, cands, pairs, left, alone); err != nil {
				t.Fatalf("seed %d, renames %v: %v", seed, p == gen.PerturbAll, err)
			}
			if p == gen.PerturbAll {
				continue
			}
			if left != nil || alone != nil {
				t.Fatalf("seed %d without renames: assets %q and candidates %q are left; want none",
					seed, assetUUIDs(left), mainPlaces(alone))
			}
			for _, pair := range pairs {
				levels[pair.Level]++
			}
		}
	}
	t.Logf("pairs by level in worlds without renames at import: exact %d, same_shot %d, same_shot_weak %d",
		levels[domain.LevelExact], levels[domain.LevelSameShot], levels[domain.LevelSameShotWeak])
	for _, l := range []domain.MatchLevel{domain.LevelExact, domain.LevelSameShot, domain.LevelSameShotWeak} {
		if levels[l] == 0 {
			t.Errorf("no world of seeds 0 to 99 without renames pairs at %s", l)
		}
	}
}
