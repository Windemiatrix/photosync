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

func byTimeAsset(uuid, name string, size int64, taken time.Time, offset *int, subsec string) domain.Asset {
	return domain.Asset{
		Key: domain.AssetKey{UUID: uuid}, OriginalName: name, Taken: taken, TZOffset: offset, SubSec: subsec,
		Width: 4032, Height: 3024,
		Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
	}
}

func byTimeFile(p string, size int64, taken time.Time, offset *int, subsec string) match.Candidate {
	return match.Candidate{
		Main: domain.RemoteEntry{
			Root: domain.LibraryShared, Path: p, Size: size,
			Meta: domain.MediaMeta{Taken: taken, TZOffset: offset, SubSec: subsec, Width: 4032, Height: 3024},
		},
		Role: domain.RoleOriginal,
	}
}

// Written from what the levels mean, not from the code. A shot is taken at
// 10:00:00.5 on 1 May 2024 at +03:00; Photos renamed it IMG_002398.HEIC and
// keeps 100 bytes and 4032 by 3024 pixels of its original. Its file is
// IMG_0001.HEIC of the same frame in the folder of that day unless a row
// says otherwise.
func TestByTime(t *testing.T) {
	t.Parallel()
	if pairs, left, alone := match.ByTime(nil, nil); pairs != nil || left != nil || alone != nil {
		t.Errorf("ByTime(nil, nil) = %#v, %#v, %#v; want nil, nil, nil", pairs, left, alone)
	}
	plus3, plus4 := 3*3600, 4*3600
	at := time.Date(2024, 5, 1, 7, 0, 0, 0, time.UTC)
	wall := at.Add(3 * time.Hour)
	var none time.Time
	const day = "2024/05-01 - Trip/"
	shot := byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "500")
	blind := shot
	blind.Width, blind.Height = 0, 0
	live := shot
	live.Resources = []domain.Resource{
		{Role: domain.ResourceLiveVideo, Filename: "IMG_002398.MOV", Size: 100},
		{Role: domain.ResourceOriginal, Filename: "IMG_002398.HEIC", Size: 90},
	}
	video := func(a domain.Asset) domain.Asset {
		a.IsVideo = true
		return a
	}
	framed := func(c match.Candidate, width, height int) match.Candidate {
		c.Main.Meta.Width, c.Main.Meta.Height = width, height
		return c
	}
	cases := []struct {
		name               string
		assets             []domain.Asset
		files              []match.Candidate
		pairs, left, alone []string
	}{
		{
			name:   "the same moment and size",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "another size",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC probable"},
		},
		{
			name:   "another size and frame",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 1920, 1080)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "another size, the frame turned",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 3024, 4032)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC probable"},
		},
		{
			name:   "another size and height",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 4032, 2268)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "another size, the width turned to the height alone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 3024, 1000)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "another size, the height turned to the width alone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 1000, 4032)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "another size and width",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 1000, 3024)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "the same size and another frame",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500"), 1920, 1080)},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "no frame on either side",
			assets: []domain.Asset{blind},
			files:  []match.Candidate{framed(byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "500"), 0, 0)},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "the size of another resource",
			assets: []domain.Asset{live},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC probable"},
		},
		{
			name:   "a video for a photo",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.MOV", 100, at, &plus3, "500")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.MOV"},
		},
		{
			name:   "a photo for a video",
			assets: []domain.Asset{video(byTimeAsset("a", "IMG_002399.MOV", 100, at, &plus3, ""))},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a video",
			assets: []domain.Asset{video(byTimeAsset("a", "IMG_002399.MOV", 1000, at, &plus3, ""))},
			files:  []match.Candidate{byTimeFile(day+"IMG_0002.mp4", 900, at, &plus3, "")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0002.mp4 probable"},
		},
		{
			name:   "the source of a converted video",
			assets: []domain.Asset{video(byTimeAsset("a", "MVI_000123.MP4", 1000, at, &plus3, ""))},
			files:  []match.Candidate{byTimeFile(day+"MVI_0001.AVI", 1000, at, &plus3, "")},
			pairs:  []string{"a 2024/05-01 - Trip/MVI_0001.AVI same_time_size"},
		},
		{
			name:   "a second later without a fraction",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(time.Second), &plus3, "")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a second earlier without a fraction",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(-time.Second), &plus3, "")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "two seconds later without a fraction",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(2*time.Second), &plus3, "")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a second later with a fraction",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(time.Second), &plus3, "500")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "another fraction",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "600")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a fraction written longer",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "75")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "750")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a fraction with a leading zero",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "075")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "75")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a fraction of nine digits",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "123456789")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "123456788")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a fraction of ten digits",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "1000000000")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "5")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a fraction that is not digits",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "-5")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "5")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a file without a time zone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, wall, nil, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a file without a time zone, an hour off",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, wall.Add(time.Hour), nil, "500")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "neither with a time zone",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, wall, nil, "500")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, wall, nil, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "an asset without a time zone",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, wall, nil, "500")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "the same moment in another time zone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus4, "500")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "the same wall clock in another time zone",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(-time.Hour), &plus4, "500")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "a capture time that keeps its fraction",
			assets: []domain.Asset{byTimeAsset("a", "IMG_002398.HEIC", 100, at.Add(500*time.Millisecond), &plus3, "500")},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500")},
			pairs:  []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name:   "a file without a capture time",
			assets: []domain.Asset{shot},
			files:  []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, none, nil, "")},
			left:   []string{"a"},
			alone:  []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name:   "the size before the time",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).HEIC", 90, at, &plus3, "500"),
				byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(time.Second), &plus3, ""),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "the size before the fraction and the extension",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).HEIC", 90, at, &plus3, "500"),
				byTimeFile(day+"IMG_0001.JPG", 100, at.Add(time.Second), &plus3, ""),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.JPG same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "an equal fraction before an equal second",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3, ""),
				byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500"),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "the same second before a second apart",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).HEIC", 100, at.Add(time.Second), &plus3, ""),
				byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, ""),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "the time before the extension",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3, ""),
				byTimeFile(day+"IMG_0001.JPG", 100, at, &plus3, "500"),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001.JPG same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).HEIC"},
		},
		{
			name:   "the extension in any case",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001 (1).JPG", 100, at, &plus3, "500"),
				byTimeFile(day+"img_0001.heic", 100, at, &plus3, "500"),
			},
			pairs: []string{"a 2024/05-01 - Trip/img_0001.heic same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001 (1).JPG"},
		},
		{
			name:   "two files a second off either way",
			assets: []domain.Asset{shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(-time.Second), &plus3, ""),
				byTimeFile(day+"IMG_0001 (1).HEIC", 100, at.Add(time.Second), &plus3, ""),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001 (1).HEIC same_time_size"},
			alone: []string{"2024/05-01 - Trip/IMG_0001.HEIC"},
		},
		{
			name: "a level before an asset",
			assets: []domain.Asset{
				byTimeAsset("a", "IMG_002398.HEIC", 90, at, &plus3, "500"),
				byTimeAsset("b", "IMG_002399.HEIC", 100, at, &plus3, "500"),
			},
			files: []match.Candidate{byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500")},
			pairs: []string{"b 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
			left:  []string{"a"},
		},
		{
			name: "a burst",
			assets: []domain.Asset{
				byTimeAsset("b", "IMG_002399.HEIC", 100, at, &plus3, "200"),
				byTimeAsset("a", "IMG_002398.HEIC", 100, at, &plus3, "100"),
			},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0002.HEIC", 90, at, &plus3, "100"),
				byTimeFile(day+"IMG_0001.HEIC", 90, at, &plus3, "200"),
			},
			pairs: []string{"b 2024/05-01 - Trip/IMG_0001.HEIC probable", "a 2024/05-01 - Trip/IMG_0002.HEIC probable"},
		},
		{
			name:   "an asset twice in Photos",
			assets: []domain.Asset{byTimeAsset("a-1", "IMG_002398.HEIC", 100, at, &plus3, "500"), shot},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0001.HEIC", 100, at, &plus3, "500"),
				byTimeFile(day+"IMG_0001 (1).HEIC", 100, at, &plus3, "500"),
			},
			pairs: []string{"a 2024/05-01 - Trip/IMG_0001 (1).HEIC same_time_size", "a-1 2024/05-01 - Trip/IMG_0001.HEIC same_time_size"},
		},
		{
			name: "left in order",
			assets: []domain.Asset{
				byTimeAsset("b", "IMG_002399.HEIC", 100, at.Add(10*time.Second), &plus3, "500"),
				byTimeAsset("a", "IMG_002398.HEIC", 100, at.Add(20*time.Second), &plus3, "500"),
			},
			files: []match.Candidate{
				byTimeFile(day+"IMG_0002.HEIC", 100, at.Add(-10*time.Second), &plus3, "500"),
				byTimeFile(day+"IMG_0001.HEIC", 100, at.Add(-20*time.Second), &plus3, "500"),
			},
			left:  []string{"a", "b"},
			alone: []string{"2024/05-01 - Trip/IMG_0001.HEIC", "2024/05-01 - Trip/IMG_0002.HEIC"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assets, files := slices.Clone(c.assets), slices.Clone(c.files)
			pairs, left, alone := match.ByTime(assets, files)
			if !reflect.DeepEqual(assets, c.assets) || !reflect.DeepEqual(files, c.files) {
				t.Errorf("%s: ByTime changed its input to %+q, %+q", c.name, assetUUIDs(assets), mainPlaces(files))
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

// checkByTime returns the first promise that the result of ByTime on assets
// and cands of the world w breaks: each asset and each candidate once, in a
// pair or left, and a level of time on each pair; when complete, also no
// asset left beside a file of its shot that has a capture time.
func checkByTime(w gen.World, assets []domain.Asset, cands []match.Candidate, pairs []match.Pair, left []domain.Asset, alone []match.Candidate, complete bool) error {
	if len(pairs)+len(left) != len(assets) || len(pairs)+len(alone) != len(cands) {
		return fmt.Errorf("ByTime returns %d pairs, %d assets left and %d candidates left for %d assets and %d candidates",
			len(pairs), len(left), len(alone), len(assets), len(cands))
	}
	keys, mains := assetUUIDs(left), mainPlaces(alone)
	for _, p := range pairs {
		if p.Level != domain.LevelSameTimeSize && p.Level != domain.LevelProbable {
			return fmt.Errorf("%q pairs at %q; want same_time_size or probable", place(p.Candidate.Main), p.Level)
		}
		keys, mains = append(keys, p.Key.UUID), append(mains, place(p.Candidate.Main))
	}
	wantKeys, wantMains := assetUUIDs(assets), mainPlaces(cands)
	for _, s := range [][]string{keys, mains, wantKeys, wantMains} {
		slices.Sort(s)
	}
	if !slices.Equal(keys, wantKeys) || !slices.Equal(mains, wantMains) {
		return fmt.Errorf("ByTime returns assets %q and candidates %q; want each of %q and %q once", keys, mains, wantKeys, wantMains)
	}
	for _, a := range left {
		for _, c := range alone {
			if complete && !c.Main.Meta.Taken.IsZero() && !otherShot(w, match.Pair{Key: a.Key, Candidate: c}) {
				return fmt.Errorf("asset %q and %q, a file of its shot with a capture time, are both left", a.Key.UUID, place(c.Main))
			}
		}
	}
	return nil
}

// otherShot reports whether p pairs a file with an asset of another shot by
// the truth of w. Photos may hold one shot twice, as two assets equal but for
// their keys; a file of either is a file of that shot.
func otherShot(w gen.World, p match.Pair) bool {
	as := w.Snapshot.Assets
	owner := as[slices.IndexFunc(as, func(a domain.Asset) bool { return a.Key == w.Truth[p.Candidate.Main.Path] })]
	paired := as[slices.IndexFunc(as, func(a domain.Asset) bool { return a.Key == p.Key })]
	owner.Key = paired.Key
	return !reflect.DeepEqual(owner, paired)
}

// covers: INV-9
func TestByTimeProperties(t *testing.T) {
	t.Parallel()
	masks := []struct {
		name  string
		p     gen.Perturbation
		exact bool
	}{
		{"every perturbation but recompression and rounding", gen.PerturbAll &^ (gen.PerturbRecompress | gen.PerturbRoundSubSec), true},
		{"every perturbation", gen.PerturbAll, false},
	}
	rapid.Check(t, func(t *rapid.T) {
		mask := rapid.SampledFrom(masks).Draw(t, "perturb")
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(0, 30).Draw(t, "frames"), Perturb: mask.p}).Draw(t, "world")
		cands, _ := match.Group(w.Files)
		_, assets, files := match.ByName(w.Snapshot.Assets, cands)
		pairs, left, alone := match.ByTime(assets, files)
		if err := checkByTime(w, assets, files, pairs, left, alone, mask.exact); err != nil {
			t.Fatalf("%s: %v", mask.name, err)
		}
		for _, p := range pairs {
			if mask.exact && otherShot(w, p) {
				t.Fatalf("%s: %s pairs %q with asset %q of another shot", mask.name, p.Level, place(p.Candidate.Main), p.Key.UUID)
			}
		}
		pp, pl, pa := match.ByTime(rapid.Permutation(assets).Draw(t, "assets"), rapid.Permutation(files).Draw(t, "files"))
		if !reflect.DeepEqual(pp, pairs) || !reflect.DeepEqual(pl, left) || !reflect.DeepEqual(pa, alone) {
			t.Fatalf("%s: permuted input gives %q, %q, %q; want %q, %q, %q", mask.name,
				pairStrings(pp), assetUUIDs(pl), mainPlaces(pa), pairStrings(pairs), assetUUIDs(left), mainPlaces(alone))
		}
		var noise []match.Candidate
		for _, c := range files {
			c.Main.Path, c.Main.Meta.Taken = "0/"+c.Main.Path, c.Main.Meta.Taken.AddDate(100, 0, 0)
			noise = append(noise, c)
		}
		np, nl, na := match.ByTime(assets, slices.Concat(files, noise))
		if want := slices.Concat(noise, alone); !reflect.DeepEqual(np, pairs) || !reflect.DeepEqual(nl, left) || !reflect.DeepEqual(na, want) {
			t.Fatalf("%s: files a century later give %q, %q, %q; want %q, %q, %q", mask.name,
				pairStrings(np), assetUUIDs(nl), mainPlaces(na), pairStrings(pairs), assetUUIDs(left), mainPlaces(want))
		}
	})
}

// Fixed seeds give the same counts on every run. ByTime takes what ByName
// leaves, as a plan does; a pair across shots counts against the ceiling of
// its level, and each level must show.
func TestByTimeOnWorlds(t *testing.T) {
	t.Parallel()
	ceilings := []struct {
		level   domain.MatchLevel
		crossed int
	}{
		{domain.LevelSameTimeSize, 0},
		{domain.LevelProbable, 0},
	}
	found, crossed := map[domain.MatchLevel]int{}, map[domain.MatchLevel]int{}
	for seed := range uint64(100) {
		w := gen.New(seed, gen.Config{Frames: 30, Perturb: gen.PerturbAll})
		cands, _ := match.Group(w.Files)
		_, assets, files := match.ByName(w.Snapshot.Assets, cands)
		pairs, left, alone := match.ByTime(assets, files)
		if err := checkByTime(w, assets, files, pairs, left, alone, true); err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		for _, p := range pairs {
			found[p.Level]++
			if otherShot(w, p) {
				crossed[p.Level]++
			}
		}
	}
	t.Logf("pairs by level in worlds of seeds 0 to 99, across shots in parentheses: same_time_size %d (%d), probable %d (%d)",
		found[domain.LevelSameTimeSize], crossed[domain.LevelSameTimeSize], found[domain.LevelProbable], crossed[domain.LevelProbable])
	for _, c := range ceilings {
		if found[c.level] == 0 {
			t.Errorf("no world of seeds 0 to 99 pairs at %s", c.level)
		}
		if crossed[c.level] > c.crossed {
			t.Errorf("%d pairs at %s cross shots; want at most %d", crossed[c.level], c.level, c.crossed)
		}
	}
}
