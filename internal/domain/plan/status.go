package plan

import "github.com/Windemiatrix/photosync/internal/domain"

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
