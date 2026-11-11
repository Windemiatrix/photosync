package gen_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/text/unicode/norm"
	"pgregory.net/rapid"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

var seeds = []uint64{1, 2, 3}

const bigWorld = 200

func TestNew(t *testing.T) {
	t.Parallel()

	cfg := gen.Config{Frames: bigWorld}
	for _, seed := range seeds {
		w := gen.New(seed, cfg)
		if err := check(w, bigWorld); err != nil {
			t.Errorf("seed %d, %d frames: %v", seed, bigWorld, err)
		}
		if !reflect.DeepEqual(w, gen.New(seed, cfg)) {
			t.Errorf("seed %d, %d frames gave two worlds", seed, bigWorld)
		}
	}
	if reflect.DeepEqual(gen.New(1, cfg), gen.New(2, cfg)) {
		t.Errorf("seeds 1 and 2 gave one world of %d frames", bigWorld)
	}
}

func TestNewWithoutShots(t *testing.T) {
	t.Parallel()

	for _, n := range []int{0, -4, -100} {
		if err := check(gen.New(1, gen.Config{Frames: n}), 0); err != nil {
			t.Errorf("seed 1, %d frames: %v", n, err)
		}
	}
}

func TestNewVaries(t *testing.T) {
	t.Parallel()

	var twoFolders, video, photo, west, east, twoTimes bool
	for _, seed := range seeds {
		w := gen.New(seed, gen.Config{Frames: bigWorld})
		folders := 0
		for _, al := range w.Snapshot.Albums {
			if al.Kind == domain.KindFolder {
				folders++
			}
		}
		twoFolders = twoFolders || folders >= 2
		taken := map[string]time.Time{}
		for _, a := range w.Snapshot.Assets {
			video = video || a.IsVideo
			photo = photo || !a.IsVideo
			if a.TZOffset != nil {
				west = west || *a.TZOffset < 0
				east = east || *a.TZOffset > 0
			}
			for _, id := range a.AlbumUUIDs {
				if first, ok := taken[id]; !ok {
					taken[id] = a.Taken
				} else if !first.Equal(a.Taken) {
					twoTimes = true
				}
			}
		}
	}
	var missing []string
	for _, c := range []struct {
		ok   bool
		what string
	}{
		{twoFolders, "two year folders in one world"},
		{video, "a video"},
		{photo, "a photo"},
		{west, "a negative offset"},
		{east, "a positive offset"},
		{twoTimes, "an event with two assets of different times"},
	} {
		if !c.ok {
			missing = append(missing, c.what)
		}
	}
	if len(missing) > 0 {
		t.Errorf("worlds of seeds %v and %d frames have no %s", seeds, bigWorld, strings.Join(missing, ", "))
	}
}

func TestWorlds(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		n := rapid.IntRange(0, 30).Draw(t, "frames")
		w := gen.Worlds(gen.Config{Frames: n}).Draw(t, "world")
		if err := check(w, n); err != nil {
			t.Fatal(err)
		}
	})
}

// check returns the first way in which w breaks the contract of a world of
// shots shots, or nil.
func check(w gen.World, shots int) error {
	assets, albums, files := w.Snapshot.Assets, w.Snapshot.Albums, w.Files
	if len(assets) != shots || len(files) != shots || len(w.Truth) != shots {
		return fmt.Errorf("%d assets, %d files, %d truths; want %d each", len(assets), len(files), len(w.Truth), shots)
	}

	uuids := map[string]bool{}
	albumOf := map[string]domain.Album{}
	for _, al := range albums {
		if al.UUID == "" || uuids[al.UUID] {
			return fmt.Errorf("album UUID %q is empty or repeated", al.UUID)
		}
		uuids[al.UUID] = true
		albumOf[al.UUID] = al
	}
	assetOf := map[domain.AssetKey]domain.Asset{}
	for _, a := range assets {
		if a.Key.CloudAssetGUID == "" || a.Key.UUID == "" || uuids[a.Key.UUID] {
			return fmt.Errorf("asset key %+v has an empty part or a repeated UUID", a.Key)
		}
		uuids[a.Key.UUID] = true
		assetOf[a.Key] = a
	}
	fileOf := map[string]domain.RemoteEntry{}
	for _, f := range files {
		if _, ok := fileOf[f.Path]; ok {
			return fmt.Errorf("path %q is repeated", f.Path)
		}
		fileOf[f.Path] = f
	}
	pathOf := map[domain.AssetKey]string{}
	for p, k := range w.Truth {
		if _, ok := pathOf[k]; ok {
			return fmt.Errorf("two paths of the truth name asset %+v", k)
		}
		pathOf[k] = p
	}

	folderTitles := map[string]bool{}
	for _, al := range albums {
		if al.ID != 0 || al.ParentID != 0 || al.Key != "" || al.KeySource != "" || !norm.NFC.IsNormalString(al.Title) {
			return fmt.Errorf("album %q has ID %d, ParentID %d, Key %q, KeySource %q, title %q; want zeros and an NFC title",
				al.UUID, al.ID, al.ParentID, al.Key, al.KeySource, al.Title)
		}
		switch al.Kind {
		case domain.KindFolder:
			if len(al.Title) != 4 || strings.Trim(al.Title, "0123456789") != "" || al.ParentUUID != "" ||
				len(al.FolderTitles) != 0 || !al.Start.IsZero() || folderTitles[al.Title] {
				return fmt.Errorf("folder %q has title %q, parent %q, folder titles %q, start %v; want a new year at the top, no start",
					al.UUID, al.Title, al.ParentUUID, al.FolderTitles, al.Start)
			}
			folderTitles[al.Title] = true
		case "":
			folder, ok := albumOf[al.ParentUUID]
			if !ok || folder.Kind != domain.KindFolder || len(al.FolderTitles) != 1 || al.FolderTitles[0] != folder.Title {
				return fmt.Errorf("event %q has parent %q and folder titles %q; want a folder of the snapshot and its title",
					al.UUID, al.ParentUUID, al.FolderTitles)
			}
			if len(al.Title) <= 6 || al.Title[2] != '/' || al.Title[5] != ' ' {
				return fmt.Errorf("event %q has title %q; want MM/DD and a word", al.UUID, al.Title)
			}
			if _, err := time.Parse("2006 01/02", folder.Title+" "+al.Title[:5]); err != nil {
				return fmt.Errorf("event %q has title %q in folder %q; want a date of that year", al.UUID, al.Title, folder.Title)
			}
		default:
			return fmt.Errorf("album %q has kind %q; want %q or none", al.UUID, al.Kind, domain.KindFolder)
		}
	}

	for _, f := range files {
		if f.Root != domain.LibraryPersonal || f.IsDir || f.ETag != "" || f.MD5 != "" || f.SHA256 != "" || !f.MTime.IsZero() {
			return fmt.Errorf("file %q has root %q, IsDir %t, ETag %q, MD5 %q, SHA256 %q, MTime %v; want %q and zeros",
				f.Path, f.Root, f.IsDir, f.ETag, f.MD5, f.SHA256, f.MTime, domain.LibraryPersonal)
		}
		k, ok := w.Truth[f.Path]
		if _, isAsset := assetOf[k]; !ok || !isAsset {
			return fmt.Errorf("file %q names no asset in the truth", f.Path)
		}
	}

	earliest := map[string]time.Time{}
	for _, a := range assets {
		if a.ID != 0 || a.TargetLibrary != "" || a.Memberships != nil || a.Library != domain.LibraryPersonal || a.DateSource != domain.DateICloud {
			return fmt.Errorf("asset %q has ID %d, TargetLibrary %q, Memberships %v, Library %q, DateSource %q; want zeros, %q, %q",
				a.Key.UUID, a.ID, a.TargetLibrary, a.Memberships, a.Library, a.DateSource, domain.LibraryPersonal, domain.DateICloud)
		}
		if len(a.Resources) != 1 {
			return fmt.Errorf("asset %q has %d resources; want 1", a.Key.UUID, len(a.Resources))
		}
		r := a.Resources[0]
		if r.Role != domain.ResourceOriginal || r.Filename != a.OriginalName || r.Size <= 0 || r.SHA256 != "" || r.MD5 != "" {
			return fmt.Errorf("asset %q named %q has resource %+v; want the original of that name, a size and no hashes",
				a.Key.UUID, a.OriginalName, r)
		}
		if len(a.AlbumUUIDs) != 1 {
			return fmt.Errorf("asset %q is in albums %q; want one", a.Key.UUID, a.AlbumUUIDs)
		}
		event, ok := albumOf[a.AlbumUUIDs[0]]
		if !ok || event.Kind != "" {
			return fmt.Errorf("asset %q is in album %q; want an event of the snapshot", a.Key.UUID, a.AlbumUUIDs[0])
		}
		if a.Taken.Location() != time.UTC || a.Taken.Nanosecond() != 0 {
			return fmt.Errorf("asset %q was taken at %v; want whole seconds in UTC", a.Key.UUID, a.Taken)
		}
		if a.TZOffset == nil {
			return fmt.Errorf("asset %q has no offset", a.Key.UUID)
		}
		if *a.TZOffset%900 != 0 || *a.TZOffset < -12*3600 || *a.TZOffset > 14*3600 {
			return fmt.Errorf("asset %q has offset %d s; want quarters of an hour from -12:00 to +14:00", a.Key.UUID, *a.TZOffset)
		}
		ext, subSecDigits := ".HEIC", 3
		if a.IsVideo {
			ext, subSecDigits = ".MOV", 0
		}
		if !strings.HasSuffix(a.OriginalName, ext) || len(a.SubSec) != subSecDigits ||
			strings.Trim(a.SubSec, "0123456789") != "" || a.Width <= 0 || a.Height <= 0 {
			return fmt.Errorf("asset %q (video %t) is named %q with SubSec %q and %dx%d; want %s, %d digits and a size",
				a.Key.UUID, a.IsVideo, a.OriginalName, a.SubSec, a.Width, a.Height, ext, subSecDigits)
		}
		folder := albumOf[event.ParentUUID].Title
		local := a.Taken.In(time.FixedZone("", *a.TZOffset)).Format("2006/01/02")
		if day := folder + "/" + event.Title[:5]; local != day {
			return fmt.Errorf("asset %q was taken on %s local time; want %s, the day of event %q", a.Key.UUID, local, day, event.UUID)
		}
		path := fmt.Sprintf("%s/%s-%s - %s/%s", folder, event.Title[0:2], event.Title[3:5], event.Title[6:], a.OriginalName)
		if pathOf[a.Key] != path {
			return fmt.Errorf("asset %q has path %q in the truth; want %q", a.Key.UUID, pathOf[a.Key], path)
		}
		f := fileOf[path]
		if f.Size != r.Size || !f.Meta.Taken.Equal(a.Taken) || f.Meta.TZOffset == nil || *f.Meta.TZOffset != *a.TZOffset ||
			f.Meta.TZOffset == a.TZOffset || f.Meta.SubSec != a.SubSec || f.Meta.Width != a.Width || f.Meta.Height != a.Height {
			return fmt.Errorf("file %q has size %d and %+v; want size %d and the metadata of asset %q under its own offset pointer",
				path, f.Size, f.Meta, r.Size, a.Key.UUID)
		}
		if first, seen := earliest[event.UUID]; !seen || a.Taken.Before(first) {
			earliest[event.UUID] = a.Taken
		}
	}

	for _, al := range albums {
		if al.Kind == "" && !al.Start.Equal(earliest[al.UUID]) {
			return fmt.Errorf("event %q starts at %v; want %v, its earliest asset", al.UUID, al.Start, earliest[al.UUID])
		}
	}
	return nil
}
