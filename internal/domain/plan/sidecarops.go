package plan

import (
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// SidecarOps returns the operations of a run on the sidecars of the files of
// the original or the source role of asset id in the remote of names, which
// follow those files, and notices of those that find their path taken. mains
// are the operations that FileOps gives for the asset in that remote, fates
// is as DecideComponents gives it, sidecars are as Sidecars gives them for
// that remote and its whole listing, files gives by link ID the entry of the
// scan where the file of a link lies now, as ReadScan gives it, names are
// the names of the remote in the run, which the operations take, mode is the
// mode of that remote, deletes the deletes policy and now the day of
// quarantine. The files of fates count by increasing link ID, and the
// sidecars of each in the order of Path.
//
// The sidecars of a file take its fate under R-COMPONENT-04, as StatusDeletes
// gives it under deletes, and nothing when ModeAllows does not run that
// action in mode. ActionMoveFile moves each (OpMove) to SidecarPath from
// where the file ends up, under that root: the link of the file as an
// operation of mains leaves it, else where the file lies; one that already
// lies beside the file there, by the root, the directory and the stem that
// Sidecars goes by, stays. ActionQuarantineFile moves each to the path in
// quarantine that Names.Quarantine gives it (OpQuarantine), and
// ActionDeleteFile removes each (OpDelete), irreversibly. Other actions give
// nothing. A destination of a move that names already hold gives the notice
// "plan.path_taken" with the remote, the root and the path instead of an
// operation, and names take every other. An operation names asset id and
// holds no links, since a sidecar has none. The operations are in the order
// of domain.Op.Compare, the notices in that of the sidecars, and the inputs
// but names are not changed.
func SidecarOps(names *Names, id domain.AssetID, mains []domain.Op, fates map[domain.LinkID]domain.LinkAction, sidecars map[domain.LinkID][]domain.RemoteEntry, files map[domain.LinkID]domain.RemoteEntry, mode domain.Mode, deletes domain.DeletesPolicy, now time.Time) (ops []domain.Op, notices []domain.Notice) {
	add := func(kind domain.OpKind, s domain.RemoteEntry, root domain.LibraryKind, dst, rule string) {
		ops = append(ops, domain.Op{
			Kind: kind, Remote: names.remote, Root: s.Root, Src: s.Path, DstRoot: root, Dst: dst,
			Asset: id, Rule: rule, Irreversible: kind == domain.OpDelete,
		})
	}
	for _, m := range slices.Sorted(maps.Keys(fates)) {
		d, _ := StatusDeletes(domain.LinkDecision{Action: fates[m], Rule: "R-COMPONENT-04"}, deletes)
		end := mainEnd(domain.Link{}, m, mains, files)
		for _, s := range slices.SortedFunc(slices.Values(sidecars[m]), func(x, y domain.RemoteEntry) int {
			return strings.Compare(x.Path, y.Path)
		}) {
			switch dst := SidecarPath(end.Path, s.Path); {
			case !ModeAllows(mode, d.Action):
			case d.Action == domain.ActionQuarantineFile:
				add(domain.OpQuarantine, s, s.Root, names.Quarantine(s.Root, now, s.Path), d.Rule)
			case d.Action == domain.ActionDeleteFile:
				add(domain.OpDelete, s, "", "", d.Rule)
			case d.Action != domain.ActionMoveFile:
			// Beside a file that the user renamed in place, a sidecar goes with
			// it already: its new name would differ in case alone, if at all.
			case sidecarKey(end.Root, end.Path) == sidecarKey(s.Root, s.Path):
			// Free gives a file that the run moves a free stem, but where the user
			// moved one a file of the user may hold the name, and two sidecars
			// whose names differ in case alone get one.
			case names.paths[end.Root][dst]:
				notices = append(notices, domain.Notice{Code: "plan.path_taken", Params: []string{names.remote, string(end.Root), dst}})
			default:
				names.take(end.Root, dst)
				add(domain.OpMove, s, end.Root, dst, d.Rule)
			}
		}
	}
	slices.SortFunc(ops, domain.Op.Compare)
	return ops, notices
}
