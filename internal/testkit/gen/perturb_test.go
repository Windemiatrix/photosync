package gen_test

import (
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
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
	{gen.PerturbLive, "live"},
	{gen.PerturbRAWPair, "raw pair"},
	{gen.PerturbSidecar, "sidecar"},
	{gen.PerturbDuplicate, "duplicate"},
	{gen.PerturbNFD, "nfd"},
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

func TestNewPerturbedVaries(t *testing.T) {
	t.Parallel()

	var upperMOV, lowerMOV, aae, xmp, besideVideo, besideCopy, liveRenamed bool
	for _, seed := range seeds {
		w := gen.New(seed, gen.Config{Frames: bigWorld, Perturb: gen.PerturbAll})
		assetOf := map[domain.AssetKey]domain.Asset{}
		for _, a := range w.Snapshot.Assets {
			assetOf[a.Key] = a
		}
		for _, f := range w.Files {
			a, ext := assetOf[w.Truth[f.Path]], path.Ext(f.Path)
			if ext == path.Ext(a.OriginalName) {
				continue
			}
			stem := strings.TrimSuffix(path.Base(f.Path), ext)
			isCopy, isLive := strings.HasSuffix(stem, " (1)"), ext == ".MOV" || ext == ".mov"
			upperMOV = upperMOV || ext == ".MOV"
			lowerMOV = lowerMOV || ext == ".mov"
			aae = aae || ext == ".AAE"
			xmp = xmp || ext == ".xmp"
			besideVideo = besideVideo || a.IsVideo
			besideCopy = besideCopy || isCopy
			liveRenamed = liveRenamed || isLive && !isCopy && stem != strings.TrimSuffix(a.OriginalName, path.Ext(a.OriginalName))
		}
	}
	var missing []string
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{upperMOV, "a Live video named .MOV"},
		{lowerMOV, "a Live video named .mov"},
		{aae, "an .AAE sidecar"},
		{xmp, "an .xmp sidecar"},
		{besideVideo, "a sidecar beside a video"},
		{besideCopy, "a file beside a duplicate"},
		{liveRenamed, "a Live video of an asset renamed at import"},
	} {
		if !c.ok {
			missing = append(missing, c.what)
		}
	}
	if len(missing) > 0 {
		t.Errorf("worlds of seeds %v and %d frames with every perturbation lack %s",
			seeds, bigWorld, strings.Join(missing, ", "))
	}
}

// Example with fixed seeds gives the same share on every run. The test, not
// rapid, picks the number of frames, so that the share measures the
// perturbations and not the pull of rapid toward small worlds.
func TestGeneratorNotVacuous(t *testing.T) {
	t.Parallel()

	const worlds = 100
	substantive, total := 0, map[gen.Perturbation]int{}
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
		for p, n := range shown {
			total[p] += n
		}
	}
	var counts, missing []string
	for _, k := range perturbKinds {
		counts = append(counts, fmt.Sprintf("%s %d", k.name, total[k.p]))
		if total[k.p] == 0 {
			missing = append(missing, k.name)
		}
	}
	t.Logf("%d of %d worlds of 2 to 30 frames show a perturbation; shots by kind: %s",
		substantive, worlds, strings.Join(counts, ", "))
	if substantive < 70 {
		t.Errorf("%d of %d worlds of 2 to 30 frames show a perturbation; want at least 70", substantive, worlds)
	}
	if len(missing) > 0 {
		t.Errorf("no world of seeds 0 to %d shows %s", worlds-1, strings.Join(missing, ", "))
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
// first difference between the assets and the files of w that no
// perturbation explains, or the first way in which w with the differences
// undone breaks the contract of a world of shots shots.
func perturbations(w gen.World, shots int) (map[gen.Perturbation]int, error) {
	s := w.Snapshot
	s.Assets = slices.Clone(s.Assets)
	assetAt := map[domain.AssetKey]int{}
	for i := range s.Assets {
		s.Assets[i].Resources = slices.Clone(s.Assets[i].Resources)
		assetAt[s.Assets[i].Key] = i
	}
	names := map[string]bool{}
	for _, f := range w.Files {
		names[path.Base(f.Path)] = true
	}
	truth, shown := maps.Clone(w.Truth), map[gen.Perturbation]int{}
	files, copies, err := undoCopies(w, assetAt, truth, shown)
	if err != nil {
		return nil, err
	}
	if files, err = undoBeside(files, s.Assets, assetAt, truth, shown); err != nil {
		return nil, err
	}

	for i := range files {
		f := &files[i]
		key, ok := truth[f.Path]
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
	s.Assets = slices.DeleteFunc(s.Assets, func(a domain.Asset) bool {
		_, isCopy := copies[a.Key]
		return isCopy
	})

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

	for i, f := range files {
		if p := norm.NFC.String(f.Path); p != f.Path {
			shown[gen.PerturbNFD]++
			truth[p] = truth[f.Path]
			delete(truth, f.Path)
			files[i].Path = p
		}
	}

	if err := check(gen.World{Snapshot: s, Files: files, Truth: truth}, shots); err != nil {
		return nil, fmt.Errorf("with the perturbations undone: %w", err)
	}
	return shown, nil
}

// undoCopies takes the copies of PerturbDuplicate out of the files of w and
// out of truth, counts the copied shots in shown and returns the files left
// and, for each copy, the key of its original, or the first copy that
// is not one.
func undoCopies(w gen.World, assetAt map[domain.AssetKey]int, truth map[string]domain.AssetKey,
	shown map[gen.Perturbation]int,
) ([]domain.RemoteEntry, map[domain.AssetKey]domain.AssetKey, error) {
	fileAt, filesOf := map[string]domain.RemoteEntry{}, map[domain.AssetKey]int{}
	for _, f := range w.Files {
		fileAt[f.Path] = f
		filesOf[w.Truth[f.Path]]++
	}
	var files []domain.RemoteEntry
	originals := map[domain.AssetKey]domain.AssetKey{}
	for _, f := range w.Files {
		ext := path.Ext(f.Path)
		stem, isCopy := strings.CutSuffix(strings.TrimSuffix(f.Path, ext), " (1)")
		if !isCopy {
			files = append(files, f)
			continue
		}
		g, hasOriginal := fileAt[stem+ext]
		key, copyKey := w.Truth[g.Path], w.Truth[f.Path]
		i, isAsset := assetAt[key]
		j, isCopyAsset := assetAt[copyKey]
		if seen, ok := originals[copyKey]; !hasOriginal || !isAsset || !isCopyAsset || i == j || (ok && seen != key) {
			return nil, nil, fmt.Errorf("file %q copies file %q; want files of two assets, one asset copied once", f.Path, stem+ext)
		}
		a, c, h := w.Snapshot.Assets[i], w.Snapshot.Assets[j], f
		c.Key, h.Path = a.Key, g.Path
		if !reflect.DeepEqual(c, a) || c.TZOffset == a.TZOffset || !reflect.DeepEqual(h, g) ||
			(h.Meta.TZOffset != nil && h.Meta.TZOffset == g.Meta.TZOffset) {
			return nil, nil, fmt.Errorf("asset %q with file %q copies asset %q with file %q; "+
				"want them equal but for the key and the name, under their own offset pointers",
				copyKey.UUID, f.Path, key.UUID, g.Path)
		}
		originals[copyKey] = key
		if ext == path.Ext(a.OriginalName) {
			shown[gen.PerturbDuplicate]++
		}
		delete(truth, f.Path)
	}
	for _, c := range slices.SortedFunc(maps.Keys(originals), domain.AssetKey.Compare) {
		if filesOf[c] != filesOf[originals[c]] {
			return nil, nil, fmt.Errorf("asset %q has %d files and its copy %q %d; want a copy of every file",
				originals[c].UUID, filesOf[originals[c]], c.UUID, filesOf[c])
		}
	}
	return files, originals, nil
}

// undoBeside takes the files beside the file of a shot out of files and out
// of truth and their resources out of assets, counts them in shown and
// returns the files left, or the first file beside that is not one.
func undoBeside(files []domain.RemoteEntry, assets []domain.Asset, assetAt map[domain.AssetKey]int,
	truth map[string]domain.AssetKey, shown map[gen.Perturbation]int,
) ([]domain.RemoteEntry, error) {
	var left []domain.RemoteEntry
	for _, f := range files {
		ext := path.Ext(f.Path)
		j, ok := assetAt[truth[f.Path]]
		if !ok || len(assets[j].Resources) == 0 || ext == path.Ext(assets[j].OriginalName) {
			left = append(left, f)
			continue
		}
		a := &assets[j]
		stem := strings.TrimSuffix(a.OriginalName, path.Ext(a.OriginalName))
		kind, want := gen.PerturbSidecar, domain.Resource{Size: a.Resources[0].Size}
		switch {
		case !a.IsVideo && (ext == ".MOV" || ext == ".mov"):
			kind, want.Role, want.Filename = gen.PerturbLive, domain.ResourceLiveVideo, stem+".MOV"
		case !a.IsVideo && ext == ".DNG":
			kind, want.Role, want.Filename = gen.PerturbRAWPair, domain.ResourceRAWPair, stem+".DNG"
		case ext != ".AAE" && ext != ".xmp":
			return nil, fmt.Errorf("file %q lies beside a file of asset %q; want .MOV, .mov or .DNG beside a photo, or .AAE or .xmp",
				f.Path, a.Key.UUID)
		}
		shot := strings.TrimSuffix(f.Path, ext) + path.Ext(a.OriginalName)
		if key, isShot := truth[shot]; !isShot || key != a.Key ||
			!reflect.DeepEqual(f, domain.RemoteEntry{Root: domain.LibraryPersonal, Path: f.Path, Size: want.Size}) {
			return nil, fmt.Errorf("file %q of asset %q is %+v; want %d bytes, no metadata and a file of the asset at %q",
				f.Path, a.Key.UUID, f, want.Size, shot)
		}
		if want.Role != "" {
			r := slices.Index(a.Resources[1:], want)
			if r < 0 {
				return nil, fmt.Errorf("asset %q has resources %+v; want %+v for file %q", a.Key.UUID, a.Resources, want, f.Path)
			}
			a.Resources = slices.Delete(a.Resources, 1+r, 2+r)
		}
		shown[kind]++
		delete(truth, f.Path)
	}
	return left, nil
}
