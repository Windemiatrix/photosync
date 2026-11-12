package match_test

import (
	"maps"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/testkit/fixtures"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

// reason is why matching links a file of an anonymized case otherwise than
// the reference manual sync did; each is a rule that matching keeps on
// purpose.
type reason string

// The reasons that the anonymized cases show.
const (
	oneFilePerAsset reason = "an asset holds one file; another copy of its shot is for the plan to find by content (R-LAYOUT-08)"
)

// divergence is a file of a case that matching links otherwise than the
// reference manual sync did; ref and here are its links as linkString writes
// them, empty for no link.
type divergence struct {
	path, ref, here string
	why             reason
}

// divergences lists, by case, every file that matching links otherwise than
// the reference manual sync did, foretold from the rules of both and the
// records of the case before any run.
var divergences = map[string][]divergence{
	"anon-cloud-duplicate": {
		{"Тэжге/IMG_20521231_215113.jpg", "00000000-F53A-8472-BE90-3B6F5B8D515C exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521231_215435.jpg", "00000000-D042-8852-9467-34B8D71A70DC exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521231_215450.jpg", "00000000-7902-8ADA-8BE9-EAC7945DDA9E exact", "", oneFilePerAsset},
	},
	"anon-level-same-shot-weak": {
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A0613.jpg", "00000000-44B2-8252-9523-165DA042E751 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A0748.jpg", "00000000-50EF-8D42-8F72-10A13559CEDB same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A0874.jpg", "00000000-2CAB-8DDA-9334-B1805763794B same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A2855.jpg", "00000000-F66B-86C7-9912-8EB07C02C8FE same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A4352.jpg", "00000000-E7AF-8043-90C8-720F5673C527 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A7384.jpg", "00000000-98F4-8ACB-817A-5BCA7DD86375 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A8078.jpg", "00000000-999B-8D8F-8D74-3922B4140ED0 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A9739.jpg", "00000000-14FE-8679-A25A-A63140C0C1A6 same_shot_weak keep_cloud", "", oneFilePerAsset},
	},
	"anon-many-folders": {
		{"2052/07-13 - Цчнпювы гы ввбафрх/Фшнцрщае/IMG_9623.jpg", "00000000-2CE7-822B-BD77-8AAE92DAAF0C same_shot replace", "", oneFilePerAsset},
		{"Тэжге/IMG_20521004_155143.jpg", "00000000-BBD5-8BF3-96CD-6D3CCA657C78 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521004_155541.jpg", "00000000-2A9C-84ED-B257-BC6C23887271 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521004_155558.jpg", "00000000-CEE4-807F-AF05-2B75C5AEEB57 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521004_155837.jpg", "00000000-2327-8C86-92EF-C751CB287C99 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521004_160306.jpg", "00000000-BFEB-866E-AB6F-38D2F3CB0B76 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521231_215533.jpg", "00000000-4929-88C4-AD38-69AAD00FC489 exact", "", oneFilePerAsset},
		{"Тэжге/IMG_20521231_220630.jpg", "00000000-6C7A-88DF-A476-B27B8EA152DE exact", "", oneFilePerAsset},
	},
	"anon-plan-keep-cloud": {
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A0748.jpg", "00000000-50EF-8D42-8F72-10A13559CEDB same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A1111.jpg", "00000000-D48F-8A90-A38C-8D24B9710D44 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A1426.jpg", "00000000-8C56-88B2-A661-890E039BF696 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A4856.jpg", "00000000-B4ED-82EA-89EF-6FD038578C41 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A5269.jpg", "00000000-8062-8AC3-B9FB-E7C8F5086925 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A6389.jpg", "00000000-A289-80D7-B47A-2D88014A5ED2 same_shot_weak keep_cloud", "", oneFilePerAsset},
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A6605.jpg", "00000000-10F6-8DEE-ABDA-4B45982EA0AD same_shot_weak keep_cloud", "", oneFilePerAsset},
	},
	"anon-same-second": {
		{"2057/01-25 - итпгхдмъгъ/фочнщпфкяъюзлщя/QP1A0874.jpg", "00000000-2CAB-8DDA-9334-B1805763794B same_shot_weak keep_cloud", "", oneFilePerAsset},
	},
}

var fixtureNames = []string{
	"anon-cloud-duplicate", "anon-convert", "anon-date-1970", "anon-edited", "anon-ext-case",
	"anon-fraction", "anon-level-exact", "anon-level-pair-component", "anon-level-probable",
	"anon-level-same-shot", "anon-level-same-shot-weak", "anon-level-same-time-size", "anon-live",
	"anon-many-folders", "anon-nfd", "anon-non-ascii", "anon-plan-keep-cloud", "anon-plan-replace",
	"anon-same-second", "anon-sidecar", "anon-stem-repeat", "anon-suffix-n", "anon-unmatched-cloud",
	"anon-unmatched-icloud", "anon-year-boundary",
}

// The cases are slices of a real library and its cloud, anonymized, with the
// links of the reference manual sync, which a person checked. Matching runs
// as at a first sync and must give those links but for the divergences.
func TestMatchFixtures(t *testing.T) {
	t.Parallel()
	cases, err := fixtures.Load(os.DirFS("../../../testdata/fixtures"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cases {
		names = append(names, c.Name)
	}
	if !slices.Equal(names, fixtureNames) {
		t.Fatalf("fixtures.Load gives the cases %+q; want %+q", names, fixtureNames)
	}
	counts := map[reason]int{}
	for name, ds := range divergences {
		if !slices.Contains(names, name) {
			t.Errorf("divergences of %+q, which is no case", name)
		}
		for _, d := range ds {
			counts[d.why]++
		}
	}
	t.Logf("divergences by reason: %v", counts)
	for _, c := range cases {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			want := map[string]string{}
			for _, l := range c.Expected {
				want[l.Path] = linkString(l.UUID, l.Level, l.Plan)
			}
			for _, d := range divergences[c.Name] {
				if d.ref != want[d.path] || d.ref == d.here {
					t.Errorf("%s: %+q: divergences say %+q instead of %+q (%s); the reference links it as %+q",
						c.Name, d.path, d.here, d.ref, d.why, want[d.path])
				}
				want[d.path] = d.here
			}
			got := fixtureLinks(c)
			paths := slices.Concat(slices.Collect(maps.Keys(want)), slices.Collect(maps.Keys(got)))
			slices.Sort(paths)
			for _, p := range slices.Compact(paths) {
				if got[p] != want[p] {
					t.Errorf("%s: %+q: matching links it as %+q; want %+q", c.Name, p, got[p], want[p])
				}
			}
		})
	}
}

// fixtureLinks matches c as a first sync does and writes every file of a pair
// as the expected links are written: a Live video or a RAW at
// pair_component, and a pair at a level that leaves the copies of the shot
// unequal with replace when the original is the better copy and keep_cloud
// otherwise, as the plan judges them.
func fixtureLinks(c fixtures.Case) map[string]string {
	kept, _ := match.Edited(c.Files, domain.DefaultPolicies().EditedDirs)
	cands, _ := match.Group(kept)
	pairs, _, _ := match.Match(c.Assets, cands)
	links := map[string]string{}
	for _, p := range pairs {
		plan := ""
		if slices.Contains([]domain.MatchLevel{domain.LevelSameShot, domain.LevelSameShotWeak, domain.LevelProbable}, p.Level) {
			a := c.Assets[slices.IndexFunc(c.Assets, func(x domain.Asset) bool { return x.Key == p.Key })]
			plan = "keep_cloud"
			if match.CompareCopies(a, p.Candidate.Main) > 0 {
				plan = "replace"
			}
		}
		links[p.Candidate.Main.Path] = linkString(p.Key.UUID, p.Level, plan)
		for _, e := range []*domain.RemoteEntry{p.Candidate.Live, p.Candidate.RAW} {
			if e != nil {
				links[e.Path] = linkString(p.Key.UUID, domain.LevelPairComponent, "")
			}
		}
	}
	return links
}

func linkString(uuid string, level domain.MatchLevel, plan string) string {
	return strings.TrimSpace(uuid + " " + string(level) + " " + plan)
}

// The five categories that the anonymized library had no case of are
// replayed on worlds of the generator, whose truth tells the asset of every
// file. Matching must pair every asset with the files of its shot, leave
// nothing, give the pairs that it gives without the actions of the
// category, and show the category in some world of the seeds.
func TestMatchCategoriesOnWorlds(t *testing.T) {
	t.Parallel()
	inAnotherEvent := func(w gen.World) []gen.Action {
		var adds []gen.Action
		for _, a := range w.Snapshot.Assets {
			if i := slices.IndexFunc(w.Snapshot.Albums, func(al domain.Album) bool {
				return al.Kind != domain.KindFolder && !slices.Contains(a.AlbumUUIDs, al.UUID)
			}); i >= 0 {
				adds = append(adds, gen.Action{Side: gen.ICloud, Kind: gen.AlbumAdd, Key: a.Key, Album: w.Snapshot.Albums[i].UUID})
			}
		}
		return adds
	}
	for _, c := range []struct {
		name    string
		perturb gen.Perturbation
		actions func(gen.World) []gen.Action
		shows   func(w gen.World, a domain.Asset, p match.Pair) bool
	}{
		{"raw-pair", gen.PerturbRAWPair, nil, func(_ gen.World, _ domain.Asset, p match.Pair) bool {
			return p.Candidate.RAW != nil
		}},
		{"burst", gen.PerturbBurst | gen.PerturbImportRename, nil, func(w gen.World, a domain.Asset, p match.Pair) bool {
			return p.Level == domain.LevelSameTimeSize && slices.ContainsFunc(w.Snapshot.Assets, func(b domain.Asset) bool {
				return b.Key != a.Key && b.Taken.Unix() == a.Taken.Unix()
			})
		}},
		{"tz-shift", gen.PerturbZoneShift | gen.PerturbRecompress, nil, func(_ gen.World, a domain.Asset, p match.Pair) bool {
			return p.Candidate.Main.Meta.TZOffset == nil && !p.Candidate.Main.Meta.Taken.IsZero() && *a.TZOffset != 0
		}},
		{"icloud-duplicate", gen.PerturbDuplicate, nil, func(_ gen.World, a domain.Asset, _ match.Pair) bool {
			return strings.HasSuffix(a.Key.UUID, "-1")
		}},
		{"album-many", 0, inAnotherEvent, func(_ gen.World, a domain.Asset, _ match.Pair) bool {
			return len(a.AlbumUUIDs) > 1
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			shown := 0
			for seed := range uint64(100) {
				plain := gen.New(seed, gen.Config{Frames: 30, Perturb: c.perturb})
				w := plain
				if c.actions != nil {
					w = plain.Apply(c.actions(plain)...)
				}
				cands, _ := match.Group(w.Files)
				pairs, left, alone := match.Match(w.Snapshot.Assets, cands)
				if err := checkMatch(w, w.Snapshot.Assets, cands, pairs, left, alone); err != nil {
					t.Fatalf("%s, seed %d: %v", c.name, seed, err)
				}
				if left != nil || alone != nil {
					t.Fatalf("%s, seed %d: assets %+q and files %+q are left; want none", c.name, seed, assetUUIDs(left), describe(alone))
				}
				plainCands, _ := match.Group(plain.Files)
				if plainPairs, _, _ := match.Match(plain.Snapshot.Assets, plainCands); !reflect.DeepEqual(plainPairs, pairs) {
					t.Fatalf("%s, seed %d: the actions turn the pairs %+q into %+q", c.name, seed, matchPairs(plainPairs), matchPairs(pairs))
				}
				for _, p := range pairs {
					if otherShot(w, p) {
						t.Fatalf("%s, seed %d: %+q goes with asset %+q of another shot", c.name, seed, place(p.Candidate.Main), p.Key.UUID)
					}
					if c.shows(w, w.Snapshot.Assets[slices.IndexFunc(w.Snapshot.Assets, func(x domain.Asset) bool { return x.Key == p.Key })], p) {
						shown++
					}
				}
			}
			if shown == 0 {
				t.Errorf("%s: no pair of the worlds of seeds 0 to 99 shows it", c.name)
			}
		})
	}
}
