package gen_test

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

var perturbKinds = []struct {
	p    gen.Perturbation
	name string
}{
	{gen.PerturbImportRename, "import rename"},
	{gen.PerturbRecompress, "recompress"},
	{gen.PerturbZoneShift, "zone shift"},
	{gen.PerturbRoundSubSec, "round subsec"},
	{gen.PerturbNoDate, "no date"},
	{gen.PerturbBurst, "burst"},
	{gen.PerturbSameName, "same name"},
}

func TestNewPerturbed(t *testing.T) {
	t.Parallel()

	for _, seed := range seeds {
		shown, err := perturbations(gen.New(seed, gen.Config{Frames: bigWorld}), bigWorld)
		if err != nil || len(shown) != 0 {
			t.Errorf("seed %d without perturbations: shown %v, error %v; want none", seed, shown, err)
		}
	}

	run := func(name string, p gen.Perturbation) {
		total := map[gen.Perturbation]int{}
		for _, seed := range seeds {
			shown, err := perturbations(gen.New(seed, gen.Config{Frames: bigWorld, Perturb: p}), bigWorld)
			if err != nil {
				t.Errorf("%s, seed %d: %v", name, seed, err)
				continue
			}
			for _, x := range slices.Sorted(maps.Keys(shown)) {
				if p&x == 0 {
					t.Errorf("%s, seed %d: %d shots show %s", name, seed, shown[x], perturbName(x))
				}
				total[x] += shown[x]
			}
		}
		var missing []string
		for _, k := range perturbKinds {
			if p&k.p != 0 && total[k.p] == 0 {
				missing = append(missing, k.name)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%s: no shot of seeds %v shows %s", name, seeds, strings.Join(missing, ", "))
		}
	}
	for _, k := range perturbKinds {
		run(k.name, k.p)
	}
	run("all", gen.PerturbAll)
}

func TestWorldsPerturbed(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 30).Draw(t, "frames")
		p := gen.Perturbation(rapid.Uint().Draw(t, "perturb"))
		w := gen.Worlds(gen.Config{Frames: n, Perturb: p}).Draw(t, "world")
		shown, err := perturbations(w, n)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range slices.Sorted(maps.Keys(shown)) {
			if p&x == 0 {
				t.Fatalf("perturb %b: %d shots show %s", p, shown[x], perturbName(x))
			}
		}
	})
}

// Example with fixed seeds gives the same share on every run. The test, not
// rapid, picks the number of frames, so that the share measures the
// perturbations and not the pull of rapid toward small worlds.
func TestGeneratorNotVacuous(t *testing.T) {
	t.Parallel()

	const worlds = 100
	substantive := 0
	for seed := range worlds {
		frames := 2 + seed%29
		w := gen.Worlds(gen.Config{Frames: frames, Perturb: gen.PerturbAll}).Example(seed)
		shown, err := perturbations(w, frames)
		if err != nil {
			t.Fatalf("seed %d, %d frames: %v", seed, frames, err)
		}
		if len(shown) > 0 {
			substantive++
		}
	}
	t.Logf("%d of %d worlds of 2 to 30 frames show a perturbation", substantive, worlds)
	if substantive < 70 {
		t.Errorf("%d of %d worlds of 2 to 30 frames show a perturbation; want at least 70", substantive, worlds)
	}
}

func perturbName(p gen.Perturbation) string {
	for _, k := range perturbKinds {
		if k.p == p {
			return k.name
		}
	}
	return fmt.Sprintf("bits %b", p)
}

// perturbations returns how many shots of w show each perturbation, or the
// first difference between an asset and its file that no perturbation
// explains, or the first way in which w with the differences undone breaks
// the contract of a world of shots shots.
func perturbations(w gen.World, shots int) (map[gen.Perturbation]int, error) {
	s := w.Snapshot
	s.Assets = slices.Clone(s.Assets)
	assetAt := map[domain.AssetKey]int{}
	for i := range s.Assets {
		s.Assets[i].Resources = slices.Clone(s.Assets[i].Resources)
		assetAt[s.Assets[i].Key] = i
	}
	files := slices.Clone(w.Files)
	names := map[string]bool{}
	for _, f := range files {
		names[path.Base(f.Path)] = true
	}
	shown := map[gen.Perturbation]int{}

	for i := range files {
		f := &files[i]
		key, ok := w.Truth[f.Path]
		j, found := assetAt[key]
		if !ok || !found {
			continue
		}
		a := &s.Assets[j]
		if len(a.Resources) != 1 || a.TZOffset == nil {
			continue
		}
		r, base := &a.Resources[0], path.Base(f.Path)

		if a.OriginalName != base {
			ext := path.Ext(a.OriginalName)
			stem := strings.TrimSuffix(a.OriginalName, ext)
			if len(stem) != 10 || !strings.HasPrefix(stem, "IMG_") || strings.Trim(stem[4:], "0123456789") != "" ||
				ext != path.Ext(base) || names[a.OriginalName] || r.Filename != a.OriginalName {
				return nil, fmt.Errorf("asset %q is named %q, its original %q, its file %q; "+
					"want the name of the file, or IMG_, six digits and its extension that no file has",
					a.Key.UUID, a.OriginalName, r.Filename, base)
			}
			shown[gen.PerturbImportRename]++
			a.OriginalName, r.Filename = base, base
		}

		if f.Size != r.Size {
			if f.Size <= 0 || f.Size > r.Size {
				return nil, fmt.Errorf("file %q has %d bytes; want %d, those of asset %q, or fewer", f.Path, f.Size, r.Size, a.Key.UUID)
			}
			shown[gen.PerturbRecompress]++
			f.Size = r.Size
		}

		if f.Meta.Taken.IsZero() && f.Meta.TZOffset == nil && f.Meta.SubSec == "" {
			shown[gen.PerturbNoDate]++
			offset := *a.TZOffset
			f.Meta.Taken, f.Meta.TZOffset, f.Meta.SubSec = a.Taken, &offset, a.SubSec
			continue
		}

		want := a.Taken
		if f.Meta.TZOffset == nil {
			shown[gen.PerturbZoneShift]++
			want = want.Add(time.Duration(*a.TZOffset) * time.Second)
			offset := *a.TZOffset
			f.Meta.TZOffset = &offset
		}
		if f.Meta.SubSec == "" && a.SubSec != "" {
			shown[gen.PerturbRoundSubSec]++
			if frac, err := strconv.ParseFloat("0."+a.SubSec, 64); err == nil && frac >= 0.5 {
				want = want.Add(time.Second)
			}
			f.Meta.SubSec = a.SubSec
		}
		if !f.Meta.Taken.Equal(want) {
			return nil, fmt.Errorf("file %q was taken at %v; want %v, from asset %q and the perturbations it shows",
				f.Path, f.Meta.Taken, want, a.Key.UUID)
		}
		f.Meta.Taken = a.Taken
	}

	years := map[string][]string{}
	for _, f := range files {
		year, _, _ := strings.Cut(f.Path, "/")
		stem := strings.TrimSuffix(path.Base(f.Path), path.Ext(f.Path))
		years[stem] = append(years[stem], year)
	}
	for _, stem := range slices.Sorted(maps.Keys(years)) {
		group := years[stem]
		if len(group) < 2 {
			continue
		}
		if len(slices.Compact(slices.Sorted(slices.Values(group)))) < len(group) {
			return nil, fmt.Errorf("files named %q lie in one year: %q", stem, group)
		}
		shown[gen.PerturbSameName] += len(group) - 1
	}

	moments := map[string][]domain.Asset{}
	for _, a := range s.Assets {
		if a.TZOffset != nil {
			k := fmt.Sprint(a.AlbumUUIDs, a.Taken, *a.TZOffset)
			moments[k] = append(moments[k], a)
		}
	}
	for _, k := range slices.Sorted(maps.Keys(moments)) {
		group := moments[k]
		if len(group) < 2 {
			continue
		}
		uuids := make([]string, 0, len(group))
		for _, a := range group {
			uuids = append(uuids, a.Key.UUID)
		}
		subsecs := map[string]bool{}
		for _, a := range group {
			if a.IsVideo || subsecs[a.SubSec] {
				return nil, fmt.Errorf("assets %q of a burst: want photos of different SubSec", uuids)
			}
			subsecs[a.SubSec] = true
		}
		shown[gen.PerturbBurst] += len(group) - 1
	}

	if err := check(gen.World{Snapshot: s, Files: files, Truth: w.Truth}, shots); err != nil {
		return nil, fmt.Errorf("with the perturbations undone: %w", err)
	}
	return shown, nil
}
