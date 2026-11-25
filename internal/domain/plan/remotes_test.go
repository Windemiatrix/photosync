package plan_test

import (
	"cmp"
	"fmt"
	"maps"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

var (
	remoteNames = []string{"alpha", "beta", "gamma"}
	remoteModes = []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly}
)

// withHashes gives the files, and the resources of the assets, the hash of
// kind of their size and extension, as a remote that reports hashes would
// list them: a photo and its RAW file or Live Photo video of one size are
// still two contents, md5 and sha256 never spell one string, and a file that
// a run puts lists the hash of the resource it comes from. HashNone gives
// none.
func withHashes(kind domain.HashKind, s domain.LibrarySnapshot, files []domain.RemoteEntry) (domain.LibrarySnapshot, []domain.RemoteEntry) {
	hash := func(size int64, name string) (md5, sha256 string) {
		ext := strings.ToLower(path.Ext(name))
		switch kind {
		case domain.HashMD5:
			return fmt.Sprintf("%032x%s", size, ext), ""
		case domain.HashSHA256:
			return "", fmt.Sprintf("%064x%s", size, ext)
		}
		return "", ""
	}
	files = slices.Clone(files)
	for i, e := range files {
		if !e.IsDir {
			files[i].MD5, files[i].SHA256 = hash(e.Size, e.Path)
		}
	}
	s.Assets = slices.Clone(s.Assets)
	for i, a := range s.Assets {
		s.Assets[i].Resources = slices.Clone(a.Resources)
		for j, r := range a.Resources {
			s.Assets[i].Resources[j].MD5, s.Assets[i].Resources[j].SHA256 = hash(r.Size, r.Filename)
		}
	}
	return s, files
}

// The user acts in the cloud of the last remote alone, so that the others
// learn of it only through Photos, which is what a run carries between
// remotes.
func remoteCases(modes ...domain.Mode) *rapid.Generator[plan.Input] {
	return rapid.Custom(func(t *rapid.T) plan.Input {
		kind := rapid.SampledFrom([]domain.HashKind{domain.HashMD5, domain.HashSHA256, domain.HashNone}).Draw(t, "hash")
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(1, 8).Draw(t, "frames"), Perturb: converging}).Draw(t, "world")
		snap, files := withHashes(kind, w.Snapshot, w.Files)
		tmpl, err := template.Parse("{event_path}/{original_name}")
		if err != nil {
			t.Fatalf("template.Parse: %v", err)
		}
		p := domain.DefaultPolicies()
		p.Deletes = rapid.SampledFrom([]domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate}).Draw(t, "deletes")
		p.Conflict = rapid.SampledFrom([]domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip}).Draw(t, "conflict")
		first := plan.Input{
			Snapshot: snap, Scans: map[string][]domain.RemoteEntry{}, Policies: p, Template: tmpl,
			Fields: fieldsByKey(snap), Now: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), RunID: "run-1",
		}
		for i, m := range modes {
			first.Remotes = append(first.Remotes, domain.RemoteSpec{Name: remoteNames[i], Mode: m, Capabilities: domain.Capabilities{ContentHash: kind}})
			first.Scans[remoteNames[i]] = files
		}
		p1, delta, err := plan.Build(first)
		if err != nil {
			t.Fatalf("the first Build: %v", err)
		}
		synced := after(first, p1, delta)
		last := remoteNames[len(modes)-1]
		var mine, service []domain.RemoteEntry
		for _, e := range synced.Scans[last] {
			if e.IsDir || strings.HasPrefix(e.Path, ".photosync/") {
				service = append(service, e)
			} else {
				mine = append(mine, e)
			}
		}
		was := gen.World{Snapshot: synced.Snapshot, Files: mine}
		now := was.Apply(gen.Actions(was).Draw(t, "actions")...)
		in := synced
		in.Scans = maps.Clone(synced.Scans)
		in.Snapshot, now.Files = withHashes(kind, now.Snapshot, now.Files)
		in.Scans[last], in.Fields = slices.Concat(now.Files, service), fieldsByKey(in.Snapshot)
		return in
	})
}

func remoteShuffled(t *rapid.T, in plan.Input) plan.Input {
	in.Snapshot.Assets = rapid.Permutation(in.Snapshot.Assets).Draw(t, "assets")
	in.Snapshot.Albums = rapid.Permutation(in.Snapshot.Albums).Draw(t, "albums")
	in.Remotes = rapid.Permutation(in.Remotes).Draw(t, "remotes")
	scans := map[string][]domain.RemoteEntry{}
	for _, name := range slices.Sorted(maps.Keys(in.Scans)) {
		scans[name] = rapid.Permutation(in.Scans[name]).Draw(t, "entries")
	}
	in.Scans = scans
	return in
}

// One content new in each of three remotes comes into Photos once by its
// hash, and the run links the files of the other remotes along;
// without hashes each file comes in as an asset of its own, and the next run
// puts each into the other two remotes. Either way the third run has nothing
// to do.
func TestPlanThreeRemotesOneContent(t *testing.T) {
	t.Parallel()
	// covers: INV-1
	w := gen.New(1, gen.Config{Frames: 6})
	added := domain.RemoteEntry{
		Root: domain.LibraryPersonal, Path: path.Join(path.Dir(w.Files[0].Path), "NEW.HEIC"), Size: 7_000_000,
		Meta: domain.MediaMeta{Taken: time.Date(2025, time.June, 1, 12, 0, 0, 0, time.UTC)},
	}
	put := domain.OpPut
	for _, c := range []struct {
		hash  domain.HashKind
		kinds [3][]domain.OpKind
	}{
		{domain.HashMD5, [3][]domain.OpKind{{domain.OpImport}, nil, nil}},
		{domain.HashNone, [3][]domain.OpKind{{domain.OpImport, domain.OpImport, domain.OpImport}, {put, put, put, put, put, put}, nil}},
	} {
		t.Run(cmp.Or(string(c.hash), "none"), func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.Parse("{event_path}/{original_name}")
			if err != nil {
				t.Fatalf("template.Parse: %v", err)
			}
			policies := domain.DefaultPolicies()
			policies.Deletes = domain.DeletesQuarantine
			snap, files := withHashes(c.hash, w.Snapshot, w.Files)
			_, news := withHashes(c.hash, domain.LibrarySnapshot{}, []domain.RemoteEntry{added})
			in := plan.Input{
				Snapshot: snap, Scans: map[string][]domain.RemoteEntry{}, Policies: policies, Template: tmpl,
				Fields: fieldsByKey(snap), Now: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), RunID: "run-1",
			}
			for _, name := range remoteNames {
				in.Remotes = append(in.Remotes, domain.RemoteSpec{Name: name, Mode: domain.ModeTwoWay, Capabilities: domain.Capabilities{ContentHash: c.hash}})
				in.Scans[name] = files
			}
			first, delta, err := plan.Build(in)
			if err != nil {
				t.Fatalf("the first Build: %v", err)
			}
			in = after(in, first, delta)
			for _, name := range remoteNames {
				in.Scans[name] = append(slices.Clone(in.Scans[name]), news...)
			}
			for run := range 3 {
				p, delta, err := plan.Build(in)
				var kinds []domain.OpKind
				for _, o := range p.Ops {
					kinds = append(kinds, o.Kind)
				}
				if err != nil || !slices.Equal(kinds, c.kinds[run]) || len(p.Links)+len(p.Albums)+len(p.Pairs)+len(p.Conflicts) > 0 {
					t.Fatalf("run %d: Build = %+v, %v; want the operations %v and no records", run+1, p, err, c.kinds[run])
				}
				in = after(in, p, delta)
			}
		})
	}
}

// A run of one remote leaves the others as they are.
func TestPlanOnlyRemoteOnGenWorlds(t *testing.T) {
	t.Parallel()
	// covers: INV-6
	rapid.Check(t, func(t *rapid.T) {
		in := remoteCases(rapid.SampledFrom(remoteModes).Draw(t, "mode"), domain.ModeTwoWay).Draw(t, "case")
		in.OnlyRemote = "alpha"
		p, _, err := plan.Build(in)
		if err != nil {
			t.Fatalf("Build of alpha alone: %v", err)
		}
		if slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return o.Remote == "beta" }) ||
			slices.ContainsFunc(p.Links, func(l domain.Link) bool { return l.Remote == "beta" }) ||
			slices.ContainsFunc(p.Pairs, func(x domain.AlbumPair) bool { return x.Remote == "beta" }) ||
			slices.ContainsFunc(p.Conflicts, func(c domain.Conflict) bool { return c.Remote == "beta" }) {
			t.Fatalf("the run of alpha alone plans for beta:\n%+v", p)
		}
	})
}

// The order of the snapshot, the listings and the remotes does not change
// the plan while the policies name the priority of the remotes.
func TestPlanDeterministicOnGenWorlds(t *testing.T) {
	t.Parallel()
	// covers: INV-9
	rapid.Check(t, func(t *rapid.T) {
		in := remoteCases(rapid.SliceOfN(rapid.SampledFrom(remoteModes), 3, 3).Draw(t, "modes")...).Draw(t, "case")
		in.Policies.RemotePriority = remoteNames
		want, _, err := plan.Build(in)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		for run := range 20 {
			if got, _, err := plan.Build(in); err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("Build %d = %+v, %v; want %+v", run+2, got, err, want)
			}
		}
		if got, _, err := plan.Build(remoteShuffled(t, in)); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("Build of the input in another order = %+v, %v; want %+v", got, err, want)
		}
	})
}

// The digest names what the operations of a plan do, not the run: not its
// ID, its time within one UTC day, the order of what it reads, the order of
// the remotes under a named priority or the cloud GUID of a new asset.
func TestDigestOnGenWorlds(t *testing.T) {
	t.Parallel()
	// covers: INV-17
	rapid.Check(t, func(t *rapid.T) {
		in := remoteCases(rapid.SliceOfN(rapid.SampledFrom(remoteModes), 1, 3).Draw(t, "modes")...).Draw(t, "case")
		in.Policies.RemotePriority = remoteNames[:len(in.Remotes)]
		digest := func(in plan.Input) string {
			p, delta, err := plan.Build(in)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			d, err := plan.Digest(p, delta)
			if err != nil {
				t.Fatalf("Digest: %v", err)
			}
			return d
		}
		want := digest(in)
		other := in
		other.RunID, other.Now = "another run", in.Now.Add(time.Hour)
		known := map[string]bool{}
		for _, a := range in.State.Assets {
			known[a.Key.UUID] = true
		}
		other.Snapshot.Assets = slices.Clone(in.Snapshot.Assets)
		for i, a := range other.Snapshot.Assets {
			if !known[a.Key.UUID] {
				other.Snapshot.Assets[i].Key.CloudAssetGUID = "guid-later-" + a.Key.UUID
			}
		}
		other.Fields = fieldsByKey(other.Snapshot)
		if got := digest(remoteShuffled(t, other)); got != want {
			t.Fatalf("the digest of the same operations is %s; want %s", got, want)
		}
		p, delta, err := plan.Build(in)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if cut, err := plan.Digest(plan.Limit(p, 1), delta); err != nil || assetsWithOps(p) > 1 && cut == want {
			t.Fatalf("the digest of fewer operations is %s, %v; want another than %s", cut, err, want)
		}
	})
}

// The cases of the properties above must hold what they claim to check. Fixed
// seeds give the same cases on every run, and the bounds are about half of
// what they give, so a change of the generator that halves them fails here.
func TestRemoteCasesNotVacuous(t *testing.T) {
	t.Parallel()
	const cases = 100
	var hashed, carried int
	for seed := range cases {
		in := remoteCases(domain.ModeUploadOnly, domain.ModeTwoWay).Example(seed)
		p, _, err := plan.Build(in)
		if err != nil {
			t.Fatalf("seed %d: Build: %v", seed, err)
		}
		if in.Remotes[0].Capabilities.ContentHash != domain.HashNone {
			hashed++
		}
		if slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return o.Remote == "beta" }) {
			carried++
		}
	}
	t.Logf("of %d cases %d list hashes and %d plan for the remote where the user acts", cases, hashed, carried)
	if hashed < 33 || carried < 43 {
		t.Errorf("the cases of seeds 0 to %d lack 33 that list hashes (%d do) or 43 that plan for the remote where the user acts (%d do)", cases-1, hashed, carried)
	}
}
