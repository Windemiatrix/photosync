package gen

import (
	"math/rand/v2"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// World is a photo library and the listing of its remote root, made from the
// same shots. Files holds files only; their paths name the directories.
// Truth maps the path of every file that comes from an asset, the file of
// its shot or a file beside it, to the key of that asset. The snapshot keeps
// the contract of domain.Asset and domain.Album: no IDs, Kind only on
// folders, references by UUID, titles in NFC, no hashes. Two events may
// share a date and a title, and so a directory.
type World struct {
	Snapshot domain.LibrarySnapshot
	Files    []domain.RemoteEntry
	Truth    map[string]domain.AssetKey
}

// Config is what a world is made of. Frames is the number of shots, each an
// asset in an event album and a file in the directory of that event; zero or
// less makes none. Up to 86400 shots, no two have one second of the day but
// those of a burst. Perturb is the set of perturbations that the shots show.
type Config struct {
	Frames  int
	Perturb Perturbation
}

// New returns the world of seed and cfg; the same seed and cfg give the same
// world.
func New(seed uint64, cfg Config) World {
	r := rand.New(rand.NewPCG(seed, 0)) //nolint:gosec // G404: a test world must be reproducible, not secret
	return build(r.IntN, cfg)
}

// Worlds returns the generator of worlds of cfg for rapid; unlike New, it
// lets rapid shrink a failing world.
func Worlds(cfg Config) *rapid.Generator[World] {
	return rapid.Custom(func(t *rapid.T) World {
		return build(func(n int) int { return rapid.IntRange(0, n-1).Draw(t, "n") }, cfg)
	})
}
