package plan_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
	"github.com/Windemiatrix/photosync/internal/domain/plan"
)

var changeKinds = []domain.ChangeKind{
	domain.ChangeNone, domain.ChangeNew, domain.ChangeUnchanged,
	domain.ChangeMoved, domain.ChangeReplaced, domain.ChangeDeleted,
}

type linkPair struct{ icloud, remote domain.ChangeKind }

// Written rule by rule from the rule texts, not from the code, so that a
// wrong cell of the code's table disagrees with it.
var linkRules = []struct {
	rule     string
	action   domain.LinkAction
	onlySame bool
	pairs    []linkPair
}{
	{"", domain.ActionNone, false, []linkPair{{domain.ChangeUnchanged, domain.ChangeUnchanged}}},
	{"R-LINK-01", domain.ActionUpload, false, []linkPair{{domain.ChangeNew, domain.ChangeNone}}},                                                     // covers: R-LINK-01
	{"R-LINK-02", domain.ActionImport, false, []linkPair{{domain.ChangeNone, domain.ChangeNew}}},                                                     // covers: R-LINK-02
	{"R-LINK-03", domain.ActionLink, false, []linkPair{{domain.ChangeNew, domain.ChangeNew}}},                                                        // covers: R-LINK-03
	{"R-LINK-04", domain.ActionMoveFile, false, []linkPair{{domain.ChangeMoved, domain.ChangeUnchanged}}},                                            // covers: R-LINK-04
	{"R-LINK-05", domain.ActionMoveAsset, false, []linkPair{{domain.ChangeUnchanged, domain.ChangeMoved}}},                                           // covers: R-LINK-05
	{"R-LINK-06", domain.ActionReplaceAsset, false, []linkPair{{domain.ChangeUnchanged, domain.ChangeReplaced}}},                                     // covers: R-LINK-06
	{"R-LINK-07", domain.ActionConflict, false, []linkPair{{domain.ChangeMoved, domain.ChangeMoved}}},                                                // covers: R-LINK-07
	{"R-LINK-08", domain.ActionDeleteFile, false, []linkPair{{domain.ChangeDeleted, domain.ChangeUnchanged}}},                                        // covers: R-LINK-08
	{"R-LINK-09", domain.ActionDeleteAsset, false, []linkPair{{domain.ChangeUnchanged, domain.ChangeDeleted}}},                                       // covers: R-LINK-09
	{"R-LINK-10", domain.ActionUpload, false, []linkPair{{domain.ChangeMoved, domain.ChangeDeleted}}},                                                // covers: R-LINK-10
	{"R-LINK-11", domain.ActionImport, false, []linkPair{{domain.ChangeDeleted, domain.ChangeMoved}, {domain.ChangeDeleted, domain.ChangeReplaced}}}, // covers: R-LINK-11
	{"R-LINK-12", domain.ActionForget, false, []linkPair{{domain.ChangeDeleted, domain.ChangeDeleted}}},                                              // covers: R-LINK-12
	{"R-LINK-13", domain.ActionUpdateBase, true, []linkPair{{domain.ChangeMoved, domain.ChangeMoved}}},                                               // covers: R-LINK-13
	{"R-LINK-14", domain.ActionMoveFileReplaceAsset, false, []linkPair{{domain.ChangeMoved, domain.ChangeReplaced}}},                                 // covers: R-LINK-14
}

// A base belongs to a link, so both sides have one or neither has; an edit
// in iCloud is no change, so only a file is replaced; and with nothing on
// either side there is nothing to decide.
func impossibleLink(p linkPair) bool {
	noBase := func(c domain.ChangeKind) bool { return c == domain.ChangeNone || c == domain.ChangeNew }
	return p.icloud == domain.ChangeReplaced || noBase(p.icloud) != noBase(p.remote) ||
		p == (linkPair{domain.ChangeNone, domain.ChangeNone})
}

// Every pair of changes, with and without the same move on both sides, is
// either decided as its row of linkRules says or cannot happen at all.
func TestDecideLink(t *testing.T) {
	t.Parallel()
	type key struct {
		linkPair
		same bool
	}
	want := map[key]domain.LinkDecision{}
	for _, onlySame := range []bool{false, true} {
		for _, r := range linkRules {
			if r.onlySame != onlySame {
				continue
			}
			for _, p := range r.pairs {
				d := domain.LinkDecision{ICloud: p.icloud, Remote: p.remote, Action: r.action, Rule: r.rule}
				want[key{p, true}] = d
				if !onlySame {
					want[key{p, false}] = d
				}
			}
		}
	}
	for _, icloud := range changeKinds {
		for _, remote := range changeKinds {
			for _, same := range []bool{false, true} {
				p := linkPair{icloud, remote}
				w, listed := want[key{p, same}]
				got, ok := plan.DecideLink(icloud, remote, same)
				switch {
				case listed == impossibleLink(p):
					t.Errorf("iCloud %s, cloud %s, same %v: listed and impossible are both %v; want exactly one", icloud, remote, same, listed)
				case !listed && ok:
					t.Errorf("DecideLink(%s, %s, %v) = %+v, true; want no decision", icloud, remote, same, got)
				case listed && (!ok || got != w):
					t.Errorf("DecideLink(%s, %s, %v) = %+v, %v; want %+v, true", icloud, remote, same, got, ok, w)
				}
			}
		}
	}
}

// Plans and explanations name the rule of a decision by its ID, so the table
// uses only IDs of the registry and every link rule of it decides a pair.
func TestDecideLinkRules(t *testing.T) {
	t.Parallel()
	used := map[string]bool{}
	for _, icloud := range changeKinds {
		for _, remote := range changeKinds {
			for _, same := range []bool{false, true} {
				if d, ok := plan.DecideLink(icloud, remote, same); ok && d.Rule != "" {
					used[d.Rule] = true
				}
			}
		}
	}
	for _, id := range slices.Sorted(maps.Keys(used)) {
		if _, ok := domain.Rules[id]; !ok {
			t.Errorf("%s is not in domain.Rules", id)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(domain.Rules)) {
		if strings.HasPrefix(id, "R-LINK-") && !used[id] {
			t.Errorf("%s decides no pair of changes", id)
		}
	}
}
