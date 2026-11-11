package layout_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/layout"
)

// covers: R-LAYOUT-01
func TestClassify(t *testing.T) {
	t.Parallel()

	const rule = "R-LAYOUT-01"
	if _, ok := domain.Rules[rule]; !ok {
		t.Fatalf("%s is not in domain.Rules", rule)
	}

	cases := []struct {
		name  string
		album domain.Album
		want  domain.AlbumKind
	}{
		{"an album in a year folder", domain.Album{FolderTitles: []string{"2024"}, Title: "05/01 Holidays"}, domain.KindEvent},
		{"an album in a folder of a year folder", domain.Album{FolderTitles: []string{"2024", "Weekends"}, Title: "05/10 City"}, domain.KindEvent},
		{"an undated album in a year folder", domain.Album{FolderTitles: []string{"2019"}, Title: "Summer"}, domain.KindEvent},
		{"a trashed album in a year folder", domain.Album{FolderTitles: []string{"2024"}, Title: "05/02 Old", InTrash: true}, domain.KindEvent},
		{"a top-level album", domain.Album{Title: "Pets"}, domain.KindThematic},
		{"a top-level album titled with a year", domain.Album{Title: "2024"}, domain.KindThematic},
		{"an album in another folder", domain.Album{FolderTitles: []string{"Crafts"}, Title: "Painting"}, domain.KindThematic},
		{"a year folder inside another folder", domain.Album{FolderTitles: []string{"Archive", "2024"}, Title: "Camp"}, domain.KindThematic},
		{"a folder of four letters", domain.Album{FolderTitles: []string{"Misc"}, Title: "Camp"}, domain.KindThematic},
		{"a folder of three digits", domain.Album{FolderTitles: []string{"202"}, Title: "Camp"}, domain.KindThematic},
		{"a folder of five digits", domain.Album{FolderTitles: []string{"20245"}, Title: "Camp"}, domain.KindThematic},
		{"a folder of four bytes of other digits", domain.Album{FolderTitles: []string{string([]rune{0x0661, 0x0662})}, Title: "Camp"}, domain.KindThematic},
		{"a folder of full-width digits", domain.Album{FolderTitles: []string{string([]rune{0xFF12, 0xFF10, 0xFF12, 0xFF14})}, Title: "Camp"}, domain.KindThematic},
		{"a year folder", domain.Album{Kind: domain.KindFolder, Title: "2024"}, domain.KindFolder},
		{"a folder in a year folder", domain.Album{Kind: domain.KindFolder, FolderTitles: []string{"2024"}, Title: "Weekends"}, domain.KindFolder},
		{"a stored thematic kind in a year folder", domain.Album{Kind: domain.KindThematic, FolderTitles: []string{"2024"}, Title: "05/01 Holidays"}, domain.KindEvent},
		{"a stored event kind outside the year folders", domain.Album{Kind: domain.KindEvent, Title: "Pets"}, domain.KindThematic},
	}
	for _, c := range cases {
		t.Run(rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if got := layout.Classify(c.album); got != c.want {
				t.Errorf("Classify(%+v) = %q, want %q", c.album, got, c.want)
			}
		})
	}
}
