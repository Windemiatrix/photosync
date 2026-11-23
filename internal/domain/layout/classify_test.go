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
	std := domain.DefaultPolicies()
	yy := tpl(std.Templates.Photos, []string{"{YY}", "{MM}-{DD} {Description}"}, "")
	in := func(titles ...string) []string { return titles }

	cases := []struct {
		name  string
		p     domain.Policies
		album domain.Album
		want  domain.AlbumKind
	}{
		{"an event of the template", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, domain.KindEvent},
		{"a trashed event", std, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays", InTrash: true}, domain.KindEvent},
		{"an album without a date", std, domain.Album{FolderTitles: in("2019"), Title: "Summer"}, domain.KindThematic},
		{"an album deeper than the template", std, domain.Album{FolderTitles: in("2024", "Weekends"), Title: "05/10 City"}, domain.KindThematic},
		{"a top-level album", std, domain.Album{Title: "Pets"}, domain.KindThematic},
		{"a top-level album titled with a year", std, domain.Album{Title: "2024"}, domain.KindThematic},
		{"a year folder", std, domain.Album{Kind: domain.KindFolder, Title: "2024"}, domain.KindFolder},
		{"a folder titled like an event", std, domain.Album{Kind: domain.KindFolder, FolderTitles: in("2024"), Title: "05/01 Trip"}, domain.KindFolder},
		{"a stored thematic kind on the path of an event", std, domain.Album{Kind: domain.KindThematic, FolderTitles: in("2024"), Title: "05/01 Holidays"}, domain.KindEvent},
		{"a stored event kind elsewhere", std, domain.Album{Kind: domain.KindEvent, Title: "Pets"}, domain.KindThematic},
		{"{YY} out of its years", yy, domain.Album{FolderTitles: in("1965"), Title: "05/01 x"}, domain.KindThematic},
		{"{YY} within its years", yy, domain.Album{FolderTitles: in("1969"), Title: "05/01 x"}, domain.KindEvent},
		{"templates that Parse refuses", domain.Policies{}, domain.Album{FolderTitles: in("2024"), Title: "05/01 Holidays"}, domain.KindThematic},
	}
	for _, c := range cases {
		t.Run(rule+" "+c.name, func(t *testing.T) {
			t.Parallel()

			if got := layout.Classify(c.album, c.p); got != c.want {
				t.Errorf("Classify(%+v) = %q, want %q", c.album, got, c.want)
			}
		})
	}
}
