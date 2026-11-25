package plan_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
	"github.com/Windemiatrix/photosync/internal/testkit/refmodel"
)

// modelPerturb leaves out of the converging worlds what the reference model
// does not hold: an asset of more than one resource or file, and two files of
// one name in a directory, as the model numbers no names.
const modelPerturb = converging &^ (gen.PerturbLive | gen.PerturbRAWPair | gen.PerturbSidecar | gen.PerturbDuplicate | gen.PerturbSameName)

// modelPolicies are the pairs that the reference model is tested under: a
// strict pair settles every conflict and removes what a side deleted, the
// others leave files and assets to the user.
var modelPolicies = []struct {
	deletes  domain.DeletesPolicy
	conflict domain.ConflictPolicy
}{
	{domain.DeletesNone, domain.ConflictAsk},
	{domain.DeletesQuarantine, domain.ConflictRemoteWins},
	{domain.DeletesPropagate, domain.ConflictICloudWins},
	{domain.DeletesQuarantine, domain.ConflictSkip},
}

func modelService(e domain.RemoteEntry) bool {
	return e.IsDir || strings.HasPrefix(e.Path, ".photosync/")
}

// modelWorld is what a run left, as the reference model reads a world: a scan
// skips the quarantine, and a file belongs to the asset that an active link of
// the state names.
func modelWorld(in plan.Input) gen.World {
	keys := map[domain.AssetID]domain.AssetKey{}
	for _, a := range in.State.Assets {
		keys[a.ID] = a.Key
	}
	w := gen.World{Snapshot: in.Snapshot, Files: slices.DeleteFunc(slices.Clone(in.Scans["alpha"]), modelService), Truth: map[string]domain.AssetKey{}}
	for _, l := range in.State.Links {
		if l.Status == domain.StatusActive {
			w.Truth[l.Path] = keys[l.AssetID]
		}
	}
	return w
}

// The model starts from a synced world, which a first run that leaves a file,
// or an asset outside Recently Deleted, without a link does not give.
func modelSynced(b refmodel.World) bool {
	linked := map[domain.AssetKey]bool{}
	for _, f := range b.Files {
		if f.Key == (domain.AssetKey{}) {
			return false
		}
		linked[f.Key] = true
	}
	for k, o := range b.Objects {
		if !o.Trashed && !linked[k] {
			return false
		}
	}
	return true
}

// A run after the first sync of a world of package gen and the actions of a
// user leaves what the reference model expects of a correct sync, wherever
// the model takes the world. Fixed seeds rather than rapid: few worlds lie
// inside the model, fewer than rapid.Skip would bear.
func TestPlanMatchesModelOnGenWorlds(t *testing.T) {
	t.Parallel()
	const seeds = 200
	tmpl, err := template.Parse("{event_path}/{original_name}")
	if err != nil {
		t.Fatalf("template.Parse: %v", err)
	}
	inside, unsynced := 0, 0
	for seed := range seeds {
		w := gen.Worlds(gen.Config{Frames: 1 + seed%6, Perturb: modelPerturb}).Example(seed)
		kind := []domain.HashKind{domain.HashMD5, domain.HashSHA256}[seed/6%2]
		snap, files := withHashes(kind, w.Snapshot, w.Files)
		took := false
		for _, c := range modelPolicies {
			p := domain.DefaultPolicies()
			p.Deletes, p.Conflict = c.deletes, c.conflict
			first := plan.Input{
				Snapshot: snap, Scans: map[string][]domain.RemoteEntry{"alpha": files}, Policies: p, Template: tmpl,
				Remotes: []domain.RemoteSpec{{Name: "alpha", Mode: domain.ModeTwoWay, Capabilities: domain.Capabilities{ContentHash: kind}}},
				Fields:  fieldsByKey(snap), Now: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), RunID: "run-1",
			}
			p1, d1, err := plan.Build(first)
			if err != nil {
				t.Fatalf("seed %d: the first Build: %v", seed, err)
			}
			synced := after(first, p1, d1)
			base := modelWorld(synced)
			actions := gen.Actions(base).Example(seeds + seed)
			now := base.Apply(actions...)
			m, err := refmodel.New(base, now)
			if err != nil {
				continue
			}
			if !modelSynced(m.Base) {
				unsynced++
				continue
			}
			want, err := m.Expected(c.deletes, c.conflict)
			if err != nil {
				continue
			}
			in := synced
			in.Snapshot, now.Files = withHashes(kind, now.Snapshot, now.Files)
			service := slices.DeleteFunc(slices.Clone(synced.Scans["alpha"]), func(e domain.RemoteEntry) bool { return !modelService(e) })
			in.Scans, in.Fields = map[string][]domain.RemoteEntry{"alpha": slices.Concat(now.Files, service)}, fieldsByKey(in.Snapshot)
			p2, d2, err := plan.Build(in)
			if err != nil {
				t.Fatalf("seed %d, %s %s: Build after %+v: %v", seed, c.deletes, c.conflict, actions, err)
			}
			got, err := refmodel.Read(modelWorld(after(in, p2, d2)))
			if err == nil {
				got = m.Canon(got)
			}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("seed %d, %s %s: after %+v the plan\n%+v\nleaves %+v, error %v\nwant %+v", seed, c.deletes, c.conflict, actions, p2, got, err, want)
			}
			took = true
		}
		if took {
			inside++
		}
	}
	t.Logf("%d of %d seeds lie inside the model; under %d pairs of a seed and policies the first sync leaves a file or an asset without a link", inside, seeds, unsynced)
	if inside < 37 {
		t.Errorf("fewer than 37 of the seeds 0 to %d lie inside the model (%d do)", seeds-1, inside)
	}
}
