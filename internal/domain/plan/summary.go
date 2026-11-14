package plan

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

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
	// Levels counts by level the matched links of a plan: those with a
	// nonzero ID that the state lacks, as only matching gives a new link an
	// ID. Such a link at the level uploaded is the upload that replaces the
	// file of a matched one, and counts in none of Levels, Samples and Copies.
	Levels map[domain.MatchLevel]int
	// Samples holds, for the levels same_time_size and probable, the IDs of at
	// most 50 matched links, the lowest, in increasing order, for the user to
	// check the matches that are least sure.
	Samples map[domain.MatchLevel][]domain.LinkID
	// Copies is the number of matched links with the original or the source
	// role that give an asset another file in a remote, whatever the plan
	// does with it: each of them where the state holds an active link of
	// those roles of the asset there, and all but one elsewhere.
	Copies int
	// Detached is the number of links of the state that the plan leaves
	// detached, out of sync until the user relinks them.
	Detached int
	// Returned is the number of links of the state that the plan makes active
	// again: quarantined links whose side the user brought back.
	Returned int
	// Closed is the number of conflicts that the plan resolves, by the choice
	// of the user or by themselves.
	Closed int
}

// Summarize returns p in numbers, where delta is what Build gives with p,
// which Limit may have cut, and state is what Build planned p from. Shared
// and Captions read the asset of an operation in delta, and one that delta
// lacks, as one gone from Photos, counts in neither; Unavailable counts each
// asset of delta, whatever p holds. The links are those of the operations of
// p and of its Links, and the conflicts those of p, so that a plan that
// Limit cut lacks those of the assets that it cut off. The maps are empty
// rather than nil when nothing counts, and the inputs are not changed.
func Summarize(p domain.Plan, delta domain.SnapshotDelta, state domain.State) Summary {
	s := Summary{
		ICloud: map[domain.OpKind]Tally{}, Remotes: map[string]map[domain.OpKind]Tally{}, Rules: map[string]int{}, Notices: map[string]int{},
		Levels: map[domain.MatchLevel]int{}, Samples: map[domain.MatchLevel][]domain.LinkID{},
	}
	assets := map[domain.AssetID]domain.Asset{}
	for _, a := range delta.Assets {
		assets[a.ID] = a
		if a.Missing && (a.InCloud == nil || !*a.InCloud) {
			s.Unavailable++
		}
	}
	changed, mass := map[domain.AssetID]bool{}, map[domain.AssetID]bool{}
	var links []domain.Link
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
		links = append(links, o.Links...)
	}
	s.Assets += len(changed)
	s.Mass = len(mass)
	before, held := map[domain.LinkID]domain.LinkStatus{}, map[domain.Link]bool{}
	key := func(l domain.Link) domain.Link { return domain.Link{AssetID: l.AssetID, Remote: l.Remote} }
	for _, l := range state.Links {
		before[l.ID] = l.Status
		if objectFile(l) {
			held[key(l)] = true
		}
	}
	for _, l := range append(links, p.Links...) {
		was, known := before[l.ID]
		switch {
		case l.ID == 0, !known && l.Level == domain.LevelUploaded:
		case !known:
			s.Levels[l.Level]++
			if l.Level == domain.LevelSameTimeSize || l.Level == domain.LevelProbable {
				s.Samples[l.Level] = append(s.Samples[l.Level], l.ID)
			}
			if l.Role == domain.RoleOriginal || l.Role == domain.RoleSource {
				if held[key(l)] {
					s.Copies++
				}
				held[key(l)] = true
			}
		case l.Status == domain.StatusDetached:
			s.Detached++
		case l.Status == domain.StatusActive && was != domain.StatusActive:
			s.Returned++
		}
	}
	for level, ids := range s.Samples {
		slices.Sort(ids)
		s.Samples[level] = ids[:min(len(ids), 50)]
	}
	for _, c := range p.Conflicts {
		if c.Status != domain.ConflictStatusResolved {
			s.Unsettled++
		} else {
			s.Closed++
		}
	}
	for _, n := range p.Notices {
		s.Notices[n.Code]++
	}
	return s
}
