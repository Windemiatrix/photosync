package gen

import (
	"fmt"
	"path"
	"time"
)

// Perturbation is a set of ways in which a shot differs between Photos and
// the cloud. Each shot meets each way of the set with a chance of one in
// eight; a way that does not fit the shot leaves it as it is. Truth still
// pairs every file with the asset of its shot. The zero set gives a world
// without differences.
type Perturbation uint

// The perturbations.
const (
	// PerturbImportRename names the asset and its original IMG_ and six
	// digits with the extension of its file, a name that no file has.
	PerturbImportRename Perturbation = 1 << iota
	// PerturbRecompress makes the file smaller than the original.
	PerturbRecompress
	// PerturbZoneShift leaves the file no TZOffset and the wall clock of the
	// shot in Taken, as a camera that writes no time zone does; Taken then
	// differs from the asset by the offset of the shot, which may be zero.
	PerturbZoneShift
	// PerturbRoundSubSec leaves the file of a photo no SubSec and its Taken
	// rounded to the nearest second, a half up.
	PerturbRoundSubSec
	// PerturbNoDate leaves the file no Taken, TZOffset or SubSec, whatever
	// the other perturbations made of them.
	PerturbNoDate
	// PerturbBurst makes a shot a photo of the event, second and offset of
	// the photo made before it, with another SubSec, as in a burst.
	PerturbBurst
	// PerturbSameName gives a shot the name, but for the extension, that the
	// shot made before it had at first, when that shot is of another year, as
	// when the counter of a camera starts over.
	PerturbSameName
	// PerturbAll is every perturbation, those added later too.
	PerturbAll Perturbation = ^Perturbation(0)
)

func (w *World) perturb(on func(Perturbation) bool, intn func(int) int) {
	a, f := &w.Snapshot.Assets[len(w.Snapshot.Assets)-1], &w.Files[len(w.Files)-1]
	if on(PerturbImportRename) {
		// Files carry their numbers from 1 in four digits or more, so no file
		// has six digits that start with 0.
		a.OriginalName = fmt.Sprintf("IMG_%06d%s", intn(100_000), path.Ext(f.Path))
		a.Resources[0].Filename = a.OriginalName
	}
	if on(PerturbRecompress) {
		f.Size -= int64(1 + intn(500_000))
	}
	if on(PerturbZoneShift) {
		f.Meta.Taken, f.Meta.TZOffset = f.Meta.Taken.Add(time.Duration(*f.Meta.TZOffset)*time.Second), nil
	}
	if on(PerturbRoundSubSec) {
		// SubSec of a photo has three digits, so its strings order as numbers.
		if f.Meta.SubSec >= "500" {
			f.Meta.Taken = f.Meta.Taken.Add(time.Second)
		}
		f.Meta.SubSec = ""
	}
	if on(PerturbNoDate) {
		f.Meta.Taken, f.Meta.TZOffset, f.Meta.SubSec = time.Time{}, nil, ""
	}
}
