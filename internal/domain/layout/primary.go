package layout

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/dates"
)

// Primary returns the album whose directory in a remote root holds the one
// file of asset: the event closest to its local capture date, else the
// thematic album it joined first, else zero for the day folder of that date
// (dates.LocalDay). asset.Memberships are the albums it is in now, each with
// the run that first saw it there, this run for a new one; albums are the
// albums of the library with those IDs and the folder titles of a snapshot.
// Folders and trashed albums do not count. An event is dated by "MM/DD" or
// "MM-DD" at the start of its title, or the first of the month for "MM", in
// the year of its year folder, else by Start, and comes last without both.
// Ties go to the earlier FirstSeenRun, then to the path of titles without
// case, then to the lower ID. At the first placement others are the other
// events, closest first, for the multi_album policy, and an asset in no album
// goes into the only event whose title names its day; otherwise others is
// empty.
func Primary(asset domain.Asset, albums []domain.Album, firstPlacement bool) (album domain.AlbumID, others []domain.AlbumID) {
	year, month, day := dates.LocalDay(asset.Taken, asset.TZOffset)
	shot := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	var events, thematic []candidate
	for _, a := range albums {
		i := slices.IndexFunc(asset.Memberships, func(m domain.Membership) bool { return m.AlbumID == a.ID })
		if i < 0 || a.InTrash {
			continue
		}
		c := candidate{album: a, run: asset.Memberships[i].FirstSeenRun}
		switch Classify(a) {
		case domain.KindEvent:
			date, _ := eventDay(a)
			c.dist = date.Sub(shot).Abs()
			events = append(events, c)
		case domain.KindThematic:
			thematic = append(thematic, c)
		}
	}
	switch {
	case len(events) > 0:
		slices.SortFunc(events, compare)
		if firstPlacement {
			for _, c := range events[1:] {
				others = append(others, c.album.ID)
			}
		}
		return events[0].album.ID, others
	case len(thematic) > 0:
		return slices.MinFunc(thematic, compare).album.ID, nil
	case firstPlacement:
		return onlyEvent(shot, albums), nil
	}
	return 0, nil
}

func onlyEvent(day time.Time, albums []domain.Album) domain.AlbumID {
	var found []domain.AlbumID
	for _, a := range albums {
		if a.InTrash || Classify(a) != domain.KindEvent {
			continue
		}
		if date, named := eventDay(a); named && date.Equal(day) {
			found = append(found, a.ID)
		}
	}
	if len(found) != 1 {
		return 0
	}
	return found[0]
}

type candidate struct {
	album domain.Album
	run   string
	dist  time.Duration
}

// Run IDs sort by time as strings; the ID makes the order total.
func compare(x, y candidate) int {
	return cmp.Or(
		cmp.Compare(x.dist, y.dist),
		cmp.Compare(x.run, y.run),
		cmp.Compare(fullPath(x.album), fullPath(y.album)),
		cmp.Compare(x.album.ID, y.album.ID),
	)
}

func fullPath(a domain.Album) string {
	return strings.ToLower(strings.Join(slices.Concat(a.FolderTitles, []string{a.Title}), "/"))
}

// The year is the year folder's, not the shot's, so a shot of December 31
// is one day from "01/01" in the next year's folder. Neither a month nor
// Start names the day of a folder in the cloud.
func eventDay(a domain.Album) (date time.Time, named bool) {
	for _, form := range []string{"01/02", "01-02", "01"} {
		n := len(form)
		if len(a.Title) < n || (len(a.Title) > n && a.Title[n] != ' ') {
			continue
		}
		if t, err := time.Parse("2006 "+form, a.FolderTitles[0]+" "+a.Title[:n]); err == nil {
			return t, form != "01"
		}
	}
	return a.Start.Truncate(24 * time.Hour), false
}
