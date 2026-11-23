package layout

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

// Events are the albums of a run as Primary takes them, read once for all
// its assets: an asset looks up its own albums instead of reading every
// album of the run.
type Events struct {
	albums map[domain.AlbumID]event
	days   map[time.Time][]domain.AlbumID
}

type event struct {
	day   time.Time
	dated bool
	path  string
}

// ReadEvents returns the Events of albums, the albums of a run, each with the
// Kind that Classify gives it: each album outside Recently Deleted other than
// a folder, and, for an event, the day of the date that p.Templates.Photos
// reads from its folder titles and title, if it reads one.
func ReadEvents(albums []domain.Album, p domain.Policies) Events {
	photos, _ := parse(p.Templates)
	out := Events{albums: map[domain.AlbumID]event{}, days: map[time.Time][]domain.AlbumID{}}
	for _, a := range albums {
		if a.Kind == domain.KindFolder || a.InTrash {
			continue
		}
		titles := slices.Concat(a.FolderTitles, []string{a.Title})
		e := event{path: strings.ToLower(strings.Join(titles, "/"))}
		if a.Kind == domain.KindEvent {
			f, _ := photos.Match(titles)
			e.day, e.dated = f.Date()
		}
		if e.dated {
			out.days[e.day] = append(out.days[e.day], a.ID)
		}
		out.albums[a.ID] = e
	}
	return out
}

// Primary returns the album whose directory in a remote root holds the one
// file of asset: the dated event closest to its local capture date
// (dates.LocalDay), else zero. asset.Memberships are the albums it is in
// now, each with the run that first saw it there, this run for a new one;
// events are those of the albums of the run. Ties go to the earlier
// FirstSeenRun, then to the path of titles without case, then to the lower
// ID. At the first placement others are the other dated events, closest
// first, for the multi_album policy, and an asset in none of the albums of
// events goes into the only event of its day; otherwise others is empty.
func Primary(asset domain.Asset, events Events, firstPlacement bool) (album domain.AlbumID, others []domain.AlbumID) {
	year, month, day := dates.LocalDay(asset.Taken, asset.TZOffset)
	shot := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	var found []candidate
	inAlbum := false
	for _, m := range asset.Memberships {
		e, ok := events.albums[m.AlbumID]
		inAlbum = inAlbum || ok
		if e.dated {
			found = append(found, candidate{id: m.AlbumID, run: m.FirstSeenRun, path: e.path, dist: e.day.Sub(shot).Abs()})
		}
	}
	switch {
	case len(found) > 0:
		slices.SortFunc(found, compare)
		if firstPlacement {
			for _, c := range found[1:] {
				others = append(others, c.id)
			}
		}
		return found[0].id, others
	case firstPlacement && !inAlbum && len(events.days[shot]) == 1:
		return events.days[shot][0], nil
	}
	return 0, nil
}

type candidate struct {
	id   domain.AlbumID
	run  string
	path string
	dist time.Duration
}

// Run IDs sort by time as strings; the ID makes the order total.
func compare(x, y candidate) int {
	return cmp.Or(
		cmp.Compare(x.dist, y.dist),
		cmp.Compare(x.run, y.run),
		cmp.Compare(x.path, y.path),
		cmp.Compare(x.id, y.id),
	)
}
