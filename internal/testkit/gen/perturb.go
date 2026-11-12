package gen

import (
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Perturbation is a set of ways in which a shot is harder to pair than a
// plain one: it differs between Photos and the cloud, or it has more files
// or assets. Each shot meets each way of the set with a chance of one in
// eight; PerturbNFD meets an event, not a shot, with the same chance. A way
// that does not fit the shot leaves it as it is. A file beside the file of a
// shot has the size of the original and no metadata. Truth still pairs every
// file with the asset it comes from. The zero set gives a world without
// differences.
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
	// PerturbLive makes a photo a Live Photo: its asset gets a live video
	// resource named as the original but .MOV, and its file a file beside it
	// named as that file but .MOV or .mov.
	PerturbLive
	// PerturbRAWPair pairs a photo with a RAW: its asset gets a RAW resource
	// named as the original but .DNG, and its file a file beside it named as
	// that file but .DNG.
	PerturbRAWPair
	// PerturbSidecar gives the file of a shot a file beside it named as that
	// file but .AAE or .xmp, which only the cloud has.
	PerturbSidecar
	// PerturbDuplicate imports a shot into Photos twice: a second asset,
	// equal but for its key, comes with its own copies of the files of the
	// shot, named with " (1)" before the extension, as photosync numbers the
	// second file of one name.
	PerturbDuplicate
	// PerturbNFD gives an event a title with a composed letter, in NFC, and
	// its directory that title in NFD, as a Mac may write names.
	PerturbNFD
	// PerturbAll is every perturbation, those added later too.
	PerturbAll Perturbation = ^Perturbation(0)
)

func (w *World) perturb(on func(Perturbation) bool, intn func(int) int) {
	n := len(w.Files) - 1
	a, f := &w.Snapshot.Assets[len(w.Snapshot.Assets)-1], &w.Files[n]
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
	if !a.IsVideo && on(PerturbLive) {
		w.addBeside(a, w.Files[n].Path, []string{".MOV", ".mov"}[intn(2)], domain.ResourceLiveVideo)
	}
	if !a.IsVideo && on(PerturbRAWPair) {
		w.addBeside(a, w.Files[n].Path, ".DNG", domain.ResourceRAWPair)
	}
	if on(PerturbSidecar) {
		w.addBeside(a, w.Files[n].Path, []string{".AAE", ".xmp"}[intn(2)], "")
	}
	if on(PerturbDuplicate) {
		w.duplicate(*a, n)
	}
}

// A camera names the video of a Live Photo .MOV and photosync names it .mov,
// while Photos keeps the name that the camera gave. A file beside a photo has
// the size of the photo, below that of every video of the world, so that no
// video pairs with a Live video by name and size.
func (w *World) addBeside(a *domain.Asset, file, ext string, role domain.ResourceRole) {
	size := a.Resources[0].Size
	if role != "" {
		a.Resources = append(a.Resources, domain.Resource{Role: role, Filename: withExt(a.OriginalName, strings.ToUpper(ext)), Size: size})
	}
	p := withExt(file, ext)
	w.Files = append(w.Files, domain.RemoteEntry{Root: domain.LibraryPersonal, Path: p, Size: size})
	w.Truth[p] = a.Key
}

func withExt(name, ext string) string {
	return strings.TrimSuffix(name, path.Ext(name)) + ext
}

// A copy shares no memory with its original: a test that edits one asset or
// file in place must not edit another.
func (w *World) duplicate(a domain.Asset, from int) {
	offset := *a.TZOffset
	a.Key = domain.AssetKey{CloudAssetGUID: a.Key.CloudAssetGUID + "-1", UUID: a.Key.UUID + "-1"}
	a.TZOffset, a.Resources, a.AlbumUUIDs = &offset, slices.Clone(a.Resources), slices.Clone(a.AlbumUUIDs)
	w.Snapshot.Assets = append(w.Snapshot.Assets, a)
	for _, f := range w.Files[from:] {
		if f.Meta.TZOffset != nil {
			fileOffset := *f.Meta.TZOffset
			f.Meta.TZOffset = &fileOffset
		}
		f.Path = withExt(f.Path, " (1)"+path.Ext(f.Path))
		w.Files = append(w.Files, f)
		w.Truth[f.Path] = a.Key
	}
}
