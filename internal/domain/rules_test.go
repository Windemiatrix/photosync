package domain_test

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/Windemiatrix/photosync/internal/domain"
)

// Plan operations, explanations and test tags name a rule by its ID, so the
// registry holds exactly these IDs, numbered without gaps within a group.
func TestRules(t *testing.T) {
	t.Parallel()
	sentence := regexp.MustCompile(`^[A-Z][ -~]*\.$`)
	// The registry is public: a rule text cites no requirement numbers of
	// documents outside the repository.
	privateRef := regexp.MustCompile(`\b(N?FR|SEC|QA)-|\bD\d`)
	want := map[string]bool{}
	for group, count := range map[string]int{"LINK": 14, "OBJECT": 9, "COMPONENT": 5, "MODE": 4, "STATUS": 9, "LAYOUT": 10} {
		for n := 1; n <= count; n++ {
			want[fmt.Sprintf("R-%s-%02d", group, n)] = true
		}
	}
	for _, id := range slices.Sorted(maps.Keys(want)) {
		text, ok := domain.Rules[id]
		switch {
		case !ok:
			t.Errorf("%s is missing", id)
		case !sentence.MatchString(text) || strings.Contains(text, ". "):
			t.Errorf("%s: %q is not one sentence of printable ASCII", id, text)
		case privateRef.MatchString(text):
			t.Errorf("%s: %q refers to a private document", id, text)
		}
	}
	for _, id := range slices.Sorted(maps.Keys(domain.Rules)) {
		if !want[id] {
			t.Errorf("%s is not a rule ID", id)
		}
	}
}
