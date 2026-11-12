package gen

import (
	"fmt"
	"strconv"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

var words = []string{"Holidays", "Trip", "Отпуск"}

type event struct {
	album int
	dir   string
	day   time.Time
}

func build(intn func(int) int, cfg Config) World {
	w := World{Truth: map[string]domain.AssetKey{}}
	folders := map[int]string{}
	events := make([]event, 1+intn(1+max(cfg.Frames, 0)/4))
	for i := range events {
		events[i] = w.addEvent(intn, folders, i)
	}
	// A draw of 7 rather than 0, so that rapid shrinks a world toward no
	// perturbations. The mask goes first, so that a kind outside the set draws
	// nothing and only the coins of the set reach a rapid counterexample.
	on := func(p Perturbation) bool { return cfg.Perturb&p != 0 && intn(8) == 7 }
	var e event
	for i := range cfg.Frames {
		prev := e
		burst := i > 0 && on(PerturbBurst)
		if !burst {
			e = events[intn(len(events))]
		}
		same := i > 0 && e.day.Year() != prev.day.Year() && on(PerturbSameName)
		w.addShot(intn, e, i, cfg.Frames, burst, same)
		w.perturb(on, intn)
	}
	return w
}

func (w *World) addEvent(intn func(int) int, folders map[int]string, i int) event {
	year, month, day := 2019+intn(6), 1+intn(12), 1+intn(28)
	word := words[intn(len(words))]
	folder, ok := folders[year]
	if !ok {
		folder = fmt.Sprintf("folder-%d", year)
		folders[year] = folder
		w.Snapshot.Albums = append(w.Snapshot.Albums, domain.Album{UUID: folder, Kind: domain.KindFolder, Title: strconv.Itoa(year)})
	}
	w.Snapshot.Albums = append(w.Snapshot.Albums, domain.Album{
		UUID:         fmt.Sprintf("album-%d", i),
		Title:        fmt.Sprintf("%02d/%02d %s", month, day, word),
		ParentUUID:   folder,
		FolderTitles: []string{strconv.Itoa(year)},
	})
	return event{
		album: len(w.Snapshot.Albums) - 1,
		dir:   fmt.Sprintf("%d/%02d-%02d - %s", year, month, day, word),
		day:   time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC),
	}
}

// Offsets span the real time zones, -12:00 to +14:00 by quarters of an hour,
// so the UTC date of a shot often differs from the day of its event. The
// second of the day is i modulo shots, so that only a burst gives two shots
// one second, until there are more shots than seconds in a day.
func (w *World) addShot(intn func(int) int, e event, i, shots int, burst, sameName bool) {
	offset := 15 * 60 * (intn(105) - 48)
	second := (intn(max(24*3600/shots, 1))*shots + i) % (24 * 3600)
	taken := e.day.Add(time.Duration(second-offset) * time.Second)
	video := intn(5) == 4
	ext, width, height, subsec := ".HEIC", 4032, 3024, fmt.Sprintf("%03d", intn(1000))
	size := int64(1_000_000 + intn(4_000_000))
	if burst {
		// A video, or a photo of SubSec 999, leaves no greater SubSec in its second.
		p := w.Snapshot.Assets[i-1]
		if s, err := strconv.Atoi(p.SubSec); err == nil && s < 999 {
			offset, taken, video, subsec = *p.TZOffset, p.Taken, false, fmt.Sprintf("%03d", s+1+intn(999-s))
		}
	}
	if video {
		ext, width, height, subsec, size = ".MOV", 1920, 1080, "", 10*size
	}
	name := fmt.Sprintf("IMG_%04d%s", i+1, ext)
	if sameName {
		// Shots are numbered from 1, so the shot before this one had number i.
		name = fmt.Sprintf("IMG_%04d%s", i, ext)
	}
	key := domain.AssetKey{CloudAssetGUID: fmt.Sprintf("guid-%d", i), UUID: fmt.Sprintf("uuid-%d", i)}
	album := &w.Snapshot.Albums[e.album]
	w.Snapshot.Assets = append(w.Snapshot.Assets, domain.Asset{
		Key: key, Library: domain.LibraryPersonal, OriginalName: name, DateSource: domain.DateICloud,
		Taken: taken, TZOffset: &offset, SubSec: subsec, Width: width, Height: height, IsVideo: video,
		Resources:  []domain.Resource{{Role: domain.ResourceOriginal, Filename: name, Size: size}},
		AlbumUUIDs: []string{album.UUID},
	})
	fileOffset := offset
	path := e.dir + "/" + name
	w.Files = append(w.Files, domain.RemoteEntry{
		Root: domain.LibraryPersonal, Path: path, Size: size,
		Meta: domain.MediaMeta{Taken: taken, TZOffset: &fileOffset, SubSec: subsec, Width: width, Height: height},
	})
	w.Truth[path] = key
	if album.Start.IsZero() || taken.Before(album.Start) {
		album.Start = taken
	}
}
