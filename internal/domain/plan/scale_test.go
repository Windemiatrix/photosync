package plan_test

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

// A first sync of twenty thousand shots with one remote and with three, and
// the run after it, must plan within the ceilings. The world is made before
// the clock starts, and the test is not parallel, so that no other test of
// the package runs or allocates beside it.
func TestPlanScale(t *testing.T) {
	if testing.Short() {
		t.Skip("plans twenty thousand shots")
	}
	w := gen.New(1, gen.Config{Frames: 20_000, Perturb: gen.PerturbAll})
	tmpl, err := template.Parse("{event_path}/{original_name}")
	if err != nil {
		t.Fatalf("template.Parse: %v", err)
	}
	p := domain.DefaultPolicies()
	p.Deletes, p.UntitledEvent, p.AlbumsRoot = domain.DeletesQuarantine, "Untitled", "Albums"
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	// The times are three times the slowest measured on the macOS CI runner
	// under the race detector with one and four CPUs, about three times slower
	// than a laptop: a loaded runner passes, an algorithm several times slower
	// does not. The bytes allocated do not depend on the machine, so twice
	// those measured leave room for new rules but not for a copy of every
	// album, pair or file for each asset.
	for _, c := range []struct {
		remotes                  int
		build, simulate, rebuild time.Duration
		bytes                    uint64
	}{
		{1, 0, 0, 0, 0},
		{3, 0, 0, 0, 0},
	} {
		t.Run(fmt.Sprintf("remotes=%d", c.remotes), func(t *testing.T) {
			in := plan.Input{
				Snapshot: w.Snapshot, Scans: map[string][]domain.RemoteEntry{}, Policies: p, Template: tmpl, Now: now, RunID: "run-1",
				// Fields answers at once, so that the times are those of planning, not
				// of reading a photo library.
				Fields: func(k domain.AssetKey) template.Photo {
					return templateFields{"original_name": {Strings: []string{k.UUID}}}
				},
			}
			for _, name := range []string{"alpha", "beta", "gamma"}[:c.remotes] {
				in.Remotes = append(in.Remotes, domain.RemoteSpec{Name: name, Mode: domain.ModeTwoWay, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}})
				in.Scans[name] = w.Files
			}
			start := time.Now()
			first, delta, err := plan.Build(in)
			took := time.Since(start)
			if half := c.remotes * len(w.Snapshot.Assets) / 2; err != nil || len(first.Links) < half {
				t.Fatalf("the first Build: %d links, error %v; want no error and at least %d links, half of the assets in each remote", len(first.Links), err, half)
			}
			if took > c.build {
				t.Errorf("the first Build of %d assets, %d albums and %d files in each remote, with %d operations and %d records, takes %v; want at most %v",
					len(w.Snapshot.Assets), len(w.Snapshot.Albums), len(w.Files), len(first.Ops), len(first.Links), took, c.build)
			}
			start = time.Now()
			state, snap, scans := plan.Simulate(in, first, delta)
			if took = time.Since(start); took > c.simulate {
				t.Errorf("Simulate of the first plan takes %v; want at most %v", took, c.simulate)
			}
			// An hour later the files that the first run puts have settled.
			next := in
			next.State, next.Snapshot, next.Scans, next.Now, next.RunID = state, snap, scans, now.Add(time.Hour), "run-2"
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			start = time.Now()
			second, _, err := plan.Build(next)
			took = time.Since(start)
			runtime.ReadMemStats(&after)
			if err != nil {
				t.Fatalf("the second Build: %v", err)
			}
			if took > c.rebuild {
				t.Errorf("the second Build with %d operations and %d records takes %v; want at most %v", len(second.Ops), len(second.Links), took, c.rebuild)
			}
			if got := after.TotalAlloc - before.TotalAlloc; got > c.bytes {
				t.Errorf("the second Build allocates %d bytes for %d assets; want at most %d", got, len(snap.Assets), c.bytes)
			}
		})
	}
}
