package domain_test

import (
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// appOnly excuses a rule that only a scenario of the app can exercise until
// the test of that scenario carries its tag; the reason names the scenario,
// as "app: S<n>".
var appOnly = map[string]string{}

// Plans, explanations and the traceability of the rules name a rule by its
// ID, so a test under internal that exercises the rule carries the whole
// comment "// covers: <ID>", one ID to a comment, or appOnly excuses the
// rule. Such a comment may name an invariant INV-<n> or a scenario S<n>
// instead.
func TestEveryRuleExercised(t *testing.T) {
	t.Parallel()
	for _, p := range ruleProblems(t, os.DirFS(".."), domain.Rules, appOnly) {
		t.Errorf("%s; see the doc comment of TestEveryRuleExercised", p)
	}
}

// The tree of internal has nothing to report, so a small tree with one of
// each problem shows that the scan would report it.
func TestEveryRuleExercisedFixture(t *testing.T) {
	t.Parallel()
	const planTest = `package plan_test

import "testing"

// A comment that is no tag.
// covers: R-LINK-01
// covers: R-LINK-99
func TestPlan(t *testing.T) {
	_ = "// covers: R-MODE-01" // covers: INV-9
	// covers: S16
}

// covers: R-LINK-01 R-LINK-03
//covers: R-LINK-03
`
	fsys := fstest.MapFS{
		"layout/layout_test.go": {Data: []byte("package layout_test\n\nfunc TestLayout() {} // covers: R-LINK-02\n")},
		"plan/plan.go":          {Data: []byte("package plan\n\n// covers: R-MODE-02\nvar A = 1\n")},
		"plan/plan_test.go":     {Data: []byte(planTest)},
	}
	rules := map[string]string{}
	for _, id := range []string{"R-LINK-01", "R-LINK-02", "R-LINK-03", "R-MODE-01", "R-MODE-02", "R-STATUS-01", "R-STATUS-02", "R-STATUS-03", "R-STATUS-04"} {
		rules[id] = "A rule."
	}
	exceptions := map[string]string{
		"R-LINK-02":   "app: S3 later",
		"R-NOPE-01":   "",
		"R-STATUS-01": "app: S12",
		"R-STATUS-02": "",
		"R-STATUS-03": "app: S4 later",
		"R-STATUS-04": "see app: S4",
	}
	rulesBefore, exceptionsBefore := maps.Clone(rules), maps.Clone(exceptions)
	got := ruleProblems(t, fsys, rules, exceptions)
	want := []string{
		// Two IDs in one comment, and a tag without its space, name no rule.
		"R-LINK-03: no test covers it",
		// A string is no comment.
		"R-MODE-01: no test covers it",
		// A file that is no test exercises nothing.
		"R-MODE-02: no test covers it",
		"exception R-LINK-02: a test covers it",
		"exception R-NOPE-01: unknown rule",
		`exception R-STATUS-02: reason "" is not app: S<n>`,
		`exception R-STATUS-03: reason "app: S4 later" is not app: S<n>`,
		`exception R-STATUS-04: reason "see app: S4" is not app: S<n>`,
		"plan/plan_test.go:13: unknown rule R-LINK-01 R-LINK-03",
		"plan/plan_test.go:7: unknown rule R-LINK-99",
	}
	for _, w := range want {
		if !slices.Contains(got, w) {
			t.Errorf("missing %s", w)
		}
	}
	for _, g := range got {
		if !slices.Contains(want, g) {
			t.Errorf("unexpected %s", g)
		}
	}
	if !maps.Equal(rules, rulesBefore) || !maps.Equal(exceptions, exceptionsBefore) {
		t.Error("ruleProblems changed its rules or its exceptions")
	}
}

// ruleProblems reads the comments of the _test.go files under fsys and gives,
// sorted: each tag "// covers: <ID>" whose ID is neither a key of rules nor an
// invariant INV-<n> or a scenario S<n>, with its file and line; each rule that
// no tag names and exceptions does not hold; and each exception for an ID that
// is not a rule, for a rule that a tag names, or with a reason that is not
// "app: S<n>".
func ruleProblems(t *testing.T, fsys fs.FS, rules, exceptions map[string]string) []string {
	t.Helper()
	others := regexp.MustCompile(`^(INV-\d+|S\d+)$`)
	scenario := regexp.MustCompile(`^app: S\d+$`)
	fset := token.NewFileSet()
	covered := map[string]bool{}
	var problems []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, "_test.go") {
			return err
		}
		src, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		f, err := parser.ParseFile(fset, p, src, parser.ParseComments)
		if err != nil {
			return err
		}
		for _, g := range f.Comments {
			for _, c := range g.List {
				id, tagged := strings.CutPrefix(c.Text, "// covers: ")
				_, known := rules[id]
				// A comment that is no tag keeps its slashes, so it names no rule.
				switch {
				case known:
					covered[id] = true
				case tagged && !others.MatchString(id):
					problems = append(problems, fmt.Sprintf("%s:%d: unknown rule %s", p, fset.Position(c.Pos()).Line, id))
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for id := range rules {
		if _, excused := exceptions[id]; !covered[id] && !excused {
			problems = append(problems, id+": no test covers it")
		}
	}
	for id, reason := range exceptions {
		_, known := rules[id]
		switch {
		case !known:
			problems = append(problems, "exception "+id+": unknown rule")
		case covered[id]:
			problems = append(problems, "exception "+id+": a test covers it")
		case !scenario.MatchString(reason):
			problems = append(problems, fmt.Sprintf("exception %s: reason %q is not app: S<n>", id, reason))
		}
	}
	slices.Sort(problems)
	return problems
}
