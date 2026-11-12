package plan_test

import (
	"slices"
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

// Written from the quarantine rules, not from the code: a quarantined link
// stays as it is until the user brings a side back, and then starts again as
// a link without a base, the side back new and the other none. A link
// quarantined in iCloud ends once its asset is gone for good, while one
// quarantined in the cloud waits for its quarantine to expire. A side
// brought back is deleted again in no mode under no deletes policy and
// detaches no link, and a two-way remote keeps the link active.
func TestStatusSides(t *testing.T) {
	t.Parallel()
	const (
		none, added, deleted = domain.ChangeNone, domain.ChangeNew, domain.ChangeDeleted
		unchanged, replaced  = domain.ChangeUnchanged, domain.ChangeReplaced
		inICloud, inCloud    = domain.StatusQuarantinedICloud, domain.StatusQuarantinedRemote
	)
	type asset struct{ found, inTrash bool }
	trashed, back, gone := asset{true, true}, asset{true, false}, asset{false, false}
	type key struct {
		status domain.LinkStatus
		asset
		file domain.ChangeKind
	}
	listed := map[key]linkPair{
		{inICloud, trashed, unchanged}: {none, added}, // covers: R-STATUS-04
		{inICloud, back, replaced}:     {added, none},
		{inICloud, back, deleted}:      {added, none},
		{inICloud, back, unchanged}:    {added, added},
		{inICloud, gone, replaced}:     {deleted, deleted},
		{inICloud, gone, deleted}:      {deleted, deleted}, // covers: R-STATUS-02
		{inICloud, gone, unchanged}:    {none, added},
		{inCloud, trashed, unchanged}:  {none, added},
		{inCloud, back, replaced}:      {added, none},
		{inCloud, back, deleted}:       {added, none},
		{inCloud, back, unchanged}:     {added, added},
		{inCloud, gone, unchanged}:     {none, added},
	}
	for _, s := range []domain.LinkStatus{inICloud, inCloud, domain.StatusDetached, domain.StatusActive} {
		for _, a := range []asset{trashed, back, gone} {
			for _, file := range []domain.ChangeKind{unchanged, replaced, deleted} {
				w, revived := listed[key{s, a, file}]
				if !revived {
					w = linkPair{none, none} // covers: R-STATUS-03
				}
				icloud, remote, ok := plan.StatusSides(s, a.found, a.inTrash, file)
				if icloud != w.icloud || remote != w.remote || ok != revived {
					t.Errorf("StatusSides(%s, found %v, in Recently Deleted %v, file %s) = %q, %q, %v; want %q, %q, %v",
						s, a.found, a.inTrash, file, icloud, remote, ok, w.icloud, w.remote, revived)
				}
			}
		}
	}
	lost := []domain.LinkAction{domain.ActionDeleteFile, domain.ActionDeleteAsset, domain.ActionQuarantineFile}
	for k, w := range listed {
		if w.icloud != added && w.remote != added {
			continue
		}
		for _, m := range []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly} {
			for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate} {
				d, decided := plan.ModeDecide(m, w.icloud, w.remote, false)
				got, status := plan.StatusDeletes(d, deletes)
				if !decided || slices.Contains(lost, got.Action) || status == domain.StatusDetached ||
					m == domain.ModeTwoWay && status != domain.StatusActive {
					t.Errorf("%s, found %v, in Recently Deleted %v, file %s, then %s under %s: %s by %q, link %q; want no deletion, no detached link and, two-way, an active one",
						k.status, k.found, k.inTrash, k.file, m, deletes, got.Action, got.Rule, status)
				}
			}
		}
	}
}

// Written from the rule on detached links, not from the code: an asset whose
// file or source a remote holds detached is out of sync with that remote,
// whatever its other links there, until the user relinks it. A detached
// component, a quarantined link or a link of another remote freezes nothing.
func TestStatusFrozen(t *testing.T) {
	t.Parallel()
	const (
		personal, shared       = domain.LibraryPersonal, domain.LibraryShared
		original, source, live = domain.RoleOriginal, domain.RoleSource, domain.RoleLiveVideo
		active, detached       = domain.StatusActive, domain.StatusDetached
	)
	link := func(id domain.LinkID, remote string, root domain.LibraryKind, role domain.LinkRole, status domain.LinkStatus) domain.Link {
		return domain.Link{
			ID: id, AssetID: 7, Remote: remote, Root: root, Path: "2024/05-01 - Holidays/IMG_0001.HEIC",
			Role: role, AlbumID: 10, Status: status,
		}
	}
	file := link(1, "disk", personal, original, active)
	for _, c := range []struct {
		name  string
		links []domain.Link
		want  bool
	}{
		{"no links", nil, false},
		{"an active file", []domain.Link{file}, false},
		{"a detached file", []domain.Link{link(2, "disk", personal, original, detached)}, true}, // covers: R-STATUS-05
		{"a detached source of a converted asset", []domain.Link{link(2, "disk", personal, source, detached)}, true},
		{"a detached file and an active one", []domain.Link{file, link(2, "disk", personal, original, detached)}, true},
		{"a detached file under the other root and an active one", []domain.Link{file, link(2, "disk", shared, original, detached)}, true},
		{"a detached Live Photo video and an active file", []domain.Link{file, link(2, "disk", personal, live, detached)}, false},
		{"a detached RAW of a pair and an active file", []domain.Link{file, link(2, "disk", personal, domain.RoleRAWPair, detached)}, false},
		{"a detached derived copy and an active file", []domain.Link{file, link(2, "disk", personal, domain.RoleDerived, detached)}, false},
		{"a file detached in another remote", []domain.Link{file, link(2, "box", personal, original, detached)}, false},
		{"a file quarantined in iCloud", []domain.Link{link(2, "disk", personal, original, domain.StatusQuarantinedICloud)}, false},
		{"a file quarantined in the cloud", []domain.Link{link(2, "disk", personal, original, domain.StatusQuarantinedRemote)}, false},
	} {
		if got := plan.StatusFrozen(c.links, "disk"); got != c.want {
			t.Errorf("%s: StatusFrozen = %v; want %v", c.name, got, c.want)
		}
	}
}
