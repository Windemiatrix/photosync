package plan_test

import (
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Written from the deletes policy and the link statuses, not from the code:
// a deletion follows the policy, an unknown policy quarantines rather than
// deletes for good, a replacement follows no policy, and a decision that does
// nothing keeps its link as its sides say. Every row runs under each policy.
func TestStatusDeletes(t *testing.T) {
	t.Parallel()
	const (
		none, added, unchanged   = domain.ChangeNone, domain.ChangeNew, domain.ChangeUnchanged
		moved, replaced, deleted = domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted
	)
	type outcome struct {
		action domain.LinkAction
		rule   string
		status domain.LinkStatus
	}
	policies := []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate, "keep"}
	decision := func(icloud, remote domain.ChangeKind, a domain.LinkAction, rule string) domain.LinkDecision {
		return domain.LinkDecision{ICloud: icloud, Remote: remote, Action: a, Rule: rule}
	}
	kept := func(d domain.LinkDecision, status domain.LinkStatus) [4]outcome {
		o := outcome{d.Action, d.Rule, status}
		return [4]outcome{o, o, o, o}
	}
	detached := outcome{domain.ActionNone, "R-STATUS-05", domain.StatusDetached}
	fileOut := outcome{domain.ActionQuarantineFile, "R-STATUS-01", domain.StatusQuarantinedRemote}
	assetOut := outcome{domain.ActionDeleteAsset, "R-STATUS-02", domain.StatusQuarantinedICloud}
	assetGone := outcome{domain.ActionDeleteAsset, "R-STATUS-06", ""}
	replace := decision(unchanged, replaced, domain.ActionReplaceAsset, "R-LINK-06")
	moveReplace := decision(moved, replaced, domain.ActionMoveFileReplaceAsset, "R-LINK-14")
	forget := decision(deleted, deleted, domain.ActionForget, "R-LINK-12")
	uploadOnlyGone := decision(unchanged, deleted, domain.ActionNone, "R-MODE-01")
	downloadOnlyGone := decision(deleted, moved, domain.ActionNone, "R-MODE-02")
	reported := decision(none, added, domain.ActionNone, "R-MODE-01")
	notUploaded := decision(added, none, domain.ActionNone, "R-MODE-02")
	nothing := decision(unchanged, unchanged, domain.ActionNone, "")
	reupload := decision(moved, deleted, domain.ActionUpload, "R-LINK-10")
	reimport := decision(deleted, replaced, domain.ActionImport, "R-LINK-11")
	for _, c := range []struct {
		name string
		d    domain.LinkDecision
		want [4]outcome // none, quarantine, propagate, unknown
	}{
		{"an asset deleted in iCloud: its file", decision(deleted, unchanged, domain.ActionDeleteFile, "R-LINK-08"), [4]outcome{
			detached, // covers: R-STATUS-05
			fileOut,  // covers: R-STATUS-01
			{domain.ActionDeleteFile, "R-STATUS-06", ""}, // covers: R-STATUS-06
			fileOut,
		}},
		{"a file deleted in the cloud: its asset", decision(unchanged, deleted, domain.ActionDeleteAsset, "R-LINK-09"), [4]outcome{
			detached, assetOut, assetGone, assetOut, // covers: R-STATUS-02
		}},
		{"every file of an asset gone, whichever rule chose the deletion", decision(unchanged, deleted, domain.ActionDeleteAsset, "R-OBJECT-01"), [4]outcome{
			detached, assetOut, assetGone, assetOut,
		}},
		{"an asset replaced from its file", replace, kept(replace, domain.StatusActive)}, // covers: R-STATUS-08
		{"a file moved and its asset replaced", moveReplace, kept(moveReplace, domain.StatusActive)},
		{"deleted on both sides", forget, kept(forget, "")},
		{"a file deleted in an upload-only remote", uploadOnlyGone, kept(uploadOnlyGone, domain.StatusDetached)},
		{"an asset deleted in iCloud for a download-only remote", downloadOnlyGone, kept(downloadOnlyGone, domain.StatusDetached)},
		{"a new file that an upload-only remote only reports", reported, kept(reported, "")},
		{"a new asset that a download-only remote does not upload", notUploaded, kept(notUploaded, "")},
		{"nothing changed", nothing, kept(nothing, domain.StatusActive)},
		{"a change in iCloud beats a deletion in the cloud", reupload, kept(reupload, domain.StatusActive)},
		{"a change in the cloud beats a deletion in iCloud", reimport, kept(reimport, domain.StatusActive)},
	} {
		for k, deletes := range policies {
			got, status := plan.StatusDeletes(c.d, deletes)
			w := c.want[k]
			if got.Action != w.action || got.Rule != w.rule || status != w.status || got.ICloud != c.d.ICloud || got.Remote != c.d.Remote {
				t.Errorf("%s under %q: StatusDeletes = %s by %q, link %q; want %s by %q, link %q", c.name, deletes, got.Action, got.Rule, status, w.action, w.rule, w.status)
			}
			if _, ok := domain.Rules[w.rule]; w.rule != "" && !ok {
				t.Errorf("%s is not in domain.Rules", w.rule)
			}
		}
	}
}
