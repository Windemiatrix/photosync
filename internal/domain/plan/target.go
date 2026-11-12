package plan

import (
	"cmp"
	"maps"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// SettleConflict returns o, what the files of a remote make of an asset
// whose decisions there take part in the plan, with each conflict given to
// its winner: resolution, the choice of the user for the conflict of the
// asset in that remote, when set, else policy. Under remote_wins every
// ActionConflict becomes ActionMoveAsset, the asset following its file;
// under icloud_wins it becomes ActionMoveFile, the file following the asset,
// and the albums that o joins and leaves by R-LINK-07 are dropped. Rules stay
// as they are, and so does o under any other policy. o is not changed.
func SettleConflict(o domain.ObjectDecision, resolution, policy domain.ConflictPolicy) domain.ObjectDecision {
	var winner domain.LinkAction
	switch cmp.Or(resolution, policy) {
	case domain.ConflictRemoteWins:
		winner = domain.ActionMoveAsset
	case domain.ConflictICloudWins:
		winner = domain.ActionMoveFile
	default:
		return o
	}
	out := domain.ObjectDecision{Links: slices.Clone(o.Links), Join: slices.Clone(o.Join), Leave: slices.Clone(o.Leave)}
	for i, d := range out.Links {
		if d.Action == domain.ActionConflict {
			out.Links[i].Action = winner
		}
	}
	if winner == domain.ActionMoveFile {
		byConflict := func(c domain.AlbumChange) bool { return c.Rule == "R-LINK-07" }
		out.Join, out.Leave = slices.DeleteFunc(out.Join, byConflict), slices.DeleteFunc(out.Leave, byConflict)
	}
	return out
}

// DecideTarget returns target, the asset as iCloud holds it once the run is
// applied, and the albums that it joins and leaves on the way there from now,
// the asset of the snapshot reconciled with the state. first is the first
// placement of now as DecideFirstPlacement gives it, zero for an asset with a
// link. objects are, by the name of the remote, what the files of each remote
// where the decisions on now take part in the plan make of it, with
// SettleConflict applied: Links in the order of link ID, and Join and Leave
// in that of album ID and then rule, as DecideObject gives them. links are as
// DecidePlacement takes them. The changes of first come before those of the
// remotes, which come in the order of priority, the remotes that it does not
// name last, in the order of their names. An album changes as the first
// change that joins or leaves it says, and a joined one has FirstSeenRun
// runID. A file of the original or the source role whose decision is
// ActionMoveAsset and that links has in a root other than that of its base
// makes that root the TargetLibrary of target, the first such file in that
// order; with none, it is that of now. A change that finds now as it says,
// such as joining an album that now is in, is in neither join nor leave. join
// and leave are in the order of album ID, as are the Memberships of target,
// and the inputs are not changed.
func DecideTarget(now domain.Asset, first domain.ObjectDecision, objects map[string]domain.ObjectDecision, links []domain.Link, priority []string, runID string) (target domain.Asset, join, leave []domain.AlbumChange) {
	names := slices.Sorted(maps.Keys(objects))
	rank := func(name string) int {
		if i := slices.Index(priority, name); i >= 0 {
			return i
		}
		return len(priority)
	}
	slices.SortStableFunc(names, func(x, y string) int { return cmp.Compare(rank(x), rank(y)) })
	order := []domain.ObjectDecision{first}
	for _, name := range names {
		order = append(order, objects[name])
	}
	target = now
	target.Memberships = slices.Clone(now.Memberships)
	in := func(id domain.AlbumID) int {
		return slices.IndexFunc(target.Memberships, func(m domain.Membership) bool { return m.AlbumID == id })
	}
	decided, moved := map[domain.AlbumID]bool{}, false
	for _, o := range order {
		for _, c := range o.Join {
			if !decided[c.AlbumID] && in(c.AlbumID) < 0 {
				target.Memberships = append(target.Memberships, domain.Membership{AlbumID: c.AlbumID, FirstSeenRun: runID})
				join = append(join, c)
			}
			decided[c.AlbumID] = true
		}
		for _, c := range o.Leave {
			if i := in(c.AlbumID); !decided[c.AlbumID] && i >= 0 {
				target.Memberships = slices.Delete(target.Memberships, i, i+1)
				leave = append(leave, c)
			}
			decided[c.AlbumID] = true
		}
		for _, d := range o.Links {
			i := slices.IndexFunc(links, func(l domain.Link) bool { return l.ID == d.Link.ID })
			if !moved && i >= 0 && d.Action == domain.ActionMoveAsset && objectFile(d.Link) && links[i].Root != d.Link.Root {
				target.TargetLibrary, moved = links[i].Root, true
			}
		}
	}
	slices.SortFunc(target.Memberships, func(x, y domain.Membership) int { return cmp.Compare(x.AlbumID, y.AlbumID) })
	return target, objectChanges(join), objectChanges(leave)
}
