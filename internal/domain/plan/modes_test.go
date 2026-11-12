package plan_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

// Written from the rule texts, not from the code: a one-way remote runs only
// what changes the side it syncs. An action unknown here runs only two-way,
// so that an action added later stays safe until it is sorted.
func TestModeAllows(t *testing.T) {
	t.Parallel()
	unknown := domain.LinkAction("not_an_action")
	all := []domain.LinkAction{
		domain.ActionNone, domain.ActionUpload, domain.ActionImport, domain.ActionLink,
		domain.ActionMoveFile, domain.ActionMoveAsset, domain.ActionReplaceAsset,
		domain.ActionMoveFileReplaceAsset, domain.ActionConflict, domain.ActionDeleteFile,
		domain.ActionDeleteAsset, domain.ActionForget, domain.ActionUpdateBase,
		domain.ActionCopyFile, domain.ActionQuarantineFile, unknown,
	}
	for _, c := range []struct {
		mode    domain.Mode
		allowed []domain.LinkAction
	}{
		{domain.ModeTwoWay, all},
		{domain.ModeUploadOnly, []domain.LinkAction{ // covers: R-MODE-01
			domain.ActionNone, domain.ActionLink, domain.ActionForget, domain.ActionUpdateBase,
			domain.ActionUpload, domain.ActionMoveFile, domain.ActionCopyFile, domain.ActionDeleteFile, domain.ActionQuarantineFile,
		}},
		{domain.ModeDownloadOnly, []domain.LinkAction{ // covers: R-MODE-02
			domain.ActionNone, domain.ActionLink, domain.ActionForget, domain.ActionUpdateBase,
			domain.ActionImport, domain.ActionMoveAsset, domain.ActionReplaceAsset, domain.ActionDeleteAsset,
		}},
	} {
		for _, a := range all {
			if got, want := plan.ModeAllows(c.mode, a), slices.Contains(c.allowed, a); got != want {
				t.Errorf("ModeAllows(%s, %s) = %v; want %v", c.mode, a, got, want)
			}
		}
	}
}

// Written per mode from the rule texts, not from the code. A two-way remote
// decides as DecideLink does. A one-way remote does not sync one side: a
// change there is no change that the base takes, a deletion there alone and
// a new asset or file there are left as they are, and the same move on both
// sides stays one.
func TestModeDecide(t *testing.T) {
	t.Parallel()
	const (
		none, added, unchanged   = domain.ChangeNone, domain.ChangeNew, domain.ChangeUnchanged
		moved, replaced, deleted = domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted
		upload, download         = domain.ModeUploadOnly, domain.ModeDownloadOnly
	)
	type key struct {
		mode domain.Mode
		linkPair
		same bool
	}
	type outcome struct {
		action domain.LinkAction
		rule   string
	}
	rows := []struct {
		mode     domain.Mode
		rule     string
		action   domain.LinkAction
		onlySame bool
		pairs    []linkPair
	}{
		{upload, "", domain.ActionNone, false, []linkPair{{unchanged, unchanged}}},
		{upload, "R-LINK-01", domain.ActionUpload, false, []linkPair{{added, none}}},
		{upload, "R-LINK-03", domain.ActionLink, false, []linkPair{{added, added}}},
		{upload, "R-MODE-01", domain.ActionNone, false, []linkPair{{none, added}, {unchanged, deleted}, {moved, deleted}}}, // covers: R-MODE-01
		{upload, "R-MODE-01", domain.ActionUpdateBase, false, []linkPair{{unchanged, moved}, {unchanged, replaced}}},
		{upload, "R-LINK-04", domain.ActionMoveFile, false, []linkPair{{moved, unchanged}, {moved, moved}, {moved, replaced}}}, // covers: R-MODE-03
		{upload, "R-LINK-13", domain.ActionUpdateBase, true, []linkPair{{moved, moved}}},
		{upload, "R-LINK-08", domain.ActionDeleteFile, false, []linkPair{{deleted, unchanged}, {deleted, moved}, {deleted, replaced}}},
		{upload, "R-LINK-12", domain.ActionForget, false, []linkPair{{deleted, deleted}}},

		{download, "", domain.ActionNone, false, []linkPair{{unchanged, unchanged}}},
		{download, "R-LINK-02", domain.ActionImport, false, []linkPair{{none, added}}},
		{download, "R-LINK-03", domain.ActionLink, false, []linkPair{{added, added}}},
		{download, "R-MODE-02", domain.ActionNone, false, []linkPair{{added, none}, {deleted, unchanged}, {deleted, moved}, {deleted, replaced}}}, // covers: R-MODE-02
		{download, "R-MODE-02", domain.ActionUpdateBase, false, []linkPair{{moved, unchanged}}},
		{download, "R-LINK-05", domain.ActionMoveAsset, false, []linkPair{{unchanged, moved}, {moved, moved}}}, // covers: R-MODE-03
		{download, "R-LINK-13", domain.ActionUpdateBase, true, []linkPair{{moved, moved}}},
		{download, "R-LINK-06", domain.ActionReplaceAsset, false, []linkPair{{unchanged, replaced}, {moved, replaced}}},
		{download, "R-LINK-09", domain.ActionDeleteAsset, false, []linkPair{{unchanged, deleted}, {moved, deleted}}},
		{download, "R-LINK-12", domain.ActionForget, false, []linkPair{{deleted, deleted}}},
	}
	want := map[key]outcome{}
	for _, onlySame := range []bool{false, true} {
		for _, r := range rows {
			if r.onlySame != onlySame {
				continue
			}
			for _, p := range r.pairs {
				want[key{r.mode, p, true}] = outcome{r.action, r.rule}
				if !onlySame {
					want[key{r.mode, p, false}] = outcome{r.action, r.rule}
				}
			}
		}
	}
	for _, r := range rows {
		if _, ok := domain.Rules[r.rule]; r.rule != "" && !ok {
			t.Errorf("%s is not in domain.Rules", r.rule)
		}
	}
	equal := func(a, b domain.LinkDecision) bool {
		return a.ICloud == b.ICloud && a.Remote == b.Remote && a.Action == b.Action && a.Rule == b.Rule
	}
	for _, icloud := range changeKinds {
		for _, remote := range changeKinds {
			for _, same := range []bool{false, true} {
				p := linkPair{icloud, remote}
				two, twoOK := plan.DecideLink(icloud, remote, same)
				if got, ok := plan.ModeDecide(domain.ModeTwoWay, icloud, remote, same); ok != twoOK || ok && !equal(got, two) {
					t.Errorf("ModeDecide(two-way, %s, %s, %v) = %+v, %v; want %+v, %v as DecideLink", icloud, remote, same, got, ok, two, twoOK)
				}
				for _, m := range []domain.Mode{upload, download} {
					w, listed := want[key{m, p, same}]
					got, ok := plan.ModeDecide(m, icloud, remote, same)
					switch {
					case listed == impossibleLink(p):
						t.Errorf("%s, iCloud %s, cloud %s, same %v: listed and impossible are both %v; want exactly one", m, icloud, remote, same, listed)
					case !listed && ok:
						t.Errorf("ModeDecide(%s, %s, %s, %v) = %+v, true; want no decision", m, icloud, remote, same, got)
					case listed && (!ok || !equal(got, domain.LinkDecision{ICloud: icloud, Remote: remote, Action: w.action, Rule: w.rule})):
						t.Errorf("ModeDecide(%s, %s, %s, %v) = %+v, %v; want %s by %q, true", m, icloud, remote, same, got, ok, w.action, w.rule)
					}
				}
			}
		}
	}
}

// Every pair of changes, with and without the same move on both sides, is
// decided in every mode under every deletes policy, and every rule named is
// in the registry. A one-way remote never runs what changes the side that it
// does not sync, so it has no conflicts, and a deletion of that side alone
// leaves the link detached whatever the policy.
//
// covers: INV-5
func TestModeProjection(t *testing.T) {
	t.Parallel()
	toCloud := []domain.LinkAction{domain.ActionUpload, domain.ActionMoveFile, domain.ActionCopyFile, domain.ActionDeleteFile, domain.ActionQuarantineFile}
	toICloud := []domain.LinkAction{domain.ActionImport, domain.ActionMoveAsset, domain.ActionReplaceAsset, domain.ActionDeleteAsset}
	both := []domain.LinkAction{domain.ActionMoveFileReplaceAsset, domain.ActionConflict}
	decided := 0
	for _, m := range []domain.Mode{domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly} {
		for _, deletes := range []domain.DeletesPolicy{domain.DeletesNone, domain.DeletesQuarantine, domain.DeletesPropagate} {
			for _, icloud := range changeKinds {
				for _, remote := range changeKinds {
					for _, same := range []bool{false, true} {
						d, ok := plan.ModeDecide(m, icloud, remote, same)
						if !ok {
							continue
						}
						decided++
						got, status := plan.StatusDeletes(d, deletes)
						where := fmt.Sprintf("%s, %s, iCloud %s, cloud %s, same %v: %s by %q, link %q", m, deletes, icloud, remote, same, got.Action, got.Rule, status)
						_, known := domain.Rules[got.Rule]
						alone := m == domain.ModeUploadOnly && remote == domain.ChangeDeleted && icloud != domain.ChangeDeleted ||
							m == domain.ModeDownloadOnly && icloud == domain.ChangeDeleted && remote != domain.ChangeDeleted
						switch {
						case got.ICloud != icloud || got.Remote != remote:
							t.Errorf("%s; the sides became %s, %s", where, got.ICloud, got.Remote)
						case m != domain.ModeTwoWay && slices.Contains(both, got.Action): // covers: R-MODE-03
							t.Errorf("%s; a one-way remote changes both sides", where)
						case m == domain.ModeUploadOnly && slices.Contains(toICloud, got.Action),
							m == domain.ModeDownloadOnly && slices.Contains(toCloud, got.Action):
							t.Errorf("%s; it changes the side that the remote does not sync", where)
						case alone && (got.Action != domain.ActionNone || status != domain.StatusDetached):
							t.Errorf("%s; want none with the link detached", where)
						case !known && (got.Rule != "" || got.Action != domain.ActionNone):
							t.Errorf("%s; the rule is not in domain.Rules", where)
						}
					}
				}
			}
		}
	}
	if decided != 270 {
		t.Errorf("decided %d combinations; want 270: each of the 15 possible pairs, with and without same, in 3 modes under 3 policies", decided)
	}
}

// Written per mode from the rule on the albums of the personal root, not
// from the code: there a move of a file is no change, as on the side that a
// one-way remote does not sync, so the base takes it, an asset moved in
// iCloud takes its file along, and an asset deleted in iCloud stays deleted.
// The same move on both sides and every cell without a move of the file
// decide as ModeDecide does.
func TestModeKeepAlbums(t *testing.T) {
	t.Parallel()
	const (
		unchanged, moved, deleted = domain.ChangeUnchanged, domain.ChangeMoved, domain.ChangeDeleted
		two, upload, download     = domain.ModeTwoWay, domain.ModeUploadOnly, domain.ModeDownloadOnly
	)
	type key struct {
		mode domain.Mode
		linkPair
	}
	type outcome struct {
		action domain.LinkAction
		rule   string
	}
	moves := map[key]outcome{
		{two, linkPair{unchanged, moved}}:      {domain.ActionUpdateBase, "R-MODE-04"}, // covers: R-MODE-04
		{two, linkPair{moved, moved}}:          {domain.ActionMoveFile, "R-LINK-04"},
		{two, linkPair{deleted, moved}}:        {domain.ActionDeleteFile, "R-LINK-08"},
		{upload, linkPair{unchanged, moved}}:   {domain.ActionUpdateBase, "R-MODE-04"},
		{upload, linkPair{moved, moved}}:       {domain.ActionMoveFile, "R-LINK-04"},
		{upload, linkPair{deleted, moved}}:     {domain.ActionDeleteFile, "R-LINK-08"},
		{download, linkPair{unchanged, moved}}: {domain.ActionUpdateBase, "R-MODE-04"},
		{download, linkPair{moved, moved}}:     {domain.ActionUpdateBase, "R-MODE-02"},
		{download, linkPair{deleted, moved}}:   {domain.ActionNone, "R-MODE-02"},
	}
	for _, o := range moves {
		if _, ok := domain.Rules[o.rule]; !ok {
			t.Errorf("%s is not in domain.Rules", o.rule)
		}
	}
	equal := func(a, b domain.LinkDecision) bool {
		return a.ICloud == b.ICloud && a.Remote == b.Remote && a.Action == b.Action && a.Rule == b.Rule
	}
	for _, m := range []domain.Mode{two, upload, download} {
		for _, icloud := range changeKinds {
			for _, remote := range changeKinds {
				for _, same := range []bool{false, true} {
					got, ok := plan.ModeKeepAlbums(m, icloud, remote, same)
					w, listed := moves[key{m, linkPair{icloud, remote}}]
					want, wantOK := plan.ModeDecide(m, icloud, remote, same)
					switch {
					case listed && (!same || icloud != moved):
						if !ok || !equal(got, domain.LinkDecision{ICloud: icloud, Remote: remote, Action: w.action, Rule: w.rule}) {
							t.Errorf("ModeKeepAlbums(%s, %s, %s, %v) = %s by %q with sides %s, %s, %v; want %s by %q, true",
								m, icloud, remote, same, got.Action, got.Rule, got.ICloud, got.Remote, ok, w.action, w.rule)
						}
					case ok != wantOK || ok && !equal(got, want):
						t.Errorf("ModeKeepAlbums(%s, %s, %s, %v) = %s by %q with sides %s, %s, %v; want %s by %q with sides %s, %s, %v as ModeDecide",
							m, icloud, remote, same, got.Action, got.Rule, got.ICloud, got.Remote, ok, want.Action, want.Rule, want.ICloud, want.Remote, wantOK)
					}
				}
			}
		}
	}
}
