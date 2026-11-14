package plan

import "github.com/Windemiatrix/photosync/internal/domain"

// Tally is a number of operations and the bytes that they transfer: the sizes
// of the links of each OpPut and OpImport among them. The cloud moves, copies
// and deletes a file in place, so the other kinds transfer none.
type Tally struct {
	Count int
	Bytes int64
}

// Summary is a plan in numbers, as Summarize gives it.
type Summary struct {
	// ICloud counts by kind the operations on iCloud, those with no Remote and
	// the imports of new assets.
	ICloud map[domain.OpKind]Tally
	// Replaced counts the imports that replace an asset, which ICloud leaves out.
	Replaced Tally
	// Remotes counts the other operations, those on the cloud, by remote and kind.
	Remotes map[string]map[domain.OpKind]Tally
	// Assets is the number of assets that the plan changes, as Limit counts
	// them: each that an operation names, and a new one for each import that
	// names none.
	Assets int
	// Irreversible is the number of OpDelete, each of which deletes a file of
	// the cloud for good. It is not that of the operations with Irreversible:
	// an OpExpire deletes only what quarantine has kept as long as the policy
	// says, and an OpAssetDelete leaves its asset in Recently Deleted.
	Irreversible int
	// NoUndo is the number of operations that undo cannot take back by
	// itself: those with Irreversible, and each OpAssetDelete, whose asset
	// only the user can bring back from Recently Deleted.
	NoUndo int
	// Mass is the number of assets whose files the plan deletes or moves to
	// quarantine, or which it takes to Recently Deleted, as it takes each
	// asset that it replaces.
	Mass int
	// Shared is the number of OpAssetDelete of assets of the shared library,
	// which every member of that library sees go.
	Shared int
	// Captions is the number of imports that replace an asset with a caption
	// or keywords, which the asset that they import lacks.
	Captions int
	// Unavailable is the number of assets whose original is neither on this
	// Mac nor in iCloud, so that they do not sync.
	Unavailable int
	// Unsettled is the number of conflicts that the plan leaves open or skipped.
	Unsettled int
	// Rules counts the operations by Rule, and Notices the notices by Code.
	Rules, Notices map[string]int
}

// Summarize returns p in numbers, where delta is what Build gives with p,
// which Limit may have cut. Shared and Captions read the asset of an
// operation in delta, and one that delta lacks, as one gone from Photos,
// counts in neither; Unavailable counts each asset of delta, whatever p
// holds. The conflicts are those of p, so that a plan that Limit cut lacks
// those of the assets that it cut off. The maps are empty rather than nil
// when nothing counts, and the inputs are not changed.
func Summarize(p domain.Plan, delta domain.SnapshotDelta) Summary {
	s := Summary{ICloud: map[domain.OpKind]Tally{}, Remotes: map[string]map[domain.OpKind]Tally{}, Rules: map[string]int{}, Notices: map[string]int{}}
	assets := map[domain.AssetID]domain.Asset{}
	for _, a := range delta.Assets {
		assets[a.ID] = a
		if a.Missing && (a.InCloud == nil || !*a.InCloud) {
			s.Unavailable++
		}
	}
	changed, mass := map[domain.AssetID]bool{}, map[domain.AssetID]bool{}
	for _, o := range p.Ops {
		var bytes int64
		if o.Kind == domain.OpPut || o.Kind == domain.OpImport {
			for _, l := range o.Links {
				bytes += l.Size
			}
		}
		count := func(t Tally) Tally { return Tally{Count: t.Count + 1, Bytes: t.Bytes + bytes} }
		switch {
		case o.Kind == domain.OpImport && o.Asset != 0:
			s.Replaced = count(s.Replaced)
			if assets[o.Asset].HasCaptionOrKeywords {
				s.Captions++
			}
		case o.Remote == "" || o.Kind == domain.OpImport:
			s.ICloud[o.Kind] = count(s.ICloud[o.Kind])
		default:
			if s.Remotes[o.Remote] == nil {
				s.Remotes[o.Remote] = map[domain.OpKind]Tally{}
			}
			s.Remotes[o.Remote][o.Kind] = count(s.Remotes[o.Remote][o.Kind])
		}
		switch {
		case o.Asset != 0:
			changed[o.Asset] = true
		case o.Kind == domain.OpImport:
			s.Assets++
		}
		if o.Asset != 0 && (o.Kind == domain.OpDelete || o.Kind == domain.OpQuarantine || o.Kind == domain.OpAssetDelete) {
			mass[o.Asset] = true
		}
		if o.Kind == domain.OpDelete {
			s.Irreversible++
		}
		if o.Irreversible || o.Kind == domain.OpAssetDelete {
			s.NoUndo++
		}
		if o.Kind == domain.OpAssetDelete && assets[o.Asset].Library == domain.LibraryShared {
			s.Shared++
		}
		s.Rules[o.Rule]++
	}
	s.Assets += len(changed)
	s.Mass = len(mass)
	for _, c := range p.Conflicts {
		if c.Status != domain.ConflictStatusResolved {
			s.Unsettled++
		}
	}
	for _, n := range p.Notices {
		s.Notices[n.Code]++
	}
	return s
}
