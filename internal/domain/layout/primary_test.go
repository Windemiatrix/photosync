package layout_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

func tpl(photos, cloud []string, names string) domain.Policies {
	p := domain.DefaultPolicies()
	p.Templates = domain.Templates{Photos: photos, Cloud: cloud, Names: names}
	return p
}

// covers: R-LAYOUT-02
// covers: R-LAYOUT-03
// covers: R-LAYOUT-04
// covers: R-LAYOUT-05
func TestPrimary(t *testing.T) {
	t.Parallel()

	const r1, r2 = "2026-01-01T000000Z", "2026-02-01T000000Z"
	std := domain.DefaultPolicies()
	byDay := tpl([]string{"{DD}", "{MM}", "{YYYY} {Description}"}, std.Templates.Cloud, "")
	flat := tpl([]string{"{YYYY}-{MM}-{DD} {Description}"}, []string{"{YYYY}", "{MM}-{DD} - {Description}"}, "")
	named := tpl([]string{"{YYYY}", "{DD} {Month} {Description}"}, std.Templates.Cloud, "ru")
	noon := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 12, 0, 0, 0, time.UTC) }
	east := func(seconds int) *int { return &seconds }
	in := func(id domain.AlbumID, run string) domain.Membership {
		return domain.Membership{AlbumID: id, FirstSeenRun: run}
	}
	shot := func(taken time.Time, offset *int, memberships ...domain.Membership) domain.Asset {
		return domain.Asset{Taken: taken, TZOffset: offset, Memberships: memberships}
	}
	event := func(id domain.AlbumID, folders []string, title string) domain.Album {
		return domain.Album{ID: id, Kind: domain.KindEvent, FolderTitles: folders, Title: title}
	}
	thematic := func(id domain.AlbumID, title string) domain.Album {
		return domain.Album{ID: id, Kind: domain.KindThematic, Title: title}
	}
	y2019, y2023, y2024 := []string{"2019"}, []string{"2023"}, []string{"2024"}

	holidays := event(1, y2024, "05/01 Holidays")
	picnic := event(2, y2024, "05/03 Picnic")
	city := event(3, y2024, "05/10 City")
	pets := thematic(4, "Pets")
	recipes := thematic(5, "Recipes")
	zoo := event(6, y2024, "05/20 Zoo")
	apple := event(7, y2024, "05/20 apple")
	zooLower := event(8, y2024, "05/20 zoo")
	newYear := event(12, y2024, "01/01 New Year")
	party := event(13, y2023, "12/20 Party")
	eve := event(14, y2023, "12/31 Eve")
	summer := event(15, y2019, "Summer")
	summer.Start = time.Date(2019, 7, 5, 8, 0, 0, 0, time.UTC)
	concert := event(17, y2019, "07/15 Concert")
	old := event(28, y2024, "05/02 Old")
	old.InTrash = true
	live := event(29, y2024, "05/02 Live")
	lake := event(30, y2024, "05/03 Lake")
	selected := event(31, y2024, "05/01 Holidays / Selected")
	market := domain.Album{ID: 32, Kind: domain.KindFolder, FolderTitles: y2024, Title: "05/03 Market"}
	notes := domain.Album{ID: 33, Kind: domain.KindThematic, FolderTitles: y2024, Title: "05/03 Notes"}
	dayFirst := event(40, []string{"01", "05"}, "2024 A")
	dayThird := event(41, []string{"03", "05"}, "2024 B")
	flatA := event(42, nil, "2024-05-01 A")
	flatB := event(43, nil, "2024-05-04 B")
	mayDay := event(44, y2024, "01 Май x")

	cases := []struct {
		rule, name string
		p          domain.Policies
		asset      domain.Asset
		albums     []domain.Album
		first      bool
		want       domain.AlbumID
		others     []domain.AlbumID
	}{
		{"R-LAYOUT-02", "no album: zero", std, shot(noon(2024, 5, 20), nil), []domain.Album{holidays, pets}, false, 0, nil},
		{"R-LAYOUT-02", "a thematic album: zero", std, shot(noon(2024, 5, 20), nil, in(4, r1)), []domain.Album{pets, holidays}, false, 0, nil},
		{"R-LAYOUT-02", "an event before a thematic album", std, shot(noon(2024, 5, 1), nil, in(4, r1), in(1, r2)), []domain.Album{pets, holidays}, true, 1, nil},
		{"R-LAYOUT-02", "a folder is not an album", std, shot(noon(2024, 5, 3), nil, in(32, r1)), []domain.Album{market}, false, 0, nil},
		{"R-LAYOUT-02", "a trashed event does not count", std, shot(noon(2024, 5, 2), nil, in(28, r1), in(4, r1)), []domain.Album{old, pets}, false, 0, nil},
		{"R-LAYOUT-02", "a membership without its album", std, shot(noon(2024, 5, 20), nil, in(99, r1)), []domain.Album{pets}, false, 0, nil},
		{"R-LAYOUT-02", "a thematic album that the template reads is no candidate", std, shot(noon(2024, 5, 3), nil, in(33, r1), in(1, r1)), []domain.Album{holidays, notes}, true, 1, nil},

		{"R-LAYOUT-03", "thematic albums give no placement at the first placement", std, shot(noon(2024, 5, 20), nil, in(4, r1), in(5, r1)), []domain.Album{pets, recipes}, true, 0, nil},

		{"R-LAYOUT-04", "the closest event at the first placement", std, shot(noon(2024, 5, 9), nil, in(1, r1), in(2, r1), in(3, r1)), []domain.Album{holidays, picnic, city}, true, 3, []domain.AlbumID{2, 1}},
		{"R-LAYOUT-04", "the closest event later, without others", std, shot(noon(2024, 5, 9), nil, in(1, r1), in(2, r1), in(3, r1)), []domain.Album{holidays, picnic, city}, false, 3, nil},
		{"R-LAYOUT-04", "a closer event beats an earlier run", std, shot(noon(2024, 5, 9), nil, in(3, r2), in(2, r1)), []domain.Album{picnic, city}, true, 3, []domain.AlbumID{2}},
		{"R-LAYOUT-04", "equally close events: first by path", std, shot(noon(2024, 5, 2), nil, in(2, r1), in(1, r1)), []domain.Album{picnic, holidays}, true, 1, []domain.AlbumID{2}},
		{"R-LAYOUT-04", "equally close events: the one joined first", std, shot(noon(2024, 5, 2), nil, in(1, r2), in(2, r1)), []domain.Album{holidays, picnic}, true, 2, []domain.AlbumID{1}},
		{"R-LAYOUT-04", "the path ignores case", std, shot(noon(2024, 5, 20), nil, in(6, r1), in(7, r1)), []domain.Album{zoo, apple}, true, 7, []domain.AlbumID{6}},
		{"R-LAYOUT-04", "paths equal but for case: the lower ID", std, shot(noon(2024, 5, 20), nil, in(6, r1), in(8, r1)), []domain.Album{zoo, zooLower}, true, 6, []domain.AlbumID{8}},
		{"R-LAYOUT-04", "the year of an event is its path's", std, shot(time.Date(2023, 12, 31, 23, 50, 0, 0, time.UTC), nil, in(12, r1), in(13, r1)), []domain.Album{newYear, party}, true, 12, []domain.AlbumID{13}},
		{"R-LAYOUT-04", "the capture date is local", std, shot(time.Date(2024, 1, 1, 4, 50, 0, 0, time.UTC), east(-5*3600), in(12, r1), in(14, r1)), []domain.Album{newYear, eve}, true, 14, []domain.AlbumID{12}},
		{"R-LAYOUT-04", "a trashed event is not among the others", std, shot(noon(2024, 5, 2), nil, in(1, r1), in(28, r1)), []domain.Album{holidays, old}, true, 1, nil},
		{"R-LAYOUT-04", "an undated event is no candidate", std, shot(noon(2019, 7, 10), nil, in(15, r1), in(17, r1)), []domain.Album{summer, concert}, true, 17, nil},
		{"R-LAYOUT-04", "an event is dated by the template", byDay, shot(noon(2024, 5, 3), nil, in(40, r1), in(41, r1)), []domain.Album{dayFirst, dayThird}, true, 41, []domain.AlbumID{40}},
		{"R-LAYOUT-04", "a template without folder levels", flat, shot(noon(2024, 5, 2), nil, in(42, r1), in(43, r1)), []domain.Album{flatA, flatB}, true, 42, []domain.AlbumID{43}},
		{"R-LAYOUT-04", "a template that spells the month by name", named, shot(noon(2024, 5, 1), nil, in(44, r1)), []domain.Album{mayDay}, true, 44, nil},

		{"R-LAYOUT-05", "the only event of the day at the first placement", std, shot(noon(2024, 5, 3), nil), []domain.Album{holidays, picnic, city}, true, 2, nil},
		{"R-LAYOUT-05", "later, no event of the day", std, shot(noon(2024, 5, 3), nil), []domain.Album{holidays, picnic, city}, false, 0, nil},
		{"R-LAYOUT-05", "two events of the day", std, shot(noon(2024, 5, 3), nil), []domain.Album{picnic, lake}, true, 0, nil},
		{"R-LAYOUT-05", "a second event of the day by its description", std, shot(noon(2024, 5, 1), nil), []domain.Album{holidays, selected}, true, 0, nil},
		{"R-LAYOUT-05", "the local day picks the event", std, shot(time.Date(2024, 5, 4, 3, 0, 0, 0, time.UTC), east(-5*3600)), []domain.Album{picnic}, true, 2, nil},
		{"R-LAYOUT-05", "a trashed event of the day does not count", std, shot(noon(2024, 5, 2), nil), []domain.Album{old, live}, true, 29, nil},
		{"R-LAYOUT-05", "a thematic album keeps the asset out of the event of its day", std, shot(noon(2024, 5, 3), nil, in(4, r1)), []domain.Album{picnic, pets}, true, 0, nil},
		{"R-LAYOUT-05", "an event of the day in another year", std, shot(noon(2023, 5, 3), nil), []domain.Album{picnic}, true, 0, nil},
		{"R-LAYOUT-05", "a folder named by the day is not an event", std, shot(noon(2024, 5, 3), nil), []domain.Album{market, notes}, true, 0, nil},
		{"R-LAYOUT-05", "a thematic album that the template reads is not the event of its day", std, shot(noon(2024, 5, 3), nil), []domain.Album{notes}, true, 0, nil},
		{"R-LAYOUT-05", "a folder keeps no asset out of the event of its day", std, shot(noon(2024, 5, 3), nil, in(32, r1)), []domain.Album{market, picnic}, true, 2, nil},
		{"R-LAYOUT-05", "an undated event names no day", std, shot(noon(2019, 7, 5), nil), []domain.Album{summer}, true, 0, nil},
	}
	for _, c := range cases {
		t.Run(c.rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if _, ok := domain.Rules[c.rule]; !ok {
				t.Errorf("%s is not in domain.Rules", c.rule)
			}
			check := func(asset domain.Asset, albums []domain.Album) {
				got, others := layout.Primary(asset, layout.ReadEvents(albums, c.p), c.first)
				if got != c.want || !slices.Equal(others, c.others) {
					t.Errorf("Primary = %d, %v; want %d, %v", got, others, c.want, c.others)
				}
			}
			check(c.asset, c.albums)
			reversed := c.asset
			reversed.Memberships = slices.Clone(c.asset.Memberships)
			slices.Reverse(reversed.Memberships)
			albums := slices.Clone(c.albums)
			slices.Reverse(albums)
			check(reversed, albums)
		})
	}
}
