package plan_test

import (
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/Windemiatrix/go-libs/osxphotos/template"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
	"github.com/Windemiatrix/photosync/internal/testkit/gen"
)

type variantLayout struct {
	templates    domain.Templates
	dirFormat    string
	monthFolders bool
}

// The templates differ from the default ones in the order of the fields and
// in the number of levels, on one side and on both.
var variantLayouts = []variantLayout{
	{domain.Templates{Photos: []string{"{YYYY}", "{MM}/{DD} {Description}"}, Cloud: []string{"{YYYY}-{MM}-{DD} {Description}"}}, "%s-%s-%s %s", false},
	{domain.Templates{Photos: []string{"{YYYY}", "{MM}", "{DD} {Description}"}, Cloud: []string{"{YYYY}", "{MM}", "{DD} - {Description}"}}, "%s/%s/%s - %s", true},
}

// Package gen lays a world out by the default templates: an album MM/DD and
// a description in the folder of its year, and a directory YYYY/MM-DD -
// description. Strings rather than package layout lay it out again, so that
// a fault of layout fails the properties and not the worlds.
func variantLaid(w gen.World, v variantLayout) (domain.LibrarySnapshot, []domain.RemoteEntry) {
	s, dirs, months := w.Snapshot, map[string]string{}, map[string]bool{}
	s.Albums = slices.Clone(s.Albums)
	for i, a := range w.Snapshot.Albums {
		if a.Kind == domain.KindFolder {
			continue
		}
		year, month, day, desc := a.FolderTitles[0], a.Title[:2], a.Title[3:5], a.Title[6:]
		dirs[fmt.Sprintf("%s/%s-%s - %s", year, month, day, desc)] = fmt.Sprintf(v.dirFormat, year, month, day, desc)
		if v.monthFolders {
			folder := a.ParentUUID + "-" + month
			if !months[folder] {
				months[folder] = true
				s.Albums = append(s.Albums, domain.Album{UUID: folder, Kind: domain.KindFolder, Title: month, ParentUUID: a.ParentUUID, FolderTitles: []string{year}})
			}
			s.Albums[i].FolderTitles, s.Albums[i].Title, s.Albums[i].ParentUUID = []string{year, month}, day+" "+desc, folder
		}
	}
	files := slices.Clone(w.Files)
	for i, e := range files {
		if to, ok := dirs[path.Dir(e.Path)]; ok {
			files[i].Path = path.Join(to, path.Base(e.Path))
		}
	}
	return s, files
}

// variantCases are the cases of genCases, but for the asset of a first
// placement, on worlds of perturb laid out by one of layouts, or by the
// default templates when there is none.
func variantCases(perturb gen.Perturbation, layouts ...variantLayout) *rapid.Generator[genCase] {
	return rapid.Custom(func(t *rapid.T) genCase {
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(1, 8).Draw(t, "frames"), Perturb: perturb}).Draw(t, "world")
		tmpl, err := template.Parse("{event_path}/{original_name}")
		if err != nil {
			t.Fatalf("template.Parse: %v", err)
		}
		p := domain.DefaultPolicies()
		p.Deletes = rapid.SampledFrom([]domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate}).Draw(t, "deletes")
		p.Conflict = rapid.SampledFrom([]domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip}).Draw(t, "conflict")
		mode := rapid.SampledFrom(remoteModes).Draw(t, "mode")
		snap, files := w.Snapshot, w.Files
		if len(layouts) > 0 {
			v := rapid.SampledFrom(layouts).Draw(t, "layout")
			p.Templates = v.templates
			snap, files = variantLaid(w, v)
		}
		first := plan.Input{
			Snapshot: snap, Scans: map[string][]domain.RemoteEntry{"alpha": files}, Policies: p, Template: tmpl,
			Remotes: []domain.RemoteSpec{{Name: "alpha", Mode: mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}},
			Fields:  fieldsByKey(snap), Now: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), RunID: "run-1",
		}
		p1, delta, err := plan.Build(first)
		if err != nil {
			t.Fatalf("the first Build: %v", err)
		}
		synced := after(first, p1, delta)
		var mine, service []domain.RemoteEntry
		for _, e := range synced.Scans["alpha"] {
			if e.IsDir || strings.HasPrefix(e.Path, ".photosync/") {
				service = append(service, e)
			} else {
				mine = append(mine, e)
			}
		}
		was := gen.World{Snapshot: synced.Snapshot, Files: mine}
		actions := gen.Actions(was).Draw(t, "actions")
		now := was.Apply(actions...)
		in := synced
		in.Snapshot, in.Scans, in.Fields = now.Snapshot, map[string][]domain.RemoteEntry{"alpha": slices.Concat(now.Files, service)}, fieldsByKey(now.Snapshot)
		return genCase{first: first, in: in, actions: actions}
	})
}

func variantDoubles(n domain.Notice) bool {
	return n.Code == "plan.doubles"
}

// The plan writes into a directory by the name that the listing gives it, so
// the worlds of package gen hold no doubles: a user of a client of the cloud
// blind to case makes them, writing a file beside another under its name in
// lower case.
func variantWithCaseDouble(in plan.Input) plan.Input {
	scan := in.Scans["alpha"]
	i := slices.IndexFunc(scan, func(e domain.RemoteEntry) bool { return !e.IsDir && !strings.HasPrefix(e.Path, ".photosync/") })
	if i < 0 {
		return in
	}
	e := scan[i]
	e.Path = path.Join(path.Dir(e.Path), strings.ToLower(path.Base(e.Path)))
	in.Scans = map[string][]domain.RemoteEntry{"alpha": append(slices.Clone(scan), e)}
	return in
}

// Templates with another order of fields and another number of levels than
// the default ones read back the events that they lay out, and the runs rest
// after the first sync and after the user acts on both sides.
func TestPlanConvergesUnderOwnTemplates(t *testing.T) {
	t.Parallel()
	// covers: INV-1
	rapid.Check(t, func(t *rapid.T) {
		c := variantCases(converging, variantLayouts...).Draw(t, "case")
		first, _, err := plan.Build(c.first)
		if err != nil {
			t.Fatalf("the first Build: %v", err)
		}
		if slices.ContainsFunc(first.Notices, func(n domain.Notice) bool { return n.Code == "plan.skipped" && n.Params[0] == "dir" }) {
			t.Fatalf("under the templates %+v the first sync skips directories:\n%+v", c.first.Policies.Templates, first.Notices)
		}
		settles(t, c.first)
		settles(t, c.in)
	})
}

// A file that the user moves out of the templates is no file of an album and
// no deletion: by the hash of its content the run leaves its asset as it is.
// Without hashes the file reads as deleted, a known limit of the plan. A copy
// out of the templates changes nothing while the files of the links of its
// content stay. Either way the run leaves the directory out of the templates
// as it is, and the next run has nothing to do.
func TestPlanSkipsOutsideTemplates(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		// The user moves the file in a library at rest: in a plan of its own, an
		// album renamed in Photos takes the pair of its directory along while
		// the asset of the file waits, a known limit of the plan. Rest may take
		// a second run, which links a file of the content of another.
		base := remoteCases(rapid.SampledFrom(remoteModes).Draw(t, "mode")).Draw(t, "case")
		for range 3 {
			p0, d0, err := plan.Build(base)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			base = after(base, p0, d0)
			if len(p0.Ops)+len(p0.Links)+len(p0.Albums)+len(p0.Pairs) == 0 {
				break
			}
		}
		scan, at := base.Scans["alpha"], map[string]int{}
		for i, e := range scan {
			at[e.Path] = i
		}
		linked := slices.DeleteFunc(slices.Clone(base.State.Links), func(l domain.Link) bool {
			_, listed := at[l.Path]
			return !listed || l.Status != domain.StatusActive || l.Role != domain.RoleOriginal
		})
		if len(linked) == 0 {
			return
		}
		l := rapid.SampledFrom(linked).Draw(t, "link")
		top, _, _ := strings.Cut(l.Path, "/")
		dir := rapid.SampledFrom([]string{path.Join(path.Dir(l.Path), "sub"), "Albums/x", ".", top}).Draw(t, "outside")
		e := scan[at[l.Path]]
		e.Path = path.Join(dir, path.Base(l.Path))
		copied := rapid.Bool().Draw(t, "copied")
		in := base
		in.Scans = map[string][]domain.RemoteEntry{"alpha": append(slices.Clone(scan), e)}
		if !copied {
			in.Scans["alpha"] = slices.Delete(in.Scans["alpha"], at[l.Path], at[l.Path]+1)
		}
		p, delta, err := plan.Build(in)
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		if slices.ContainsFunc(p.Ops, func(o domain.Op) bool {
			return o.Src != "" && path.Dir(o.Src) == dir || o.Dst != "" && path.Dir(o.Dst) == dir
		}) {
			t.Fatalf("the plan changes %s, a directory out of the templates:\n%+v", dir, p)
		}
		lost := slices.ContainsFunc(base.State.Links, func(m domain.Link) bool {
			_, listed := at[m.Path]
			return !listed && m.Remote == "alpha" && m.MD5+m.SHA256 != "" && m.MD5+m.SHA256 == e.MD5+e.SHA256
		})
		touched := func(o domain.Op) bool {
			return o.Asset == l.AssetID || slices.ContainsFunc(o.Links, func(x domain.Link) bool { return x.AssetID == l.AssetID })
		}
		switch {
		case copied && !lost:
			want, _, err := plan.Build(base)
			got := p
			got.Notices, want.Notices = nil, nil
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("a copy in %s turns the plan\n%+v\ninto\n%+v (%v)", dir, want, p, err)
			}
		case !copied && e.MD5+e.SHA256 != "" &&
			(slices.ContainsFunc(p.Ops, touched) || slices.ContainsFunc(p.Links, func(x domain.Link) bool { return x.AssetID == l.AssetID })):
			t.Fatalf("the plan changes asset %d, whose file the user moved to %s:\n%+v", l.AssetID, dir, p)
		}
		next := after(in, p, delta)
		second, _, err := plan.Build(next)
		if err != nil || len(second.Ops)+len(second.Links)+len(second.Albums)+len(second.Pairs) > 0 && !componentQuarantineCleared(second, next.State) {
			t.Fatalf("the plan after\n%+v\nis\n%+v (%v); want no operation, record, album or pair", p, second, err)
		}
	})
}

// A run changes no file among the names that a client of the cloud blind to
// the form of Unicode or to case takes for one, and puts none below a
// directory among them. It names them in the notice of doubles, and the next
// run, with nothing to free them, names them again.
func TestPlanLeavesDoublesOnGenWorlds(t *testing.T) {
	t.Parallel()
	rapid.Check(t, func(t *rapid.T) {
		c := variantCases(gen.PerturbAll).Draw(t, "case")
		for _, in := range []plan.Input{variantWithCaseDouble(c.first), variantWithCaseDouble(c.in)} {
			p, delta, err := plan.Build(in)
			if err != nil {
				t.Fatalf("Build: %v", err)
			}
			doubled := map[domain.RemoteEntry]bool{}
			for _, n := range p.Notices {
				if !variantDoubles(n) {
					continue
				}
				for _, q := range n.Params[2:] {
					doubled[domain.RemoteEntry{Root: domain.LibraryKind(n.Params[1]), Path: q}] = true
				}
			}
			for _, o := range p.Ops {
				hit := doubled[domain.RemoteEntry{Root: o.Root, Path: o.Src}]
				for d := o.Dst; d != path.Dir(d); d = path.Dir(d) {
					hit = hit || doubled[domain.RemoteEntry{Root: o.DstRoot, Path: d}]
				}
				if hit && o.Remote != "" && o.Kind != domain.OpImport {
					t.Fatalf("the operation %+v changes a double or puts below one:\n%+v", o, p)
				}
			}
			if len(doubled) == 0 {
				continue
			}
			second, _, err := plan.Build(after(in, p, delta))
			if err != nil || !slices.ContainsFunc(second.Notices, variantDoubles) {
				t.Fatalf("the plan after one with doubles is\n%+v (%v); want the doubles named again", second, err)
			}
		}
	})
}

// The cases of the doubles must meet doubles. Fixed seeds give the same cases
// on every run.
func TestVariantCasesNotVacuous(t *testing.T) {
	t.Parallel()
	const cases = 100
	doubled := 0
	for seed := range cases {
		c := variantCases(gen.PerturbAll).Example(seed)
		met := false
		for _, in := range []plan.Input{variantWithCaseDouble(c.first), variantWithCaseDouble(c.in)} {
			p, _, err := plan.Build(in)
			if err != nil {
				t.Fatalf("seed %d: Build: %v", seed, err)
			}
			met = met || slices.ContainsFunc(p.Notices, variantDoubles)
		}
		if met {
			doubled++
		}
	}
	t.Logf("%d of %d cases plan around doubles", doubled, cases)
	if doubled < 50 {
		t.Errorf("fewer than 50 of the cases of seeds 0 to %d plan around doubles (%d do)", cases-1, doubled)
	}
}
