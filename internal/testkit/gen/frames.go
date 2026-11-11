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
	for i := range cfg.Frames {
		w.addShot(intn, events[intn(len(events))], i)
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
// so the UTC date of a shot often differs from the day of its event.
func (w *World) addShot(intn func(int) int, e event, i int) {
	offset := 15 * 60 * (intn(105) - 48)
	taken := e.day.Add(time.Duration(intn(24*3600)-offset) * time.Second)
	video := intn(5) == 4
	ext, width, height, subsec := ".HEIC", 4032, 3024, fmt.Sprintf("%03d", intn(1000))
	size := int64(1_000_000 + intn(4_000_000))
	if video {
		ext, width, height, subsec, size = ".MOV", 1920, 1080, "", 10*size
	}
	name := fmt.Sprintf("IMG_%04d%s", i+1, ext)
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
