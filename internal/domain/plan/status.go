package plan

import (
	"slices"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// StatusDeletes returns d, a decision of ModeDecide or of the reduction of
// the decisions on the links of an asset, with the deletes policy applied,
// and the status of its link after the run, "" when the run leaves no link.
// Under quarantine, ActionDeleteFile becomes ActionQuarantineFile with a
// quarantined_remote link (R-STATUS-01) and ActionDeleteAsset stays with a
// quarantined_icloud link (R-STATUS-02); under none, either becomes
// ActionNone with a detached link (R-STATUS-05); under propagate, either
// stays and leaves no link (R-STATUS-06); any other policy counts as
// quarantine. ActionForget leaves no link. ActionNone leaves the link
// detached when a side is deleted and leaves no link when a side is
// ChangeNone. Any other decision stays as it is, a replacement under every
// policy too, with an active link.
func StatusDeletes(d domain.LinkDecision, deletes domain.DeletesPolicy) (domain.LinkDecision, domain.LinkStatus) {
	switch {
	case d.Action == domain.ActionDeleteFile || d.Action == domain.ActionDeleteAsset:
		switch deletes {
		case domain.DeletesNone:
			d.Action, d.Rule = domain.ActionNone, "R-STATUS-05"
			return d, domain.StatusDetached
		case domain.DeletesPropagate:
			d.Rule = "R-STATUS-06"
			return d, ""
		}
		if d.Action == domain.ActionDeleteFile {
			d.Action, d.Rule = domain.ActionQuarantineFile, "R-STATUS-01"
			return d, domain.StatusQuarantinedRemote
		}
		d.Rule = "R-STATUS-02"
		return d, domain.StatusQuarantinedICloud
	case d.Action == domain.ActionForget:
		return d, ""
	case d.Action != domain.ActionNone:
		return d, domain.StatusActive
	case d.ICloud == domain.ChangeDeleted || d.Remote == domain.ChangeDeleted:
		return d, domain.StatusDetached
	case d.ICloud == domain.ChangeNone || d.Remote == domain.ChangeNone:
		return d, ""
	}
	return d, domain.StatusActive
}

// StatusSides returns the changes of the two sides of a link in status
// quarantined_icloud or quarantined_remote once the user brings one back
// (R-STATUS-04): a side back is ChangeNew and a side still away ChangeNone,
// so that the link, active again, follows the rules for new assets and
// files. found reports that the snapshot holds the asset of the link, and
// inTrash that the asset is in Recently Deleted; it is back when found
// outside it. file is how RemoteSide finds the file at the path of the link;
// it is back when ChangeUnchanged. With neither back, a link quarantined in
// iCloud whose asset is gone from the library is ChangeDeleted on both
// sides; otherwise ok is false, both sides are ChangeNone and the link stays
// as it is (R-STATUS-03), as does a link in any other status. The policies
// do not matter here: see Syncs.
func StatusSides(status domain.LinkStatus, found, inTrash bool, file domain.ChangeKind) (icloud, remote domain.ChangeKind, ok bool) {
	if status != domain.StatusQuarantinedICloud && status != domain.StatusQuarantinedRemote {
		return domain.ChangeNone, domain.ChangeNone, false
	}
	// The deletes policy, not the user, took away the side still away: as a
	// deletion, a one-way remote would detach the link or delete the side
	// brought back again.
	icloud, remote = domain.ChangeNone, domain.ChangeNone
	if found && !inTrash {
		icloud = domain.ChangeNew
	}
	if file == domain.ChangeUnchanged {
		remote = domain.ChangeNew
	}
	switch {
	case icloud == domain.ChangeNew || remote == domain.ChangeNew:
		return icloud, remote, true
	case !found && status == domain.StatusQuarantinedICloud:
		return domain.ChangeDeleted, domain.ChangeDeleted, true
	}
	return icloud, remote, false
}

// StatusFrozen reports whether the asset of links is out of sync with remote
// until the user relinks it (R-STATUS-05): a link of it there with the
// original or the source role is detached. links are the links of the asset
// in every remote, each in the status that the run leaves it in. A frozen
// asset gets no decisions and no placements in remote and is not new there;
// a detached link of another role freezes nothing.
func StatusFrozen(links []domain.Link, remote string) bool {
	return slices.ContainsFunc(links, func(l domain.Link) bool {
		return l.Remote == remote && l.Status == domain.StatusDetached &&
			(l.Role == domain.RoleOriginal || l.Role == domain.RoleSource)
	})
}
