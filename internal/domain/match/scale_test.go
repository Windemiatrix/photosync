package match_test

import (
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/match"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

// Three times the slowest best time measured on the macOS CI runner under the
// race detector with one and four CPUs, about three times slower than a
// laptop: a loaded runner passes, an algorithm several times slower does not.
const matchScaleCeiling = 18 * time.Second

// A first sync of twenty thousand shots must match within the ceiling. The
// world is made before the clock starts, and the best of three runs keeps a
// pause of the machine out. The test is not parallel, so that no other test
// of the package runs beside it.
func TestMatchScale(t *testing.T) {
	if testing.Short() {
		t.Skip("matches twenty thousand shots")
	}
	w := gen.New(1, gen.Config{Frames: 20_000, Perturb: gen.PerturbAll})
	var best time.Duration
	var pairs []match.Pair
	for i := range 3 {
		start := time.Now()
		kept, _ := match.Edited(w.Files, domain.DefaultPolicies().EditedDirs)
		cands, _ := match.Group(kept)
		pairs, _, _ = match.Match(w.Snapshot.Assets, cands)
		if d := time.Since(start); i == 0 || d < best {
			best = d
		}
	}
	if len(pairs) < len(w.Snapshot.Assets)/2 {
		t.Fatalf("%d pairs of %d assets; want at least half of them paired", len(pairs), len(w.Snapshot.Assets))
	}
	if best > matchScaleCeiling {
		t.Errorf("matching %d assets and %d files takes %v; want at most %v", len(w.Snapshot.Assets), len(w.Files), best, matchScaleCeiling)
	}
}
