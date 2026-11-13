package plan

import (
	"cmp"
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// DecideComponents returns what a run makes of the Live Photo videos and RAW
// files of an asset, which follow the files of the original or the source
// role that they lie beside. decisions, status and placements are as
// DecideLayout gives them, links are the links of the asset in the state in
// every remote, role and status, files gives by link ID the entry of the scan
// of its remote where the file of a link lies now, as ReadScan gives it, and
// remotes are those of the configuration.
//
// fates gives, by link ID, what becomes of each file of decisions with the
// original or the source role and an active link in the state, as
// ComponentAction takes it: ActionDeleteFile when its file is gone or the run
// leaves its link other than active; else the action of a placement that
// moves it or takes it to quarantine; else ActionMoveFile when the user moved
// it; else ActionNone. Each active link of a component in a remote of
// decisions gets one decision in components, under the link ID of the file of
// fates that ComponentBeside finds it beside in the state, 0 for none, whose
// fate is then ActionDeleteFile: that of ComponentAction on how its file
// changed, as RemoteSide says with a file found elsewhere moved, and on that
// fate, same holding when no placement moves that file and the component lies
// beside it as both lie now; its iCloud side is that of the decision on that
// file, unchanged for none. An action that the mode of the remote does not
// run becomes ActionUpdateBase when the file of the component moved or has
// new content, else ActionNone, by R-MODE-01 or R-MODE-02, and then
// StatusDeletes applies deletes. after gives, by link ID, the status that
// each decision leaves its link in, "" when it follows a file to quarantine
// by a placement. copies gives, under the same key, the placement of the
// component into the album of each copy that placements make of that file, as
// ComponentAction gives it for ActionCopyFile, with Link where the component
// lies now, or as in the state when gone, an upload too. The lists are in the
// order of the link ID of the component, those of copies then in the order of
// placements, and the inputs are not changed.
func DecideComponents(decisions map[string][]domain.LinkDecision, status map[domain.LinkID]domain.LinkStatus, placements map[string][]domain.Placement, links []domain.Link, files map[domain.LinkID]domain.RemoteEntry, remotes []domain.RemoteSpec, deletes domain.DeletesPolicy) (fates map[domain.LinkID]domain.LinkAction, components map[domain.LinkID][]domain.LinkDecision, copies map[domain.LinkID][]domain.Placement, after map[domain.LinkID]domain.LinkStatus) {
	now := func(l domain.Link) domain.Link {
		if e, ok := files[l.ID]; ok {
			l.Root, l.Path = e.Root, e.Path
		}
		return l
	}
	fates, ends := map[domain.LinkID]domain.LinkAction{}, map[domain.LinkID]domain.Link{}
	for name, ds := range decisions {
		for _, d := range ds {
			if !objectFile(d.Link) {
				continue
			}
			fate, end := domain.ActionNone, now(d.Link)
			if d.Remote == domain.ChangeMoved {
				fate = domain.ActionMoveFile
			}
			for _, pl := range placements[name] {
				if pl.Link.ID == d.Link.ID && pl.Action != domain.ActionCopyFile {
					fate, end = pl.Action, domain.Link{}
				}
			}
			if d.Remote == domain.ChangeDeleted || status[d.Link.ID] != domain.StatusActive {
				fate = domain.ActionDeleteFile
			}
			fates[d.Link.ID], ends[d.Link.ID] = fate, end
		}
	}
	specs, rules := map[string]domain.RemoteSpec{}, map[domain.Mode]string{domain.ModeUploadOnly: "R-MODE-01", domain.ModeDownloadOnly: "R-MODE-02"}
	for _, r := range remotes {
		specs[r.Name] = r
	}
	components, copies, after = map[domain.LinkID][]domain.LinkDecision{}, map[domain.LinkID][]domain.Placement{}, map[domain.LinkID]domain.LinkStatus{}
	for _, c := range slices.SortedFunc(slices.Values(links), func(x, y domain.Link) int { return cmp.Compare(x.ID, y.ID) }) {
		ds, in := decisions[c.Remote]
		if !in || c.Status != domain.StatusActive || c.Role != domain.RoleLiveVideo && c.Role != domain.RoleRAWPair {
			continue
		}
		var main domain.LinkID
		r, fate, icloud := specs[c.Remote], domain.ActionDeleteFile, domain.ChangeUnchanged
		for _, d := range ds {
			if f, ok := fates[d.Link.ID]; ok && ComponentBeside(d.Link, c) {
				main, fate, icloud = d.Link.ID, f, d.ICloud
			}
		}
		e, there := files[c.ID]
		at := there && e.Root == c.Root && e.Path == c.Path
		side := RemoteSide(c, e, at, r.Capabilities.ContentHash)
		if there && !at {
			side = domain.ChangeMoved
		}
		decide := func(f domain.LinkAction, same bool) (domain.LinkDecision, domain.LinkStatus) {
			d := domain.LinkDecision{Link: c, ICloud: icloud, Remote: side}
			d.Action, d.Rule, _ = ComponentAction(side, f, same)
			if !ModeAllows(r.Mode, d.Action) {
				d.Action, d.Rule = domain.ActionNone, rules[r.Mode]
				if side == domain.ChangeMoved || side == domain.ChangeReplaced {
					d.Action = domain.ActionUpdateBase
				}
			}
			return StatusDeletes(d, deletes)
		}
		d, st := decide(fate, ComponentBeside(ends[main], now(c)))
		// A needless copy goes to quarantine without a link (R-OBJECT-04), and
		// so do the files that follow it there.
		if fate == domain.ActionQuarantineFile {
			st = ""
		}
		components[main], after[c.ID] = append(components[main], d), st
		for _, pl := range placements[c.Remote] {
			if pl.Action == domain.ActionCopyFile && pl.Link.ID == main {
				cd, _ := decide(domain.ActionCopyFile, false)
				copies[main] = append(copies[main], domain.Placement{Action: cd.Action, Link: now(c), AlbumID: pl.AlbumID, Rule: cd.Rule})
			}
		}
	}
	return fates, components, copies, after
}
