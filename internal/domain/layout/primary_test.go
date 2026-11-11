package layout_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// covers: R-LAYOUT-02
// covers: R-LAYOUT-03
// covers: R-LAYOUT-04
// covers: R-LAYOUT-05
// covers: R-LAYOUT-09
func TestPrimary(t *testing.T) {
	t.Parallel()

	const r1, r2 = "2026-01-01T000000Z", "2026-02-01T000000Z"
	noon := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }
	east := func(seconds int) *int { return &seconds }
	in := func(id domain.AlbumID, run string) domain.Membership {
		return domain.Membership{AlbumID: id, FirstSeenRun: run}
	}
	shot := func(taken time.Time, offset *int, memberships ...domain.Membership) domain.Asset {
		return domain.Asset{Taken: taken, TZOffset: offset, Memberships: memberships}
	}

	holidays := domain.Album{ID: 1, FolderTitles: []string{"2024"}, Title: "05/01 Holidays"}
	picnic := domain.Album{ID: 2, FolderTitles: []string{"2024"}, Title: "05/03 Picnic"}
	city := domain.Album{ID: 3, FolderTitles: []string{"2024", "Weekends"}, Title: "05/10 City"}
	pets := domain.Album{ID: 4, Title: "Pets"}
	recipes := domain.Album{ID: 5, Title: "Recipes"}
	zoo := domain.Album{ID: 6, Title: "Zoo"}
	apple := domain.Album{ID: 7, Title: "apple"}
	zebra := domain.Album{ID: 8, FolderTitles: []string{"Crafts"}, Title: "Zebra"}
	kites := domain.Album{ID: 9, Title: "Kites"}
	paint1 := domain.Album{ID: 10, FolderTitles: []string{"Crafts"}, Title: "Painting"}
	paint2 := domain.Album{ID: 11, FolderTitles: []string{"Crafts"}, Title: "Painting"}
	newYear := domain.Album{ID: 12, FolderTitles: []string{"2024"}, Title: "01/01 New Year"}
	party := domain.Album{ID: 13, FolderTitles: []string{"2023"}, Title: "12/20 Party"}
	eve := domain.Album{ID: 14, FolderTitles: []string{"2023"}, Title: "12/31 Eve"}
	summer := domain.Album{ID: 15, FolderTitles: []string{"2019"}, Title: "Summer", Start: time.Date(2019, 7, 5, 8, 0, 0, 0, time.UTC)}
	bestOf := domain.Album{ID: 16, FolderTitles: []string{"2019"}, Title: "12/31 Best of"}
	concert := domain.Album{ID: 17, FolderTitles: []string{"2019"}, Title: "07/15 Concert"}
	camp := domain.Album{ID: 18, FolderTitles: []string{"2019"}, Title: "Camp"}
	garden := domain.Album{ID: 19, FolderTitles: []string{"2019"}, Title: "06 - Garden"}
	july := domain.Album{ID: 20, FolderTitles: []string{"2019"}, Title: "07"}
	juneParty := domain.Album{ID: 21, FolderTitles: []string{"2019"}, Title: "06/10 Party"}
	ferry := domain.Album{ID: 22, FolderTitles: []string{"2019"}, Title: "09-24 Ferry"}
	september := domain.Album{ID: 23, FolderTitles: []string{"2019"}, Title: "09/30 September"}
	glued := domain.Album{ID: 24, FolderTitles: []string{"2024"}, Title: "05/01Holidays"}
	walk := domain.Album{ID: 25, FolderTitles: []string{"2024"}, Title: "05/20 Walk"}
	leap := domain.Album{ID: 26, FolderTitles: []string{"2023"}, Title: "02/29 Leap", Start: time.Date(2023, 3, 13, 10, 0, 0, 0, time.UTC)}
	march := domain.Album{ID: 27, FolderTitles: []string{"2023"}, Title: "03/01 March"}
	old := domain.Album{ID: 28, FolderTitles: []string{"2024"}, Title: "05/02 Old", InTrash: true}
	live := domain.Album{ID: 29, FolderTitles: []string{"2024"}, Title: "05/02 Live"}
	lake := domain.Album{ID: 30, FolderTitles: []string{"2024", "Weekends"}, Title: "05/03 Lake"}
	selected := domain.Album{ID: 31, FolderTitles: []string{"2024"}, Title: "05/01 Holidays / Selected"}
	market := domain.Album{ID: 32, FolderTitles: []string{"2024"}, Title: "05/03 Market", Kind: domain.KindFolder}
	notes := domain.Album{ID: 33, Title: "05/03 Notes"}

	cases := []struct {
		rule, name string
		asset      domain.Asset
		albums     []domain.Album
		first      bool
		want       domain.AlbumID
		others     []domain.AlbumID
	}{
		{"R-LAYOUT-02", "no album: the day folder", shot(noon(2024, 5, 20), nil), []domain.Album{holidays, pets}, false, 0, nil},
		{"R-LAYOUT-02", "a thematic album", shot(noon(2024, 5, 20), nil, in(4, r1)), []domain.Album{pets, holidays}, false, 4, nil},
		{"R-LAYOUT-02", "an event before a thematic album", shot(noon(2024, 5, 1), nil, in(4, r1), in(1, r2)), []domain.Album{pets, holidays}, true, 1, nil},
		{"R-LAYOUT-02", "a folder is not an album", shot(noon(2024, 5, 3), nil, in(32, r1)), []domain.Album{market}, false, 0, nil},
		{"R-LAYOUT-02", "a trashed album does not count", shot(noon(2024, 5, 2), nil, in(28, r1), in(4, r1)), []domain.Album{old, pets}, false, 4, nil},
		{"R-LAYOUT-02", "a membership without its album", shot(noon(2024, 5, 20), nil, in(99, r1)), []domain.Album{pets}, false, 0, nil},

		{"R-LAYOUT-03", "a thematic album joined in a later run does not take over", shot(noon(2024, 5, 20), nil, in(4, r2), in(5, r1)), []domain.Album{pets, recipes}, false, 5, nil},
		{"R-LAYOUT-03", "the same run: first by path", shot(noon(2024, 5, 20), nil, in(5, r1), in(4, r1)), []domain.Album{recipes, pets}, false, 4, nil},
		{"R-LAYOUT-03", "the path ignores case", shot(noon(2024, 5, 20), nil, in(6, r1), in(7, r1)), []domain.Album{zoo, apple}, false, 7, nil},
		{"R-LAYOUT-03", "the path starts with the folders", shot(noon(2024, 5, 20), nil, in(9, r1), in(8, r1)), []domain.Album{kites, zebra}, false, 8, nil},
		{"R-LAYOUT-03", "equal paths: the lower ID", shot(noon(2024, 5, 20), nil, in(11, r1), in(10, r1)), []domain.Album{paint2, paint1}, false, 10, nil},

		{"R-LAYOUT-04", "the closest event at the first placement", shot(noon(2024, 5, 9), nil, in(1, r1), in(2, r1), in(3, r1)), []domain.Album{holidays, picnic, city}, true, 3, []domain.AlbumID{2, 1}},
		{"R-LAYOUT-04", "the closest event later, without others", shot(noon(2024, 5, 9), nil, in(1, r1), in(2, r1), in(3, r1)), []domain.Album{holidays, picnic, city}, false, 3, nil},
		{"R-LAYOUT-04", "a closer event beats an earlier run", shot(noon(2024, 5, 9), nil, in(3, r2), in(2, r1)), []domain.Album{picnic, city}, true, 3, []domain.AlbumID{2}},
		{"R-LAYOUT-04", "equally close events: first by path", shot(noon(2024, 5, 2), nil, in(2, r1), in(1, r1)), []domain.Album{picnic, holidays}, true, 1, []domain.AlbumID{2}},
		{"R-LAYOUT-04", "equally close events: the one joined first", shot(noon(2024, 5, 2), nil, in(1, r2), in(2, r1)), []domain.Album{holidays, picnic}, true, 2, []domain.AlbumID{1}},
		{"R-LAYOUT-04", "the year of an event is its folder's", shot(time.Date(2023, 12, 31, 23, 50, 0, 0, time.UTC), nil, in(12, r1), in(13, r1)), []domain.Album{newYear, party}, true, 12, []domain.AlbumID{13}},
		{"R-LAYOUT-04", "the capture date is local", shot(time.Date(2024, 1, 1, 4, 50, 0, 0, time.UTC), east(-5*3600), in(12, r1), in(14, r1)), []domain.Album{newYear, eve}, true, 14, []domain.AlbumID{12}},
		{"R-LAYOUT-04", "a trashed event is not among the others", shot(noon(2024, 5, 2), nil, in(1, r1), in(28, r1)), []domain.Album{holidays, old}, true, 1, nil},

		{"R-LAYOUT-09", "an undated event takes its start", shot(noon(2019, 7, 10), nil, in(15, r1), in(16, r1)), []domain.Album{summer, bestOf}, true, 15, []domain.AlbumID{16}},
		{"R-LAYOUT-09", "a start counts by its day", shot(noon(2019, 7, 10), nil, in(15, r1), in(17, r1)), []domain.Album{summer, concert}, true, 17, []domain.AlbumID{15}},
		{"R-LAYOUT-09", "an undated event without a start comes last", shot(noon(2019, 7, 10), nil, in(18, r1), in(16, r1)), []domain.Album{camp, bestOf}, true, 16, []domain.AlbumID{18}},
		{"R-LAYOUT-09", "a month stands for its first day", shot(noon(2019, 6, 3), nil, in(19, r1), in(21, r1)), []domain.Album{garden, juneParty}, true, 19, []domain.AlbumID{21}},
		{"R-LAYOUT-09", "a bare month", shot(noon(2019, 7, 2), nil, in(20, r1), in(21, r1)), []domain.Album{july, juneParty}, true, 20, []domain.AlbumID{21}},
		{"R-LAYOUT-09", "an MM-DD title names its day", shot(noon(2019, 9, 25), nil, in(22, r1), in(23, r1)), []domain.Album{ferry, september}, true, 22, []domain.AlbumID{23}},
		{"R-LAYOUT-09", "a date ends at a space", shot(noon(2024, 5, 1), nil, in(24, r1), in(25, r1)), []domain.Album{glued, walk}, true, 25, []domain.AlbumID{24}},
		{"R-LAYOUT-09", "a day that does not exist takes the start", shot(noon(2023, 3, 14), nil, in(26, r1), in(27, r1)), []domain.Album{leap, march}, true, 26, []domain.AlbumID{27}},

		{"R-LAYOUT-05", "the only event of the day at the first placement", shot(noon(2024, 5, 3), nil), []domain.Album{holidays, picnic, city, notes}, true, 2, nil},
		{"R-LAYOUT-05", "later the day folder stays", shot(noon(2024, 5, 3), nil), []domain.Album{holidays, picnic, city, notes}, false, 0, nil},
		{"R-LAYOUT-05", "two events of the day", shot(noon(2024, 5, 3), nil), []domain.Album{picnic, lake}, true, 0, nil},
		{"R-LAYOUT-05", "a nested part is a second event of the day", shot(noon(2024, 5, 1), nil), []domain.Album{holidays, selected}, true, 0, nil},
		{"R-LAYOUT-05", "the local day picks the event", shot(time.Date(2024, 5, 4, 3, 0, 0, 0, time.UTC), east(-5*3600)), []domain.Album{picnic}, true, 2, nil},
		{"R-LAYOUT-05", "a month names no day", shot(noon(2019, 6, 1), nil), []domain.Album{garden}, true, 0, nil},
		{"R-LAYOUT-05", "a start names no day", shot(noon(2019, 7, 5), nil), []domain.Album{summer}, true, 0, nil},
		{"R-LAYOUT-05", "a trashed event of the day does not count", shot(noon(2024, 5, 2), nil), []domain.Album{old, live}, true, 29, nil},
		{"R-LAYOUT-05", "a thematic album keeps the asset out of the event of its day", shot(noon(2024, 5, 3), nil, in(4, r1)), []domain.Album{picnic, pets}, true, 4, nil},
		{"R-LAYOUT-05", "an event of the day in another year", shot(noon(2023, 5, 3), nil), []domain.Album{picnic}, true, 0, nil},
		{"R-LAYOUT-05", "a folder named by the day is not an event", shot(noon(2024, 5, 3), nil), []domain.Album{market, notes}, true, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[c.rule]; !ok {
				t.Errorf("%s is not in domain.Rules", c.rule)
			}
			check := func(asset domain.Asset, albums []domain.Album) {
				got, others := layout.Primary(asset, albums, c.first)
				if got != c.want || !slices.Equal(others, c.others) {
					t.Errorf("Primary = %d, %v; want %d, %v", got, others, c.want, c.others)
				}
			}
			check(c.asset, c.albums)
			// The placement must not depend on the order of albums or memberships.
			reversed := c.asset
			reversed.Memberships = slices.Clone(c.asset.Memberships)
			slices.Reverse(reversed.Memberships)
			albums := slices.Clone(c.albums)
			slices.Reverse(albums)
			check(reversed, albums)
		})
	}
}
