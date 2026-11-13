package plan_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/pathtemplate"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

type templateFields map[string]template.Value

func (f templateFields) Field(name string) (template.Value, bool) {
	v, ok := f[name]
	return v, ok
}

// A run gives a new file a name that no file of its directory takes, in the
// cloud or earlier in the run, so it never writes over a file of the user.
// The expected paths follow naming.Free, QuarantinePath and naming.Doubles,
// not the code, and must not depend on the order of the scan and the links.
func TestNames(t *testing.T) {
	t.Parallel()
	const (
		personal, shared = domain.LibraryPersonal, domain.LibraryShared
		holidays, trip   = "2024/05-01 - Holidays", "2024/05-02 - Trip"
		inQuarantine     = ".photosync/quarantine/2026-11-13/"
	)
	day := time.Date(2026, 11, 13, 22, 30, 0, 0, time.UTC)
	scan := []domain.RemoteEntry{
		{Root: personal, Path: "2024", IsDir: true},
		{Root: personal, Path: holidays, IsDir: true},
		{Root: personal, Path: holidays + "/IMG_0001.HEIC", Size: 100},
		{Root: personal, Path: holidays + "/IMG_0001.MOV", Size: 100},
		{Root: personal, Path: holidays + "/IMG_0002.jpg", Size: 100},
		{Root: personal, Path: inQuarantine + holidays + "/IMG_0003.HEIC", Size: 100},
		{Root: personal, Path: "Misc/a.jpg", Size: 100},
		{Root: personal, Path: "Misc/A.JPG", Size: 100},
		{Root: shared, Path: holidays + "/IMG_0004.HEIC", Size: 100},
		{Root: shared, Path: "Misc/b.jpg", Size: 100},
		{Root: shared, Path: "Misc/B.jpg", Size: 100},
	}
	links := []domain.Link{
		{ID: 1, AssetID: 7, Remote: "alpha", Root: personal, Path: holidays + "/IMG_0001.HEIC", Role: domain.RoleOriginal, Status: domain.StatusActive},
		{ID: 2, AssetID: 8, Remote: "alpha", Root: personal, Path: "Misc/x.jpg", Role: domain.RoleOriginal,
			Status: domain.StatusQuarantinedRemote, QuarantinePath: inQuarantine + "Misc/x.jpg"},
		{ID: 3, AssetID: 9, Remote: "beta", Root: personal, Path: "Misc/y.jpg", Role: domain.RoleOriginal,
			Status: domain.StatusQuarantinedRemote, QuarantinePath: inQuarantine + "Misc/y.jpg"},
	}
	doubles := func(n *plan.Names) []string {
		doubled, notices := n.Doubles()
		var out []string
		for e, ok := range doubled {
			if e != (domain.RemoteEntry{Root: e.Root, Path: e.Path}) {
				out = append(out, fmt.Sprintf("a key with more than a root and a path: %+v", e))
			}
			if ok {
				out = append(out, string(e.Root)+":"+e.Path)
			}
		}
		slices.Sort(out)
		for _, x := range notices {
			out = append(out, strings.Join(append([]string{x.Code}, x.Params...), "|"))
		}
		return out
	}
	for _, c := range []struct {
		name string
		run  func(n *plan.Names) []string
		want []string
	}{
		{"a name free in its directory", func(n *plan.Names) []string {
			return []string{n.Free(personal, holidays, "IMG_0005.HEIC")}
		}, []string{holidays + "/IMG_0005.HEIC"}},
		{"a stem that a file of another extension and case takes", func(n *plan.Names) []string {
			return []string{n.Free(personal, holidays, "img_0002.HEIC")}
		}, []string{holidays + "/img_0002 (1).HEIC"}},
		{"a name that the run gave", func(n *plan.Names) []string {
			return []string{n.Free(personal, holidays, "IMG_0005.HEIC"), n.Free(personal, holidays, "IMG_0005.HEIC")}
		}, []string{holidays + "/IMG_0005.HEIC", holidays + "/IMG_0005 (1).HEIC"}},
		{"the same directory under the other root", func(n *plan.Names) []string {
			return []string{n.Free(shared, holidays, "IMG_0001.HEIC"), n.Free(shared, holidays, "IMG_0004.HEIC")}
		}, []string{holidays + "/IMG_0001.HEIC", holidays + "/IMG_0004 (1).HEIC"}},
		{"another directory of the root", func(n *plan.Names) []string {
			return []string{n.Free(personal, trip, "IMG_0002.HEIC")}
		}, []string{trip + "/IMG_0002.HEIC"}},
		{"a directory takes its name in its parent", func(n *plan.Names) []string {
			return []string{n.Free(personal, ".", "2024.jpg")}
		}, []string{"2024 (1).jpg"}},
		{"a free quarantine path", func(n *plan.Names) []string {
			return []string{n.Quarantine(personal, day, holidays+"/IMG_0001.HEIC")}
		}, []string{inQuarantine + holidays + "/IMG_0001.HEIC"}},
		{"a quarantine path that a file of the scan takes", func(n *plan.Names) []string {
			return []string{n.Quarantine(personal, day, holidays+"/IMG_0003.HEIC")}
		}, []string{inQuarantine + holidays + "/IMG_0003 (1).HEIC"}},
		{"a quarantine path that a link of the remote holds, its file gone", func(n *plan.Names) []string {
			return []string{n.Quarantine(personal, day, "Misc/x.jpg")}
		}, []string{inQuarantine + "Misc/x (1).jpg"}},
		{"the quarantine path of a link of another remote", func(n *plan.Names) []string {
			return []string{n.Quarantine(personal, day, "Misc/y.jpg")}
		}, []string{inQuarantine + "Misc/y.jpg"}},
		{"a quarantine path that the run gave, its number too", func(n *plan.Names) []string {
			return []string{n.Quarantine(personal, day, "Misc/x.jpg"), n.Quarantine(personal, day, "Misc/x (1).jpg")}
		}, []string{inQuarantine + "Misc/x (1).jpg", inQuarantine + "Misc/x (1) (1).jpg"}},
		{"quarantine under the other root", func(n *plan.Names) []string {
			return []string{n.Quarantine(shared, day, holidays+"/IMG_0003.HEIC"), n.Quarantine(shared, day, "Misc/x.jpg")}
		}, []string{inQuarantine + holidays + "/IMG_0003.HEIC", inQuarantine + "Misc/x.jpg"}},
		{"the doubles of the scan, the personal root first", doubles, []string{
			"personal:Misc/A.JPG", "personal:Misc/a.jpg", "shared:Misc/B.jpg", "shared:Misc/b.jpg",
			"plan.doubles|alpha|personal|Misc/A.JPG|Misc/a.jpg",
			"plan.doubles|alpha|shared|Misc/B.jpg|Misc/b.jpg",
		}},
		{"a name given in a directory of another case doubles a file there", func(n *plan.Names) []string {
			return append([]string{n.Free(personal, "2024/05-01 - holidays", "IMG_0001.HEIC")}, doubles(n)...)
		}, []string{
			"2024/05-01 - holidays/IMG_0001.HEIC",
			"personal:2024/05-01 - Holidays/IMG_0001.HEIC", "personal:2024/05-01 - holidays/IMG_0001.HEIC",
			"personal:Misc/A.JPG", "personal:Misc/a.jpg", "shared:Misc/B.jpg", "shared:Misc/b.jpg",
			"plan.doubles|alpha|personal|2024/05-01 - Holidays/IMG_0001.HEIC|2024/05-01 - holidays/IMG_0001.HEIC",
			"plan.doubles|alpha|personal|Misc/A.JPG|Misc/a.jpg",
			"plan.doubles|alpha|shared|Misc/B.jpg|Misc/b.jpg",
		}},
		{"nothing scanned and no links", func(*plan.Names) []string {
			n := plan.NewNames("alpha", nil, nil)
			return append([]string{n.Free(personal, holidays, "IMG_0001.HEIC"), n.Quarantine(personal, day, holidays+"/IMG_0003.HEIC")}, doubles(n)...)
		}, []string{holidays + "/IMG_0001.HEIC", inQuarantine + holidays + "/IMG_0003.HEIC"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			s, ls := slices.Clone(scan), slices.Clone(links)
			slices.Reverse(s)
			slices.Reverse(ls)
			if got := c.run(plan.NewNames("alpha", scan, links)); !slices.Equal(got, c.want) {
				t.Errorf("got %q; want %q", got, c.want)
			}
			if got := c.run(plan.NewNames("alpha", s, ls)); !slices.Equal(got, c.want) {
				t.Errorf("with the scan and the links reversed: got %q; want %q", got, c.want)
			}
			slices.Reverse(s)
			slices.Reverse(ls)
			if !slices.Equal(s, scan) || !slices.Equal(ls, links) {
				t.Error("NewNames changed its input")
			}
		})
	}
}

// A run uploads a file at the paths of the template, each under a name that
// no file of its directory takes. An error that one asset meets must leave
// only that asset out, and any other error must stop the run, since the
// template fails every asset alike.
func TestNamesRender(t *testing.T) {
	t.Parallel()
	const holidays = "2024/05-01 - Holidays"
	now := time.Date(2026, 11, 13, 10, 0, 0, 0, time.UTC)
	scan := []domain.RemoteEntry{
		{Root: domain.LibraryPersonal, Path: holidays + "/IMG_0001.HEIC", Size: 100},
		{Root: domain.LibraryPersonal, Path: holidays + "/IMG_0001.MOV", Size: 100},
		{Root: domain.LibraryPersonal, Path: "Trips/IMG_0005.jpg", Size: 100},
		{Root: domain.LibraryShared, Path: holidays + "/IMG_0004.HEIC", Size: 100},
	}
	keywords := make([]string, 101)
	for i := range keywords {
		keywords[i] = fmt.Sprintf("k%03d", i)
	}
	fields := templateFields{"album": {Strings: []string{"Holidays", "Trips"}}, "keyword": {Strings: keywords}}
	photo := domain.Asset{
		ID: 7, Key: domain.AssetKey{UUID: "uuid-7"}, Library: domain.LibraryPersonal, TargetLibrary: domain.LibraryPersonal,
		Resources: []domain.Resource{{Role: domain.ResourceOriginal, Filename: "IMG_0001.HEIC", Size: 100}},
	}
	inShared, bare := photo, photo
	inShared.TargetLibrary, bare.Resources = domain.LibraryShared, nil
	for _, c := range []struct {
		name, tmpl string
		mode       domain.Mode
		asset      domain.Asset
		paths      []string
		notice     string // a part of the error that the notice holds, "" for no notice
		err        error
	}{
		{"a free name in the directory of the event", "{event_path}/IMG_0005", domain.ModeTwoWay, photo,
			[]string{holidays + "/IMG_0005.HEIC"}, "", nil},
		{"a stem that the photo and the video take", "{event_path}/IMG_0001", domain.ModeTwoWay, photo,
			[]string{holidays + "/IMG_0001 (1).HEIC"}, "", nil},
		{"the root of the library that the asset goes to", "{event_path}/IMG_0004", domain.ModeTwoWay, inShared,
			[]string{holidays + "/IMG_0004 (1).HEIC"}, "", nil},
		{"upload-only: a path for each album, each name free in its directory", "{album}/IMG_0005", domain.ModeUploadOnly, photo,
			[]string{"Holidays/IMG_0005.HEIC", "Trips/IMG_0005 (1).HEIC"}, "", nil},
		{"an unknown field stops the run", "{event_path}/{foo}", domain.ModeTwoWay, photo,
			nil, "", pathtemplate.ErrUnknownField},
		{"a field that photosync never renders stops the run", "{event_path}/{detected_text}", domain.ModeTwoWay, photo,
			nil, "", template.ErrUnsupportedField},
		{"an asset without an original file is left out", "{event_path}/IMG_0005", domain.ModeTwoWay, bare,
			nil, "has no original file", nil},
		{"download-only: a file below the directory of the event is left out", "{event_path}/Misc/IMG_0005", domain.ModeDownloadOnly, photo,
			nil, "does not fit the mode", nil},
		{"too many paths leave the asset out", "{event_path}/{keyword}{keyword}", domain.ModeTwoWay, photo,
			nil, "too many values", nil},
		{"a template that fails to render leaves the asset out", "{event_path}/{var}", domain.ModeTwoWay, photo,
			nil, "var takes a name and a value", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tmpl, err := template.Parse(c.tmpl)
			if err != nil {
				t.Fatalf("template.Parse(%q): %v", c.tmpl, err)
			}
			reversed := slices.Clone(scan)
			slices.Reverse(reversed)
			for _, s := range [][]domain.RemoteEntry{scan, reversed} {
				paths, notices, err := plan.NewNames("alpha", s, nil).Render(tmpl, holidays, c.asset, fields, c.mode, now)
				noticed := len(notices) == 0
				if c.notice != "" {
					noticed = len(notices) == 1 && notices[0].Code == "plan.no_path" && len(notices[0].Params) == 3 &&
						notices[0].Params[0] == "alpha" && notices[0].Params[1] == "uuid-7" && strings.Contains(notices[0].Params[2], c.notice)
				}
				if !slices.Equal(paths, c.paths) || !noticed || !errors.Is(err, c.err) {
					t.Errorf("Render(%q) = %q, %q, %v; want %q, a notice with %q, %v", c.tmpl, paths, notices, err, c.paths, c.notice, c.err)
				}
			}
		})
	}
}
