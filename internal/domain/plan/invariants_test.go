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

// converging leaves out the one perturbation whose worlds do not converge by
// design: a directory in NFD beside the title of its album in NFC makes
// doubles, and a run leaves doubles alone until the user renames one.
const converging = gen.PerturbAll &^ gen.PerturbNFD

type genCase struct {
	first, in plan.Input
	actions   []gen.Action
}

func genCases() *rapid.Generator[genCase] {
	return rapid.Custom(func(t *rapid.T) genCase {
		w := gen.Worlds(gen.Config{Frames: rapid.IntRange(1, 8).Draw(t, "frames"), Perturb: converging}).Draw(t, "world")
		tmpl, err := template.Parse("{event_path}/{original_name}")
		if err != nil {
			t.Fatalf("template.Parse: %v", err)
		}
		p := domain.DefaultPolicies()
		p.Deletes = rapid.SampledFrom([]domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate}).Draw(t, "deletes")
		p.Conflict = rapid.SampledFrom([]domain.ConflictPolicy{domain.ConflictRemoteWins, domain.ConflictICloudWins, domain.ConflictAsk, domain.ConflictSkip}).Draw(t, "conflict")
		mode := rapid.SampledFrom([]domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly}).Draw(t, "mode")
		first := plan.Input{
			Snapshot: w.Snapshot, Scans: map[string][]domain.RemoteEntry{"alpha": w.Files}, Policies: p, Template: tmpl,
			Remotes: []domain.RemoteSpec{{Name: "alpha", Mode: mode, Capabilities: domain.Capabilities{ContentHash: domain.HashMD5}}},
			Fields:  fieldsByKey(w.Snapshot), Now: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC), RunID: "run-1",
		}
		p1, delta, err := plan.Build(first)
		if err != nil {
			t.Fatalf("the first Build: %v", err)
		}
		synced := after(first, p1, delta)
		var files, service []domain.RemoteEntry
		for _, e := range synced.Scans["alpha"] {
			if e.IsDir || strings.HasPrefix(e.Path, ".photosync/") {
				service = append(service, e)
			} else {
				files = append(files, e)
			}
		}
		was := gen.World{Snapshot: synced.Snapshot, Files: files}
		actions := gen.Actions(was).Draw(t, "actions")
		now := was.Apply(actions...)
		if rapid.Bool().Draw(t, "loose") {
			now.Snapshot.Assets = withLoose(t, now.Snapshot.Assets)
		}
		in := synced
		in.Snapshot, in.Scans, in.Fields = now.Snapshot, map[string][]domain.RemoteEntry{"alpha": slices.Concat(now.Files, service)}, fieldsByKey(now.Snapshot)
		return genCase{first: first, in: in, actions: actions}
	})
}

// An asset without an album, on the day of an asset in an event, takes a first
// placement, which no action of package gen makes: a new asset there always
// comes into an album. An hour away from that asset and without SubSec, it
// pairs with no file by its time.
func withLoose(t *rapid.T, assets []domain.Asset) []domain.Asset {
	placed := slices.DeleteFunc(slices.Clone(assets), func(a domain.Asset) bool {
		return a.InTrash || len(a.AlbumUUIDs) == 0 || a.TZOffset == nil
	})
	if len(placed) == 0 {
		return assets
	}
	a := rapid.SampledFrom(placed).Draw(t, "dated like")
	shift := time.Hour
	if a.Taken.Add(time.Duration(*a.TZOffset)*time.Second).Hour() >= 12 {
		shift = -time.Hour
	}
	a.Key, a.AlbumUUIDs = domain.AssetKey{CloudAssetGUID: "guid-LOOSE", UUID: "uuid-LOOSE"}, nil
	a.Taken, a.SubSec, a.OriginalName = a.Taken.Add(shift), "", "LOOSE"+path.Ext(a.OriginalName)
	a.Resources = []domain.Resource{{Role: domain.ResourceOriginal, Filename: a.OriginalName, Size: 7_000_000}}
	return append(slices.Clone(assets), a)
}

func fieldsByKey(s domain.LibrarySnapshot) func(domain.AssetKey) template.Photo {
	byKey := map[domain.AssetKey]template.Photo{}
	for _, a := range s.Assets {
		byKey[a.Key] = templateFields{"original_name": {Strings: []string{a.OriginalName}}}
	}
	return func(k domain.AssetKey) template.Photo {
		if f, ok := byKey[k]; ok {
			return f
		}
		return templateFields{}
	}
}

// A quarter of an hour is longer than SettleMinutes, so what a run puts has
// settled by the next run, and short enough that all the runs of a case fall
// on one day, the day in the path of the quarantine.
func after(in plan.Input, p domain.Plan, delta domain.SnapshotDelta) plan.Input {
	next := in
	next.State, next.Snapshot, next.Scans = plan.Simulate(in, p, delta)
	next.Now = in.Now.Add(15 * time.Minute)
	next.Fields, next.RunID = fieldsByKey(next.Snapshot), next.Now.Format(time.RFC3339)
	return next
}

func settles(t rapid.TB, in plan.Input) {
	t.Helper()
	p, delta, err := plan.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	checkPlan(t, in, p)
	next := after(in, p, delta)
	checkApplied(t, p, next)
	second, again, err := plan.Build(next)
	if err != nil {
		t.Fatalf("the second Build: %v", err)
	}
	checkPlan(t, next, second)
	end := after(next, second, again)
	if componentQuarantineCleared(second, next.State) {
		third, _, err := plan.Build(end)
		if err != nil || len(third.Ops)+len(third.Links)+len(third.Albums)+len(third.Pairs) > 0 {
			t.Fatalf("the plan after a component left quarantine\n%+v\n%v\nwant no operation, record, album or pair", third, err)
		}
		return
	}
	open := !slices.ContainsFunc(second.Conflicts, func(c domain.Conflict) bool {
		return c.Status != domain.ConflictStatusOpen && c.Status != domain.ConflictStatusSkipped
	})
	if len(second.Ops)+len(second.Links)+len(second.Albums)+len(second.Pairs) > 0 || !open ||
		!reflect.DeepEqual(end.State, next.State) || !reflect.DeepEqual(end.Snapshot, next.Snapshot) || !reflect.DeepEqual(end.Scans, next.Scans) {
		t.Fatalf("the plan\n%+v\nleaves\n%+v\n%+v\n%+v\nand the next plan\n%+v\nleaves\n%+v\n%+v\n%+v\nwant no operation, record, album or pair, only open or skipped conflicts, and nothing that it changes",
			p, next.State, next.Snapshot, next.Scans, second, end.State, end.Snapshot, end.Scans)
	}
}

// Assets count as plan.Limit counts them: an import of a new asset is an
// asset of its own, and an operation of no asset is none.
func assetsWithOps(p domain.Plan) int {
	n, seen := 0, map[domain.AssetID]bool{}
	for _, o := range p.Ops {
		switch {
		case o.Kind == domain.OpImport && o.Asset == 0:
			n++
		case o.Asset != 0 && !seen[o.Asset]:
			n, seen[o.Asset] = n+1, true
		}
	}
	return n
}

// outcome is what a sync leaves in the photo library and the listings, as
// sorted lines that do not depend on when or in how many runs it got there:
// an asset that a run imports, whose UUID names that run, counts by its name,
// an album by its place, and a file without its MTime, the time of the run
// that put it.
func outcome(in plan.Input) []string {
	places := map[string]string{}
	for _, a := range in.Snapshot.Albums {
		places[a.UUID] = strings.Join(append(slices.Clone(a.FolderTitles), a.Title), "/")
	}
	var lines []string
	for _, a := range in.Snapshot.Albums {
		lines = append(lines, "album "+places[a.UUID])
	}
	for _, a := range in.Snapshot.Assets {
		key := a.Key.UUID
		if strings.HasPrefix(key, "sim-") {
			key = "imported"
		}
		var albums []string
		for _, u := range a.AlbumUUIDs {
			albums = append(albums, places[u])
		}
		slices.Sort(albums)
		lines = append(lines, fmt.Sprintf("asset %s %s %v trashed=%t in %q", key, a.OriginalName, a.Resources, a.InTrash, albums))
	}
	for name, es := range in.Scans {
		for _, e := range es {
			lines = append(lines, fmt.Sprintf("file %s %s %s %d %s %s dir=%t", name, e.Root, e.Path, e.Size, e.MD5, e.SHA256, e.IsDir))
		}
	}
	slices.Sort(lines)
	return lines
}

func limited(t rapid.TB, in plan.Input) {
	t.Helper()
	p, delta, err := plan.Build(in)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	want, budget := outcome(after(in, p, delta)), assetsWithOps(p)
	cur, before := in, budget+1
	for run := 0; ; run++ {
		next, d, err := plan.Build(cur)
		if err != nil {
			t.Fatalf("Build of run %d: %v", run, err)
		}
		checkPlan(t, cur, next)
		if len(next.Ops)+len(next.Links)+len(next.Albums)+len(next.Pairs) == 0 {
			break
		}
		n := assetsWithOps(next)
		if n >= before && n > 0 || run > budget {
			t.Fatalf("run %d with a limit of one asset plans for %d assets after %d; want fewer, in at most %d runs:\n%+v", run, n, before, budget+1, next)
		}
		before, cur = n, after(cur, plan.Limit(next, 1), d)
	}
	if pairFollowCut(in, p) {
		return
	}
	got := unnumbered(outcome(cur))
	if want = unnumbered(want); !slices.Equal(got, want) {
		t.Fatalf("runs with a limit of one asset end in\n%s\nwant the end of one run without it\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// A pair that follows an album renamed in Photos moves in the records of the
// run, and the files of its directory with the operations of their assets: a
// limit that cuts one of those off leaves its file in the directory of the old
// name, which a later run may take for a new album. A known limit of the
// plan.
func pairFollowCut(in plan.Input, p domain.Plan) bool {
	var old []string
	for _, x := range p.Pairs {
		for _, y := range in.State.AlbumPairs {
			if x.AlbumID == y.AlbumID && x.Remote == y.Remote && x.Root == y.Root && x.DirPath != y.DirPath {
				old = append(old, y.DirPath)
			}
		}
	}
	kept := plan.Limit(p, 1).Ops
	return slices.ContainsFunc(p.Ops, func(o domain.Op) bool {
		cut := !slices.ContainsFunc(kept, func(k domain.Op) bool { return k.Compare(o) == 0 })
		return cut && (slices.Contains(old, path.Dir(o.Src)) || slices.Contains(old, path.Dir(o.Dst)))
	})
}

// The order of runs decides which of two files of one name in a directory
// gets " (N)": a known limit of the plan, so the runs compare without it.
func unnumbered(lines []string) []string {
	out := make([]string, len(lines))
	for k, line := range lines {
		if !strings.HasPrefix(line, "file ") {
			out[k] = line
			continue
		}
		var b strings.Builder
		for {
			i := strings.Index(line, " (")
			if i < 0 {
				break
			}
			j := strings.IndexByte(line[i:], ')')
			if j > 2 && strings.Trim(line[i+2:i+j], "0123456789") == "" {
				b.WriteString(line[:i])
			} else {
				b.WriteString(line[:i+2])
				j = 1
			}
			line = line[i+j+1:]
		}
		out[k] = b.String() + line
	}
	slices.Sort(out)
	return out
}

// A component that follows its main file to quarantine while the asset stays
// keeps a quarantined_remote link that the next run clears with a record
// alone: a known limit of the plan.
func componentQuarantineCleared(second domain.Plan, after domain.State) bool {
	if len(second.Ops)+len(second.Conflicts)+len(second.Albums)+len(second.Pairs) > 0 || len(second.Links) == 0 {
		return false
	}
	was := map[domain.LinkID]domain.Link{}
	for _, l := range after.Links {
		was[l.ID] = l
	}
	for _, l := range second.Links {
		w, ok := was[l.ID]
		if !ok || l.Status != "" || w.Status != domain.StatusQuarantinedRemote || w.Role != domain.RoleLiveVideo && w.Role != domain.RoleRAWPair {
			return false
		}
	}
	return true
}

// checkPlan holds a plan to what package domain promises of every plan: a
// link changed once, the operations in order, nothing removed for good but by
// a delete under propagate, an expire or an asset_delete by R-STATUS-06
// (INV-2), nothing that the mode of its remote does not run (INV-5), and no
// path out of its root or out of the quarantine of the day (INV-20).
func checkPlan(t rapid.TB, in plan.Input, p domain.Plan) {
	t.Helper()
	known, modes := map[domain.LinkID]bool{}, map[string]domain.Mode{}
	for _, l := range in.State.Links {
		known[l.ID] = true
	}
	for _, r := range in.Remotes {
		modes[r.Name] = r.Mode
	}
	upOnly := !slices.ContainsFunc(in.Remotes, func(r domain.RemoteSpec) bool { return r.Mode != domain.ModeUploadOnly })
	times := map[domain.Link]int{}
	count := func(l domain.Link) {
		if known[l.ID] {
			times[domain.Link{ID: l.ID}]++
		} else {
			times[domain.Link{Remote: l.Remote, Root: l.Root, Path: l.Path}]++
		}
	}
	for _, l := range p.Links {
		if l.ID == 0 {
			t.Fatalf("the plan records a link without an ID:\n%+v", p)
		}
		count(l)
	}
	moved := map[domain.LinkID]bool{}
	for _, o := range p.Ops {
		for _, l := range o.Links {
			moved[l.ID] = moved[l.ID] || o.Kind == domain.OpMove
		}
	}
	for _, o := range p.Ops {
		for _, l := range o.Links {
			// An import that replaces an asset takes the new content of a link
			// that a move of the same run takes to its new place.
			if o.Kind != domain.OpImport || !known[l.ID] || !moved[l.ID] {
				count(l)
			}
		}
	}
	for l, n := range times {
		if n > 1 {
			t.Fatalf("the plan changes the link %+v %d times:\n%+v", l, n, p)
		}
	}
	clean := func(s string) bool {
		return s == "" || path.Clean(s) == s && !path.IsAbs(s) && !slices.Contains(strings.Split(s, "/"), "..")
	}
	today := ".photosync/quarantine/" + in.Now.UTC().Format(time.DateOnly) + "/"
	for i, o := range p.Ops {
		mode := modes[o.Remote]
		rest, kept := strings.CutPrefix(o.Src, ".photosync/quarantine/")
		day, _, _ := strings.Cut(rest, "/")
		_, undated := time.Parse(time.DateOnly, day)
		switch {
		case i > 0 && p.Ops[i-1].Compare(o) >= 0:
			t.Fatalf("the operation %+v comes after %+v:\n%+v", o, p.Ops[i-1], p)
		case o.Irreversible != (o.Kind == domain.OpDelete || o.Kind == domain.OpExpire || o.Kind == domain.OpAssetDelete && o.Rule == "R-STATUS-06"),
			o.Kind == domain.OpDelete && in.Policies.Deletes != domain.DeletesPropagate,
			o.Kind == domain.OpExpire && o.Rule != "R-STATUS-09":
			t.Fatalf("the operation %+v is irreversible=%t under deletes %s", o, o.Irreversible, in.Policies.Deletes)
		case o.Remote == "" && upOnly, o.Kind == domain.OpImport && mode == domain.ModeUploadOnly,
			o.Remote != "" && o.Kind != domain.OpImport && o.Kind != domain.OpExpire && mode == domain.ModeDownloadOnly:
			t.Fatalf("the operation %+v goes against the modes of %+v", o, in.Remotes)
		case o.Remote != "" && (!clean(o.Src) || !clean(o.Dst)),
			(o.Kind == domain.OpMove || o.Kind == domain.OpQuarantine || o.Kind == domain.OpCopy) && (o.Root == "" || o.Src == ""),
			(o.Kind == domain.OpMove || o.Kind == domain.OpCopy || o.Kind == domain.OpPut) && (o.DstRoot == "" || o.Dst == ""),
			o.Kind == domain.OpQuarantine && (o.DstRoot != o.Root || !strings.HasPrefix(o.Dst, today)),
			o.Kind == domain.OpExpire && (!kept || undated != nil):
			t.Fatalf("the operation %+v leaves its root or the quarantine of %s", o, today)
		}
	}
}

// checkApplied holds what a run leaves once its plan p succeeds, next, to
// INV-3, to one directory for each album pair, to components beside their
// main files, and to a state that the next run reconciles to itself. An
// asset that waits for the user in a remote keeps its links there as they
// were; so does a component of an asset whose main file lies in a directory
// where the plan found a name taken, a known limit of the plan.
func checkApplied(t rapid.TB, p domain.Plan, next plan.Input) {
	t.Helper()
	entries, modes, waits := map[domain.Link]domain.RemoteEntry{}, map[string]domain.Mode{}, map[domain.Conflict]bool{}
	for name, es := range next.Scans {
		for _, e := range es {
			entries[domain.Link{Remote: name, Root: e.Root, Path: e.Path}] = e
		}
	}
	for _, r := range next.Remotes {
		modes[r.Name] = r.Mode
	}
	for _, c := range next.State.Conflicts {
		waits[domain.Conflict{AssetID: c.AssetID, Remote: c.Remote}] = true
	}
	taken := map[domain.Link]bool{}
	for _, n := range p.Notices {
		if n.Code == "plan.path_taken" {
			taken[domain.Link{Remote: n.Params[0], Root: domain.LibraryKind(n.Params[1]), Path: path.Dir(n.Params[2])}] = true
		}
	}
	mainOf := func(m, l domain.Link) bool {
		return (m.Role == domain.RoleOriginal || m.Role == domain.RoleSource) && m.Status == domain.StatusActive && m.AssetID == l.AssetID && m.Remote == l.Remote
	}
	places := map[domain.Link]bool{}
	for _, l := range next.State.Links {
		at := domain.Link{Remote: l.Remote, Root: l.Root, Path: l.Path}
		// Quarantine frees the path of a link; the state holds the others one to
		// a path.
		if l.Status == domain.StatusActive || l.Status == domain.StatusDetached {
			if places[at] {
				t.Fatalf("two links of the state lie at %+v:\n%+v", at, next.State.Links)
			}
			places[at] = true
		}
		if l.Status != domain.StatusActive || waits[domain.Conflict{AssetID: l.AssetID, Remote: l.Remote}] {
			continue
		}
		e, ok := entries[at]
		if !ok || e.Size != l.Size || l.MD5 != "" && l.MD5 != e.MD5 || l.SHA256 != "" && l.SHA256 != e.SHA256 {
			t.Fatalf("the active link %+v has the file %+v (listed: %t)", l, e, ok)
		}
		// The plan moves nothing in a download-only cloud, so a component stays
		// where it lies when the user moves its main file; the check skips such
		// remotes whole.
		if (l.Role == domain.RoleLiveVideo || l.Role == domain.RoleRAWPair) && modes[l.Remote] != domain.ModeDownloadOnly &&
			!slices.ContainsFunc(next.State.Links, func(m domain.Link) bool {
				return mainOf(m, l) && taken[domain.Link{Remote: m.Remote, Root: m.Root, Path: path.Dir(m.Path)}]
			}) &&
			!slices.ContainsFunc(next.State.Links, func(m domain.Link) bool { return mainOf(m, l) && plan.ComponentBeside(m, l) }) {
			t.Fatalf("the component %+v lies beside no main file of its asset:\n%+v", l, next.State.Links)
		}
	}
	dirs := map[domain.AlbumPair]bool{}
	for _, x := range next.State.AlbumPairs {
		k := domain.AlbumPair{Remote: x.Remote, Root: x.Root, DirPath: x.DirPath}
		if dirs[k] {
			t.Fatalf("two album pairs hold %+v:\n%+v", k, next.State.AlbumPairs)
		}
		dirs[k] = true
	}
	delta, _, err := plan.Reconcile(next.State, next.Snapshot, next.RunID, next.Policies)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	held, albums := map[domain.AssetKey]domain.Asset{}, map[domain.AlbumID]domain.Album{}
	for _, a := range next.State.Assets {
		held[a.Key] = a
	}
	for _, a := range next.State.Albums {
		albums[a.ID] = a
	}
	for _, a := range delta.Assets {
		if h, ok := held[a.Key]; ok && !reflect.DeepEqual(a, h) {
			t.Fatalf("the state holds\n%+v\nwhich the next run reconciles to\n%+v", h, a)
		}
	}
	for _, a := range delta.Albums {
		if !reflect.DeepEqual(a, albums[a.ID]) {
			t.Fatalf("the state holds\n%+v\nwhich the next run reconciles to\n%+v", albums[a.ID], a)
		}
	}
}

// After the first sync of a world of package gen, and again after a user acts
// on both sides, the next run has nothing to do, whatever the mode and the
// deletes and conflict policies.
func TestPlanConvergesOnGenWorlds(t *testing.T) {
	t.Parallel()
	// covers: INV-1
	// covers: INV-2
	// covers: INV-3
	// covers: INV-5
	// covers: INV-20
	rapid.Check(t, func(t *rapid.T) {
		c := genCases().Draw(t, "case")
		settles(t, c.first)
		settles(t, c.in)
	})
}

// Runs with a limit of one asset end where one run without a limit ends, each
// planning for fewer assets than the one before: a new asset that the limit
// cuts off from its first link stays new and takes its first placement later.
func TestPlanLimitConvergesOnGenWorlds(t *testing.T) {
	t.Parallel()
	// covers: INV-1
	// covers: INV-2
	// covers: INV-5
	// covers: INV-20
	rapid.Check(t, func(t *rapid.T) {
		limited(t, genCases().Draw(t, "case").in)
	})
}

// The cases of the properties above must hold what they claim to check. Fixed
// seeds give the same cases on every run.
func TestGenCasesNotVacuous(t *testing.T) {
	t.Parallel()
	const cases = 100
	var matched, acting, several, loose, quarantined, unlimited, unsettled int
	drawn := map[gen.Action]bool{}
	for seed := range cases {
		c := genCases().Example(seed)
		first, _, err := plan.Build(c.first)
		if err != nil {
			t.Fatalf("seed %d: the first Build: %v", seed, err)
		}
		p, delta, err := plan.Build(c.in)
		if err != nil {
			t.Fatalf("seed %d: Build after the actions: %v", seed, err)
		}
		next := after(c.in, p, delta)
		second, _, err := plan.Build(next)
		if err != nil {
			t.Fatalf("seed %d: the second Build: %v", seed, err)
		}
		if pairFollowCut(c.in, p) {
			unlimited++
		}
		if componentQuarantineCleared(second, next.State) {
			unsettled++
		}
		if len(first.Links) > 0 {
			matched++
		}
		if len(p.Ops)+len(p.Links)+len(p.Albums)+len(p.Pairs) > 0 {
			acting++
		}
		if assetsWithOps(p) > 1 {
			several++
		}
		if slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return o.Kind == domain.OpPut && strings.HasPrefix(path.Base(o.Dst), "LOOSE") }) {
			loose++
		}
		if slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return o.Kind == domain.OpQuarantine && o.Rule == "R-OBJECT-04" }) {
			quarantined++
		}
		for _, a := range c.actions {
			drawn[gen.Action{Side: a.Side, Kind: a.Kind}] = true
		}
	}
	t.Logf("of %d cases the first sync matches files in %d; after the actions %d plan something, %d for more than one asset, %d put an asset without an album into an event, %d take the file of an asset out of every event to quarantine; a known limit spares %d from the comparison of runs with a limit and %d from the check of the next run",
		cases, matched, acting, several, loose, quarantined, unlimited, unsettled)
	var missing []string
	// A known limit that spares more cases than these would leave the
	// properties checking little.
	if unlimited > 15 {
		missing = append(missing, fmt.Sprintf("at most 15 cases spared by a moved pair (%d are)", unlimited))
	}
	if unsettled > 3 {
		missing = append(missing, fmt.Sprintf("at most 3 cases spared by a component out of quarantine (%d are)", unsettled))
	}
	for _, c := range []struct {
		what      string
		got, want int
	}{
		{"a first sync that matches files", matched, 48},
		{"a plan after the actions", acting, 46},
		{"a plan for more than one asset", several, 25},
		{"a first placement", loose, 4},
		{"a quarantine of an asset out of every event", quarantined, 6},
	} {
		if c.got < c.want {
			missing = append(missing, fmt.Sprintf("%d cases with %s (%d are)", c.want, c.what, c.got))
		}
	}
	for _, kind := range []gen.ActionKind{gen.Create, gen.Delete, gen.AlbumAdd, gen.AlbumRemove, gen.AlbumMove, gen.AlbumRename} {
		if !drawn[gen.Action{Side: gen.ICloud, Kind: kind}] {
			missing = append(missing, fmt.Sprintf("the action %s in Photos", kind))
		}
	}
	for _, kind := range []gen.ActionKind{gen.Create, gen.Move, gen.Rename, gen.Replace, gen.Delete, gen.AlbumRename} {
		if !drawn[gen.Action{Side: gen.Remote, Kind: kind}] {
			missing = append(missing, fmt.Sprintf("the action %s in the cloud", kind))
		}
	}
	if len(missing) > 0 {
		t.Errorf("the cases of seeds 0 to %d lack %s", cases-1, strings.Join(missing, "; "))
	}
}

// The checks of every plan must meet what they look at in the cases of the
// properties above. Fixed seeds give the same cases on every run.
func TestPlanChecksNotVacuous(t *testing.T) {
	t.Parallel()
	const cases = 100
	whats := []struct {
		what string
		want int
	}{
		{"a link that a record or an operation changes", 50}, {"two operations", 33}, {"an operation that removes for good", 4},
		{"an operation of an upload-only remote", 15}, {"an operation of a download-only remote", 11}, {"a quarantine", 22},
		{"a move or a copy", 7}, {"a Live Photo video or RAW file linked after the run", 17},
	}
	met := make([]int, len(whats))
	for seed := range cases {
		c := genCases().Example(seed)
		seen := make([]bool, len(whats))
		for _, in := range []plan.Input{c.first, c.in} {
			p, delta, err := plan.Build(in)
			if err != nil {
				t.Fatalf("seed %d: Build: %v", seed, err)
			}
			next := after(in, p, delta)
			has := func(kinds ...domain.OpKind) bool {
				return slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return slices.Contains(kinds, o.Kind) })
			}
			for i, ok := range []bool{
				len(p.Links) > 0 || slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return len(o.Links) > 0 }),
				len(p.Ops) > 1,
				slices.ContainsFunc(p.Ops, func(o domain.Op) bool { return o.Irreversible }),
				len(p.Ops) > 0 && in.Remotes[0].Mode == domain.ModeUploadOnly,
				len(p.Ops) > 0 && in.Remotes[0].Mode == domain.ModeDownloadOnly,
				has(domain.OpQuarantine),
				has(domain.OpMove, domain.OpCopy),
				in.Remotes[0].Mode != domain.ModeDownloadOnly && slices.ContainsFunc(next.State.Links, func(l domain.Link) bool {
					return l.Status == domain.StatusActive && (l.Role == domain.RoleLiveVideo || l.Role == domain.RoleRAWPair) &&
						!slices.ContainsFunc(next.State.Conflicts, func(c domain.Conflict) bool { return c.AssetID == l.AssetID && c.Remote == l.Remote })
				}),
			} {
				seen[i] = seen[i] || ok
			}
		}
		for i, ok := range seen {
			if ok {
				met[i]++
			}
		}
	}
	var missing []string
	for i, w := range whats {
		t.Logf("%d of %d cases have %s", met[i], cases, w.what)
		if met[i] < w.want {
			missing = append(missing, fmt.Sprintf("%d cases with %s (%d are)", w.want, w.what, met[i]))
		}
	}
	if len(missing) > 0 {
		t.Errorf("the cases of seeds 0 to %d lack %s", cases-1, strings.Join(missing, "; "))
	}
}
